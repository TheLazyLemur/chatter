// Package app is the composition root of the desktop app.
package app

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/veandco/go-sdl2/sdl"

	"chatter/internal/audiofile"
	"chatter/internal/config"
	"chatter/internal/core"
	"chatter/internal/desktop"
	"chatter/internal/gui"
	"chatter/internal/history"
	"chatter/internal/hotkey"
	"chatter/internal/sdlaudio"
	"chatter/internal/wiring"
	"chatter/pkg/clock"
)

func Run(ctx context.Context) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfgPath, err := config.DefaultPath()
	if err != nil {
		return err
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}
	dir := filepath.Dir(cfgPath)
	histDir := cfg.HistoryDir
	if histDir == "" {
		histDir = filepath.Join(dir, "history")
	}
	logFile, err := os.OpenFile(filepath.Join(dir, "chatter.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer logFile.Close()
	logger := log.New(logFile, "", log.LstdFlags)
	messages := make(chan string, 16)

	sdl.SetHint(sdl.HINT_MAC_BACKGROUND_APP, "1") // a menu bar app: no Dock icon
	if err := sdl.Init(sdl.INIT_VIDEO | sdl.INIT_AUDIO); err != nil {
		return fmt.Errorf("sdl: %w", err)
	}
	defer sdl.Quit()

	speech := wiring.NewSpeech(cfg.Transcriber, logFile)
	if speech.Server != nil {
		defer speech.Server.Stop()
		go func() {
			if err := speech.Server.EnsureRunning(ctx, 5*time.Minute); err != nil {
				logger.Printf("transcription server: %v", err)
				messages <- "Transcription server: " + err.Error()
			}
		}()
	}
	rewriter, rewriterInfo := wiring.NewRewriter(cfg.Rewriter)
	speaker, err := sdlaudio.OpenSpeaker()
	if err != nil {
		return err
	}
	defer speaker.Close()
	desk := desktop.New()
	hist := &history.Store{Dir: histDir}
	clk := clock.System{}

	proc := &core.Processor{Transcriber: speech.Transcriber, Rewriter: rewriter, History: hist, Clock: clk}
	d := core.NewDictation(proc, &sdlaudio.Microphone{}, speech.Live, desk, desk, speaker, &audiofile.FFmpeg{}, clk, cfg.Settings)
	if last, err := hist.List("", 1); err == nil && len(last) > 0 {
		d.SetLast(last[0])
	}

	showWindow := make(chan struct{}, 1)
	keys := &hotkeys{d: d, show: showWindow}
	var modTime time.Time
	if fi, err := os.Stat(cfgPath); err == nil {
		modTime = fi.ModTime()
	}
	save := func(s core.Settings) error {
		if err := d.SetSettings(s); err != nil {
			return err
		}
		cfg.Settings = s
		if err := config.Save(cfgPath, cfg); err != nil {
			return err
		}
		if fi, err := os.Stat(cfgPath); err == nil {
			modTime = fi.ModTime()
		}
		return keys.register(cfg)
	}
	keys.save = save
	if err := keys.register(cfg); err != nil {
		messages <- err.Error()
	}

	lastCheck := time.Now()
	tick := func() string {
		keys.syncCancel(cfg.Hotkeys.Cancel)
		if time.Since(lastCheck) > time.Second {
			lastCheck = time.Now()
			if fi, err := os.Stat(cfgPath); err == nil && !fi.ModTime().Equal(modTime) {
				modTime = fi.ModTime()
				next, err := config.Load(cfgPath)
				if err != nil {
					return "Settings not reloaded: " + err.Error()
				}
				restart := next.Transcriber.Kind != cfg.Transcriber.Kind || next.Transcriber.BaseURL != cfg.Transcriber.BaseURL ||
					next.Rewriter != cfg.Rewriter
				if err := d.SetSettings(next.Settings); err != nil {
					return "Settings not reloaded: " + err.Error()
				}
				cfg = next
				if err := keys.register(cfg); err != nil {
					return err.Error()
				}
				if restart {
					return "Settings reloaded. Restart Chatter to switch transcriber or rewriter."
				}
				d.Notify("Settings reloaded", false)
			}
		}
		select {
		case m := <-messages:
			return m
		default:
			return ""
		}
	}

	return gui.Run(ctx, gui.Deps{
		Dictation:    d,
		History:      hist,
		SaveSettings: save,
		Play:         speaker.PlaySamples,
		StopPlayback: speaker.StopPlayback,
		Microphones:  sdlaudio.Devices,
		OpenPath:     desktop.Open,
		Accessible:   desktop.Trusted,
		Health:       speech.Health,
		Tick:         tick,
		Info:         []string{speech.Describe, rewriterInfo, "Settings: " + cfgPath, "Log: " + logFile.Name()},
		Hotkeys:      keys.labels(cfg),
		SettingsPath: cfgPath,
		HistoryDir:   histDir,
		FontPath:     cfg.FontPath,
		ShowWindow:   showWindow,
	})
}

// hotkeys binds config.Hotkeys and per-mode hotkeys to the Dictation.
type hotkeys struct {
	d        *core.Dictation
	show     chan struct{}
	save     func(core.Settings) error
	cancelID int
}

func (h *hotkeys) register(cfg config.Config) error {
	hotkey.UnregisterAll()
	h.cancelID = 0
	var errs []string
	bind := func(combo string, down, up func()) {
		if combo == "" {
			return
		}
		if _, err := hotkey.Register(combo, down, up); err != nil {
			errs = append(errs, err.Error())
		}
	}
	bind(cfg.Hotkeys.Record, func() { h.d.PressRecord("") }, h.d.ReleaseRecord)
	for _, m := range cfg.Modes {
		key := m.Key
		bind(m.Hotkey, func() { h.d.PressRecord(key) }, h.d.ReleaseRecord)
	}
	bind(cfg.Hotkeys.CycleMode, func() {
		s := h.d.Settings()
		s.DefaultMode = core.NextMode(s, s.DefaultMode)
		if err := h.save(s); err != nil {
			h.d.Notify(err.Error(), true)
			return
		}
		h.d.Notify("Mode: "+core.ModeLabel(s, s.DefaultMode), false)
	}, nil)
	bind(cfg.Hotkeys.PasteLast, func() {
		go func() {
			if err := h.d.PasteLast(context.Background()); err != nil {
				h.d.Notify(err.Error(), true)
			}
		}()
	}, nil)
	bind(cfg.Hotkeys.OpenWindow, func() {
		select {
		case h.show <- struct{}{}:
		default:
		}
	}, nil)
	if len(errs) > 0 {
		return fmt.Errorf("hotkeys: %s", strings.Join(errs, "; "))
	}
	return nil
}

// syncCancel holds the cancel hotkey only while recording, so Esc works normally otherwise.
func (h *hotkeys) syncCancel(combo string) {
	recording := h.d.Status().State != core.StateIdle
	switch {
	case recording && h.cancelID == 0 && combo != "":
		id, err := hotkey.Register(combo, h.d.Cancel, nil)
		if err == nil {
			h.cancelID = id
		}
	case !recording && h.cancelID != 0:
		hotkey.Unregister(h.cancelID)
		h.cancelID = 0
	}
}

func (h *hotkeys) labels(cfg config.Config) [][2]string {
	out := [][2]string{
		{"Record", pretty(cfg.Hotkeys.Record) + "  (tap to toggle, hold to talk)"},
		{"Cancel", pretty(cfg.Hotkeys.Cancel) + "  (while recording)"},
		{"Next mode", pretty(cfg.Hotkeys.CycleMode)},
		{"Paste last", pretty(cfg.Hotkeys.PasteLast)},
		{"Open window", pretty(cfg.Hotkeys.OpenWindow)},
	}
	for _, m := range cfg.Modes {
		if m.Hotkey != "" {
			out = append(out, [2]string{"Record in " + m.Name, pretty(m.Hotkey)})
		}
	}
	return out
}

func pretty(combo string) string {
	if combo == "" {
		return "not set"
	}
	mods := map[string]string{"control": "⌃", "ctrl": "⌃", "option": "⌥", "opt": "⌥", "alt": "⌥", "shift": "⇧", "cmd": "⌘", "command": "⌘"}
	var b strings.Builder
	for _, p := range strings.Split(strings.ToLower(strings.ReplaceAll(combo, " ", "")), "+") {
		if m, ok := mods[p]; ok {
			b.WriteString(m)
		} else if p != "" {
			b.WriteString(strings.ToUpper(p[:1]) + p[1:])
		}
	}
	return b.String()
}
