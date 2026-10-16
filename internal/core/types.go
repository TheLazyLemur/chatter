// Package core holds Chatter's domain types, rules and ports. It does no I/O.
package core

import (
	"time"
)

// SampleRate is the rate every Recording uses: what speech models expect.
const SampleRate = 16000

// Recording is mono float32 PCM at SampleRate.
type Recording struct {
	Samples []float32
}

func (r Recording) Duration() time.Duration {
	return time.Duration(len(r.Samples)) * time.Second / SampleRate
}

// Mode shapes a Transcript into Output. Empty Instructions means the Transcript is the Output.
type Mode struct {
	Key          string   `json:"key"`
	Name         string   `json:"name"`
	Instructions string   `json:"instructions"`
	UseSelection bool     `json:"use_selection"`
	UseClipboard bool     `json:"use_clipboard"`
	UseApp       bool     `json:"use_app"`
	Apps         []string `json:"apps"`       // Target app names or bundle IDs that auto-activate this Mode
	Vocabulary   []string `json:"vocabulary"` // added to the global Vocabulary for this Mode
	Hotkey       string   `json:"hotkey"`     // starts a Dictation in this Mode
}

type Replacement struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// App identifies the Target app.
type App struct {
	Name     string `json:"name"`
	BundleID string `json:"bundle_id"`
	PID      int    `json:"pid"`
}

// Context is what the user was looking at when the Dictation started.
type Context struct {
	App          App
	SelectedText string
	Clipboard    string
}

// Settings are the user's choices that the domain rules depend on.
type Settings struct {
	Modes            []Mode        `json:"modes"`
	DefaultMode      string        `json:"default_mode"`
	Vocabulary       []string      `json:"vocabulary"`
	Replacements     []Replacement `json:"replacements"`
	AutoPaste        bool          `json:"auto_paste"`
	RestoreClipboard bool          `json:"restore_clipboard"`
	SoundCues        bool          `json:"sound_cues"`
	Microphone       string        `json:"microphone"` // empty: system default
	HoldThresholdMS  int           `json:"hold_threshold_ms"`
	MinRecordingMS   int           `json:"min_recording_ms"`
}

func (s Settings) HoldThreshold() time.Duration {
	return time.Duration(s.HoldThresholdMS) * time.Millisecond
}

func (s Settings) MinRecording() time.Duration {
	return time.Duration(s.MinRecordingMS) * time.Millisecond
}

// Entry is a History entry.
type Entry struct {
	ID         string        `json:"id"`
	CreatedAt  time.Time     `json:"created_at"`
	ModeKey    string        `json:"mode_key"`
	ModeName   string        `json:"mode_name"`
	App        App           `json:"app"`
	Duration   time.Duration `json:"duration"`
	Transcript string        `json:"transcript"`
	Output     string        `json:"output"`
	Error      string        `json:"error,omitempty"` // a failed step; Output falls back to Transcript
	Source     string        `json:"source"`          // "microphone" or the file path
}

type State int

const (
	StateIdle State = iota
	StateRecording
	StateProcessing
)

func (s State) String() string {
	return [...]string{"idle", "recording", "processing"}[s]
}

// Status is a snapshot of the Dictation for display.
type Status struct {
	State   State
	Mode    Mode
	Started time.Time
	Levels  []float32 // recent input levels, 0..1, oldest first
	Preview string    // Live preview
	Stage   string    // what processing is doing now
	Last    *Entry
	Notice  string // one-line outcome of the last Dictation
	IsError bool
	At      time.Time // when Notice was set
}
