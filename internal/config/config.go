// Package config loads and saves Chatter's settings file.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"chatter/internal/core"
)

type Hotkeys struct {
	Record     string `json:"record"`      // tap to toggle, hold for Push-to-talk
	Cancel     string `json:"cancel"`      // active only while recording
	CycleMode  string `json:"cycle_mode"`  // switch the default Mode
	PasteLast  string `json:"paste_last"`  // paste the last Output again
	OpenWindow string `json:"open_window"` // show the main window
}

type Transcriber struct {
	Kind        string   `json:"kind"`     // "http" (any OpenAI-compatible server) or "cli" (fermion transcribe)
	BaseURL     string   `json:"base_url"` // http
	Model       string   `json:"model"`
	APIKeyEnv   string   `json:"api_key_env"` // name of the env var holding the key
	CLICommand  []string `json:"cli_command"` // cli: the file path and --json are appended
	Autostart   []string `json:"autostart"`   // http: started when nothing answers at base_url
	HealthURL   string   `json:"health_url"`
	LivePreview bool     `json:"live_preview"`
	StreamURL   string   `json:"stream_url"` // Phonon WebSocket stream
}

type Rewriter struct {
	BaseURL   string `json:"base_url"` // empty disables rewriting
	Model     string `json:"model"`
	APIKeyEnv string `json:"api_key_env"`
}

type Config struct {
	core.Settings
	Hotkeys     Hotkeys     `json:"hotkeys"`
	Transcriber Transcriber `json:"transcriber"`
	Rewriter    Rewriter    `json:"rewriter"`
	HistoryDir  string      `json:"history_dir"`
	FontPath    string      `json:"font_path"`
}

// Dir is where Chatter keeps its settings and History.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "Chatter"), nil
}

func DefaultPath() (string, error) {
	d, err := Dir()
	return filepath.Join(d, "settings.json"), err
}

// Load reads path over the defaults, writing the defaults first if the file is missing.
func Load(path string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, Save(path, cfg)
	}
	if err != nil {
		return Config{}, err
	}
	// replace, don't merge, the lists the user edits
	cfg.Modes, cfg.Vocabulary, cfg.Replacements = nil, nil, nil
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	if cfg.Transcriber.Kind != "http" && cfg.Transcriber.Kind != "cli" {
		return Config{}, fmt.Errorf("%s: transcriber.kind must be \"http\" or \"cli\", got %q", path, cfg.Transcriber.Kind)
	}
	return cfg, nil
}

// Save writes cfg atomically.
func Save(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// APIKey reads the key named by env, if any.
func APIKey(env string) string {
	if env == "" {
		return ""
	}
	return os.Getenv(env)
}

func Default() Config {
	return Config{
		Settings: core.Settings{
			Modes:            defaultModes(),
			DefaultMode:      "voice",
			Vocabulary:       []string{"Chatter", "Phonon"},
			Replacements:     []core.Replacement{{From: "new paragraph", To: "\n\n"}, {From: "new line", To: "\n"}},
			AutoPaste:        true,
			RestoreClipboard: true,
			SoundCues:        true,
			HoldThresholdMS:  400,
			MinRecordingMS:   300,
		},
		Hotkeys: Hotkeys{
			Record:     "control+option+space", // not ⌥Space: Raycast and Alfred default to it
			Cancel:     "escape",
			CycleMode:  "control+option+m",
			PasteLast:  "control+option+v",
			OpenWindow: "control+option+o",
		},
		Transcriber: Transcriber{
			Kind:        "http",
			BaseURL:     "http://127.0.0.1:8000/v1",
			Model:       "phonon-2",
			CLICommand:  []string{"fermion", "transcribe", "phonon-2"},
			Autostart:   []string{"fermion", "serve", "phonon-2", "--port", "8000"},
			HealthURL:   "http://127.0.0.1:8000/health",
			LivePreview: true,
			StreamURL:   "ws://127.0.0.1:8000/v1/audio/stream",
		},
		Rewriter: Rewriter{
			BaseURL: "http://127.0.0.1:11434/v1", // Ollama
			Model:   "glm-5.3-flash:cloud",
		},
	}
}

func defaultModes() []core.Mode {
	return []core.Mode{
		{Key: "voice", Name: "Voice", Hotkey: ""},
		{Key: "message", Name: "Message", UseApp: true,
			Apps: []string{"Slack", "Messages", "Discord", "WhatsApp", "Telegram", "Signal"},
			Instructions: "Turn this into a chat message. Fix grammar and punctuation, drop filler words (um, uh, like, you know) " +
				"and false starts, and apply spoken self-corrections. Keep the speaker's wording and casual tone. " +
				"Keep it short. No greeting or sign-off unless one was spoken."},
		{Key: "email", Name: "Email", UseApp: true, UseSelection: true,
			Apps: []string{"Mail", "Microsoft Outlook", "Superhuman", "Spark"},
			Instructions: "Write an email from this dictation. Add a greeting and a sign-off if they are missing, " +
				"use short paragraphs, fix grammar and drop filler words. Keep the speaker's meaning and tone. " +
				"If selected text is given it is the email being replied to. Never invent facts, dates or commitments."},
		{Key: "note", Name: "Note",
			Apps: []string{"Notes", "Obsidian", "Notion", "Bear"},
			Instructions: "Turn this into concise Markdown notes: a short title line, then bullet points. " +
				"Keep every fact, name and number; drop filler."},
		{Key: "super", Name: "Super", UseApp: true, UseSelection: true, UseClipboard: true,
			Instructions: "Clean up the dictation: fix grammar and punctuation, drop filler and false starts, apply " +
				"spoken self-corrections (\"no wait, I mean ...\"), and format a list when the speaker lists things. " +
				"Use the context to spell names and terms correctly and to match the tone of where the text is going."},
		{Key: "prompt", Name: "Prompt", UseApp: true,
			Apps: []string{"Terminal", "iTerm2", "Ghostty", "WezTerm", "Code", "Cursor", "Zed"},
			Instructions: "Rewrite this as a clear instruction for an AI coding assistant: the goal first, then " +
				"constraints and details as bullet points. Keep code identifiers, file names, commands and technical " +
				"terms exactly as spoken."},
	}
}
