// Package wiring builds the outbound adapters that both deployable units choose from config,
// so the Transcriber and Rewriter seams are selected in exactly one place.
package wiring

import (
	"context"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"chatter/internal/config"
	"chatter/internal/core"
	"chatter/internal/provider/fermion"
	"chatter/internal/provider/openaiaudio"
	"chatter/internal/provider/openaichat"
)

// Speech is the chosen Transcriber with the extras its kind supports.
type Speech struct {
	Transcriber core.Transcriber
	Live        core.LivePreview
	Server      *fermion.Server // nil when nothing is supervised
	Health      func(context.Context) error
	Describe    string
}

func NewSpeech(cfg config.Transcriber, log io.Writer) Speech {
	client := &http.Client{Timeout: 5 * time.Minute}
	switch cfg.Kind {
	case "cli":
		cmd := ResolveCommand(cfg.CLICommand)
		return Speech{
			Transcriber: &fermion.CLITranscriber{Command: cmd},
			Live:        core.NoLivePreview{},
			Health:      func(context.Context) error { return nil },
			Describe:    "Transcriber: " + filepath.Base(cmd[0]) + " (command line, model loads per dictation)",
		}
	case "http":
		key := config.APIKey(cfg.APIKeyEnv)
		t := openaiaudio.New(openaiaudio.Config{BaseURL: cfg.BaseURL, Model: cfg.Model, APIKey: key}, client)
		s := Speech{Transcriber: t, Live: core.NoLivePreview{}, Health: t.Health, Describe: "Transcriber: " + cfg.Model + " at " + cfg.BaseURL}
		if cfg.LivePreview && cfg.StreamURL != "" {
			s.Live = &fermion.LivePreview{URL: cfg.StreamURL, APIKey: key}
		}
		if len(cfg.Autostart) > 0 {
			s.Server = &fermion.Server{Command: ResolveCommand(cfg.Autostart), HealthURL: cfg.HealthURL, Log: log}
			s.Transcriber = &fermion.Supervised{Server: s.Server, Transcriber: t}
		}
		return s
	}
	panic("wiring: transcriber kind was not validated: " + cfg.Kind)
}

// NewRewriter returns the configured Rewriter, or core.NoRewriter when none is set.
func NewRewriter(cfg config.Rewriter) (core.Rewriter, string) {
	if cfg.BaseURL == "" {
		return core.NoRewriter{}, "Rewriter: none (modes with instructions will paste the raw transcript)"
	}
	r := openaichat.New(openaichat.Config{BaseURL: cfg.BaseURL, Model: cfg.Model, APIKey: config.APIKey(cfg.APIKeyEnv)},
		&http.Client{Timeout: 90 * time.Second})
	return r, "Rewriter: " + cfg.Model + " at " + cfg.BaseURL
}

// ResolveCommand finds cmd[0] on PATH, or in a .venv beside the binary, the working directory or the config dir.
func ResolveCommand(cmd []string) []string {
	if len(cmd) == 0 {
		panic("wiring: empty command")
	}
	if _, err := exec.LookPath(cmd[0]); err == nil || filepath.IsAbs(cmd[0]) {
		return cmd
	}
	var dirs []string
	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Dir(exe), filepath.Join(filepath.Dir(exe), ".."))
	}
	if wd, err := os.Getwd(); err == nil {
		dirs = append(dirs, wd)
	}
	if d, err := config.Dir(); err == nil {
		dirs = append(dirs, d) // where `make setup` installs it, so Chatter.app finds it
	}
	for _, d := range dirs {
		p := filepath.Join(d, ".venv", "bin", cmd[0])
		if _, err := os.Stat(p); err == nil {
			abs, _ := filepath.Abs(p)
			return append([]string{abs}, cmd[1:]...)
		}
	}
	return cmd
}
