// Package cli is the composition root of chatter-cli.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"chatter/internal/audiofile"
	"chatter/internal/config"
	"chatter/internal/core"
	"chatter/internal/history"
	"chatter/internal/wiring"
	"chatter/pkg/clock"
)

const usage = `usage:
  chatter-cli transcribe [-mode KEY] [-copy] FILE...   transcribe audio/video files (saved to History)
  chatter-cli history [-n 20] [-q TEXT] [-json]        list History
  chatter-cli modes                                    list modes
  chatter-cli doctor                                   check the transcriber, rewriter and ffmpeg`

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	if len(args) == 0 {
		return errors.New(usage)
	}
	cfgPath, err := config.DefaultPath()
	if err != nil {
		return err
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	histDir := cfg.HistoryDir
	if histDir == "" {
		histDir = filepath.Join(filepath.Dir(cfgPath), "history")
	}
	hist := &history.Store{Dir: histDir}

	switch args[0] {
	case "transcribe":
		fs := flag.NewFlagSet("transcribe", flag.ContinueOnError)
		mode := fs.String("mode", "", "mode key (default: the default mode)")
		copyOut := fs.Bool("copy", false, "copy the last output to the clipboard")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() == 0 {
			return errors.New(usage)
		}
		m, err := core.SelectMode(cfg.Settings, *mode, core.App{})
		if err != nil {
			return err
		}
		speech := wiring.NewSpeech(cfg.Transcriber, stderr)
		if speech.Server != nil {
			fmt.Fprintln(stderr, "waiting for the transcription server…")
			if err := speech.Server.EnsureRunning(ctx, 5*time.Minute); err != nil {
				return err
			}
			defer speech.Server.Stop()
		}
		rw, _ := wiring.NewRewriter(cfg.Rewriter)
		proc := &core.Processor{Transcriber: speech.Transcriber, Rewriter: rw, History: hist, Clock: clock.System{}}
		dec := &audiofile.FFmpeg{}
		var last string
		for _, path := range fs.Args() {
			rec, err := dec.Decode(ctx, path)
			if err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			e, err := proc.Process(ctx, rec, m, core.Context{}, cfg.Settings, path, func(s string) { fmt.Fprintln(stderr, s+"…", filepath.Base(path)) })
			if err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			if e.Error != "" {
				fmt.Fprintln(stderr, "warning:", e.Error)
			}
			fmt.Fprintln(stdout, e.Output)
			last = e.Output
		}
		if *copyOut {
			cmd := exec.Command("pbcopy")
			cmd.Stdin = strings.NewReader(last)
			return cmd.Run()
		}
		return nil

	case "history":
		fs := flag.NewFlagSet("history", flag.ContinueOnError)
		n := fs.Int("n", 20, "how many entries")
		q := fs.String("q", "", "search text")
		asJSON := fs.Bool("json", false, "print JSON")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		entries, err := hist.List(*q, *n)
		if err != nil {
			return err
		}
		if *asJSON {
			enc := json.NewEncoder(stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(entries)
		}
		for _, e := range entries {
			fmt.Fprintf(stdout, "%s  %-8s %6s  %s\n", e.CreatedAt.Local().Format("2006-01-02 15:04"), e.ModeName,
				e.Duration.Round(time.Second), strings.ReplaceAll(firstNonEmpty(e.Output, e.Error), "\n", " ⏎ "))
		}
		return nil

	case "modes":
		auto := " "
		if cfg.DefaultMode == core.AutoMode {
			auto = "*"
		}
		fmt.Fprintf(stdout, "%s %-8s %-8s %s\n", auto, core.AutoMode, "Auto", "mode follows the app")
		for _, m := range cfg.Modes {
			mark := " "
			if m.Key == cfg.DefaultMode {
				mark = "*"
			}
			fmt.Fprintf(stdout, "%s %-8s %-8s %s\n", mark, m.Key, m.Name, strings.Join(m.Apps, ", "))
		}
		return nil

	case "doctor":
		speech := wiring.NewSpeech(cfg.Transcriber, io.Discard)
		check := func(name string, err error) {
			status := "ok"
			if err != nil {
				status = "FAIL: " + err.Error()
			}
			fmt.Fprintf(stdout, "%-12s %s\n", name, status)
		}
		hctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		fmt.Fprintln(stdout, speech.Describe)
		check("transcriber", speech.Health(hctx))
		rw, desc := wiring.NewRewriter(cfg.Rewriter)
		fmt.Fprintln(stdout, desc)
		_, err := rw.Rewrite(hctx, core.Prompt{System: "Reply with: ok", User: "ok"})
		check("rewriter", err)
		_, err = exec.LookPath("ffmpeg")
		check("ffmpeg", err)
		fmt.Fprintln(stdout, "settings    ", cfgPath)
		fmt.Fprintln(stdout, "history     ", histDir)
		return nil
	}
	return errors.New(usage)
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
