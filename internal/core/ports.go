package core

import (
	"context"
	"errors"
	"time"
)

// TranscribeOptions carries the per-request hints a Transcriber may honour.
type TranscribeOptions struct {
	Vocabulary []string
}

// Transcriber is the speech-model seam. Swap the model by swapping the adapter.
type Transcriber interface {
	Transcribe(ctx context.Context, rec Recording, opts TranscribeOptions) (string, error)
}

// LivePreview streams audio while recording and reports provisional text.
type LivePreview interface {
	Open(ctx context.Context, opts TranscribeOptions, onText func(preview string)) (LiveStream, error)
}

type LiveStream interface {
	Write(samples []float32) error
	Close() error
}

// Prompt is what the Rewriter is asked; the core decides its content.
type Prompt struct {
	System string
	User   string
}

type Rewriter interface {
	Rewrite(ctx context.Context, p Prompt) (string, error)
}

// Microphone captures mono float32 at SampleRate and hands frames to onFrames from its own goroutine.
type Microphone interface {
	Start(device string, onFrames func([]float32)) error
	Stop() error
}

// ContextReader snapshots the Context. Selection and clipboard are read only when asked for.
type ContextReader interface {
	Capture(selection, clipboard bool) (Context, error)
}

type DeliveryOptions struct {
	Paste            bool
	RestoreClipboard bool
}

// Deliverer puts Output into the Target app.
type Deliverer interface {
	Deliver(ctx context.Context, text string, target App, opts DeliveryOptions) error
}

type History interface {
	Save(e Entry, rec Recording) error
	List(query string, limit int) ([]Entry, error)
	Get(id string) (Entry, Recording, error)
	Delete(id string) error
}

type Cue int

const (
	CueStart Cue = iota
	CueStop
	CueCancel
	CueError
)

type Cues interface {
	Play(c Cue)
}

// AudioDecoder turns an audio file into a Recording.
type AudioDecoder interface {
	Decode(ctx context.Context, path string) (Recording, error)
}

type Clock interface {
	Now() time.Time
}

// NoLivePreview is the LivePreview to wire when preview is switched off.
type NoLivePreview struct{}

func (NoLivePreview) Open(context.Context, TranscribeOptions, func(string)) (LiveStream, error) {
	return nil, ErrNoPreview
}

var ErrNoPreview = errors.New("live preview disabled")

// NoRewriter is the Rewriter to wire when none is configured. Modes with instructions then fail loudly.
type NoRewriter struct{}

func (NoRewriter) Rewrite(context.Context, Prompt) (string, error) {
	return "", errors.New("no rewriter configured: set rewriter.base_url in settings")
}

// NoCues plays nothing.
type NoCues struct{}

func (NoCues) Play(Cue) {}
