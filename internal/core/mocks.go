package core

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

// Port fakes shared by every test that needs one.

type FakeTranscriber struct {
	Text string
	Err  error
	Got  []TranscribeOptions
}

func (f *FakeTranscriber) Transcribe(_ context.Context, _ Recording, o TranscribeOptions) (string, error) {
	f.Got = append(f.Got, o)
	return f.Text, f.Err
}

type FakeRewriter struct {
	Out string
	Err error
	Got []Prompt
}

func (f *FakeRewriter) Rewrite(_ context.Context, p Prompt) (string, error) {
	f.Got = append(f.Got, p)
	return f.Out, f.Err
}

// FakeMic records the frame callback; Emit feeds it as a capture goroutine would.
type FakeMic struct {
	mu       sync.Mutex
	onFrames func([]float32)
	Device   string
	Running  bool
}

func (f *FakeMic) Start(device string, onFrames func([]float32)) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Device, f.onFrames, f.Running = device, onFrames, true
	return nil
}

func (f *FakeMic) Stop() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Running = false
	return nil
}

func (f *FakeMic) Emit(samples []float32) {
	f.mu.Lock()
	cb := f.onFrames
	f.mu.Unlock()
	cb(samples)
}

type FakeContextReader struct {
	Ctx Context
	Got [][2]bool
}

func (f *FakeContextReader) Capture(selection, clipboard bool) (Context, error) {
	f.Got = append(f.Got, [2]bool{selection, clipboard})
	c := f.Ctx
	if !selection {
		c.SelectedText = ""
	}
	if !clipboard {
		c.Clipboard = ""
	}
	return c, nil
}

type Delivered struct {
	Text   string
	Target App
	Opts   DeliveryOptions
}

type FakeDeliverer struct {
	Got []Delivered
	Err error
}

func (f *FakeDeliverer) Deliver(_ context.Context, text string, target App, o DeliveryOptions) error {
	f.Got = append(f.Got, Delivered{text, target, o})
	return f.Err
}

type MemoryHistory struct {
	mu      sync.Mutex
	Entries map[string]Entry
	Audio   map[string]Recording
}

func NewMemoryHistory() *MemoryHistory {
	return &MemoryHistory{Entries: map[string]Entry{}, Audio: map[string]Recording{}}
}

func (h *MemoryHistory) Save(e Entry, r Recording) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Entries[e.ID], h.Audio[e.ID] = e, r
	return nil
}

func (h *MemoryHistory) List(query string, limit int) ([]Entry, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []Entry
	for _, e := range h.Entries {
		if strings.Contains(strings.ToLower(e.Output+" "+e.Transcript), strings.ToLower(query)) {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (h *MemoryHistory) Get(id string) (Entry, Recording, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	e, ok := h.Entries[id]
	if !ok {
		return Entry{}, Recording{}, errors.New("not found")
	}
	return e, h.Audio[id], nil
}

func (h *MemoryHistory) Delete(id string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.Entries, id)
	delete(h.Audio, id)
	return nil
}

type FakeCues struct{ Played []Cue }

func (f *FakeCues) Play(c Cue) { f.Played = append(f.Played, c) }

type FakeDecoder struct {
	Rec Recording
	Err error
}

func (f FakeDecoder) Decode(context.Context, string) (Recording, error) { return f.Rec, f.Err }

// FakeClock returns Now until moved with Advance.
type FakeClock struct {
	mu sync.Mutex
	T  time.Time
}

func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.T
}

func (c *FakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.T = c.T.Add(d)
}
