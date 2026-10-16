package core

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
)

const levelHistory = 48

// Dictation runs the record → process → deliver cycle and keeps a Status for display.
type Dictation struct {
	processor *Processor
	mic       Microphone
	live      LivePreview
	reader    ContextReader
	out       Deliverer
	cues      Cues
	decoder   AudioDecoder
	clock     Clock

	mu       sync.Mutex
	settings Settings
	trigger  Trigger
	status   Status
	gen      int // bumped per Dictation so late mic and preview callbacks are dropped
	samples  []float32
	context  Context
	liveCh   chan []float32
	cancel   context.CancelFunc // cancels the in-flight processing
}

func NewDictation(p *Processor, mic Microphone, live LivePreview, reader ContextReader, out Deliverer, cues Cues, decoder AudioDecoder, clock Clock, s Settings) *Dictation {
	if p == nil || mic == nil || live == nil || reader == nil || out == nil || cues == nil || decoder == nil || clock == nil {
		panic("core: NewDictation needs every port; wire a No* adapter for disabled features")
	}
	if err := s.Validate(); err != nil {
		panic(err)
	}
	m, _ := s.ModeByKey(s.DefaultMode)
	return &Dictation{processor: p, mic: mic, live: live, reader: reader, out: out, cues: cues, decoder: decoder, clock: clock,
		settings: s, status: Status{Mode: m}}
}

func (d *Dictation) Settings() Settings {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.settings
}

func (d *Dictation) SetSettings(s Settings) error {
	if err := s.Validate(); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.settings = s
	if d.status.State == StateIdle {
		d.status.Mode, _ = s.ModeByKey(s.DefaultMode)
	}
	return nil
}

func (d *Dictation) Status() Status {
	d.mu.Lock()
	defer d.mu.Unlock()
	st := d.status
	st.Levels = append([]float32(nil), d.status.Levels...)
	return st
}

// PressRecord handles the record hotkey going down. modeKey "" lets the Target app pick the Mode.
func (d *Dictation) PressRecord(modeKey string) {
	d.mu.Lock()
	a := d.trigger.Down(d.clock.Now(), d.status.State == StateRecording)
	busy := d.status.State == StateProcessing
	if busy {
		d.trigger = Trigger{}
	}
	d.mu.Unlock()
	switch {
	case busy:
		d.Notify("Still processing the last dictation", true)
	case a == StartDictation:
		if err := d.Start(modeKey); err != nil {
			d.Notify(err.Error(), true)
		}
	case a == FinishDictation:
		go d.Finish(context.Background()) //nolint:errcheck // outcome lands in Status
	}
}

// ReleaseRecord handles the record hotkey coming up: it ends a Push-to-talk Dictation.
func (d *Dictation) ReleaseRecord() {
	d.mu.Lock()
	a := d.trigger.Up(d.clock.Now(), d.status.State == StateRecording, d.settings.HoldThreshold())
	d.mu.Unlock()
	if a == FinishDictation {
		go d.Finish(context.Background()) //nolint:errcheck // outcome lands in Status
	}
}

// Start begins a Dictation in modeKey, or in the Mode the Target app selects.
func (d *Dictation) Start(modeKey string) error {
	d.mu.Lock()
	if d.status.State != StateIdle {
		d.mu.Unlock()
		return ErrBusy
	}
	s := d.settings
	d.mu.Unlock()

	c, err := d.reader.Capture(false, false)
	if err != nil {
		return fmt.Errorf("read target app: %w", err)
	}
	m, err := SelectMode(s, modeKey, c.App)
	if err != nil {
		return err
	}
	if m.UseSelection || m.UseClipboard {
		if c, err = d.reader.Capture(m.UseSelection, m.UseClipboard); err != nil {
			return fmt.Errorf("read context: %w", err)
		}
	}

	d.mu.Lock()
	d.gen++
	gen := d.gen
	d.samples = d.samples[:0]
	d.context = c
	d.liveCh = make(chan []float32, 512)
	d.status = Status{State: StateRecording, Mode: m, Started: d.clock.Now(), Last: d.status.Last}
	liveCh := d.liveCh
	d.mu.Unlock()

	if err := d.mic.Start(s.Microphone, func(f []float32) { d.onFrames(gen, f) }); err != nil {
		d.mu.Lock()
		d.status.State = StateIdle
		close(d.liveCh)
		d.liveCh = nil
		d.mu.Unlock()
		return fmt.Errorf("microphone: %w", err)
	}
	go d.runLive(gen, liveCh, TranscribeOptions{Vocabulary: VocabularyFor(s, m)})
	if s.SoundCues {
		d.cues.Play(CueStart)
	}
	return nil
}

func (d *Dictation) onFrames(gen int, f []float32) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if gen != d.gen || d.status.State != StateRecording {
		return
	}
	d.samples = append(d.samples, f...)
	d.status.Levels = append(d.status.Levels, Level(f))
	if n := len(d.status.Levels); n > levelHistory {
		d.status.Levels = d.status.Levels[n-levelHistory:]
	}
	select {
	case d.liveCh <- f:
	default: // preview fell behind; it is provisional, so drop rather than stall capture
	}
}

func (d *Dictation) runLive(gen int, ch chan []float32, opts TranscribeOptions) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := d.live.Open(ctx, opts, func(text string) {
		d.mu.Lock()
		if gen == d.gen && d.status.State == StateRecording {
			d.status.Preview = text
		}
		d.mu.Unlock()
	})
	if err != nil {
		for range ch { // preview is optional: keep draining so capture never blocks
		}
		return
	}
	for f := range ch {
		if err := stream.Write(f); err != nil {
			break
		}
	}
	for range ch {
	}
	_ = stream.Close()
}

// stopCapture moves recording → processing and hands back what was captured.
func (d *Dictation) stopCapture(next State) (Recording, Mode, Context, Settings, bool) {
	d.mu.Lock()
	if d.status.State != StateRecording {
		d.mu.Unlock()
		return Recording{}, Mode{}, Context{}, Settings{}, false
	}
	d.status.State = next
	d.status.Preview = ""
	rec := Recording{Samples: append([]float32(nil), d.samples...)}
	m, c, s := d.status.Mode, d.context, d.settings
	close(d.liveCh)
	d.liveCh = nil
	d.mu.Unlock()
	// outside the lock: Stop waits for the capture goroutine, which takes the lock in onFrames
	_ = d.mic.Stop()
	return rec, m, c, s, true
}

// Finish ends the recording, processes it and delivers the Output to the Target app.
func (d *Dictation) Finish(ctx context.Context) (Entry, error) {
	rec, m, c, s, ok := d.stopCapture(StateProcessing)
	if !ok {
		return Entry{}, errors.New("not recording")
	}
	if s.SoundCues {
		d.cues.Play(CueStop)
	}
	if rec.Duration() < s.MinRecording() {
		d.finish(nil, "Too short, discarded", false)
		return Entry{}, ErrTooShort
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	d.mu.Lock()
	d.cancel = cancel
	d.mu.Unlock()

	e, err := d.processor.Process(ctx, rec, m, c, s, "microphone", d.setStage)
	if err != nil {
		d.fail(s, err)
		return e, err
	}
	d.setStage("Pasting")
	if err := d.out.Deliver(ctx, e.Output, c.App, DeliveryOptions{Paste: s.AutoPaste, RestoreClipboard: s.RestoreClipboard}); err != nil {
		d.fail(s, fmt.Errorf("deliver: %w (the text is in History)", err))
		return e, err
	}
	d.finish(&e, deliveredNotice(e, s.AutoPaste), e.Error != "")
	return e, nil
}

// Cancel abandons the current Dictation: a recording is discarded, processing is stopped.
func (d *Dictation) Cancel() {
	if _, _, _, s, ok := d.stopCapture(StateIdle); ok {
		if s.SoundCues {
			d.cues.Play(CueCancel)
		}
		d.Notify("Cancelled", false)
		return
	}
	d.mu.Lock()
	if d.cancel != nil {
		d.cancel()
	}
	d.mu.Unlock()
}

// TranscribeFile processes an audio file in a Mode and leaves the Output on the clipboard.
func (d *Dictation) TranscribeFile(ctx context.Context, path, modeKey string) (Entry, error) {
	d.mu.Lock()
	if d.status.State != StateIdle {
		d.mu.Unlock()
		return Entry{}, ErrBusy
	}
	s := d.settings
	m, err := SelectMode(s, modeKey, App{})
	if err != nil {
		d.mu.Unlock()
		return Entry{}, err
	}
	d.status = Status{State: StateProcessing, Mode: m, Stage: "Decoding", Started: d.clock.Now(), Last: d.status.Last}
	d.mu.Unlock()

	rec, err := d.decoder.Decode(ctx, path)
	if err != nil {
		d.fail(s, fmt.Errorf("decode %s: %w", path, err))
		return Entry{}, err
	}
	e, err := d.processor.Process(ctx, rec, m, Context{}, s, path, d.setStage)
	if err != nil {
		d.fail(s, err)
		return e, err
	}
	if err := d.out.Deliver(ctx, e.Output, App{}, DeliveryOptions{}); err != nil {
		d.fail(s, fmt.Errorf("copy: %w (the text is in History)", err))
		return e, err
	}
	d.finish(&e, fmt.Sprintf("Transcribed %s: copied %d words", shortName(path), wordCount(e.Output)), e.Error != "")
	return e, nil
}

// Reprocess runs a History entry through a Mode again and copies the new Output.
func (d *Dictation) Reprocess(ctx context.Context, id, modeKey string) (Entry, error) {
	d.mu.Lock()
	if d.status.State != StateIdle {
		d.mu.Unlock()
		return Entry{}, ErrBusy
	}
	s := d.settings
	m, err := SelectMode(s, modeKey, App{})
	if err != nil {
		d.mu.Unlock()
		return Entry{}, err
	}
	d.status = Status{State: StateProcessing, Mode: m, Started: d.clock.Now(), Last: d.status.Last}
	d.mu.Unlock()

	e, err := d.processor.Reprocess(ctx, id, m, s, d.setStage)
	if err != nil {
		d.fail(s, err)
		return e, err
	}
	if err := d.out.Deliver(ctx, e.Output, App{}, DeliveryOptions{}); err != nil {
		d.fail(s, fmt.Errorf("copy: %w (the text is in History)", err))
		return e, err
	}
	d.finish(&e, fmt.Sprintf("Re-ran in %s: copied %d words", m.Name, wordCount(e.Output)), e.Error != "")
	return e, nil
}

// PasteLast delivers the most recent Output into whatever app is frontmost now.
func (d *Dictation) PasteLast(ctx context.Context) error {
	st := d.Status()
	if st.Last == nil {
		return errors.New("nothing dictated yet")
	}
	c, err := d.reader.Capture(false, false)
	if err != nil {
		return err
	}
	s := d.Settings()
	return d.out.Deliver(ctx, st.Last.Output, c.App, DeliveryOptions{Paste: true, RestoreClipboard: s.RestoreClipboard})
}

// SetLast seeds the last entry, e.g. from History at startup.
func (d *Dictation) SetLast(e Entry) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.status.Last = &e
}

func (d *Dictation) setStage(stage string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.status.Stage = stage
}

func (d *Dictation) fail(s Settings, err error) {
	if s.SoundCues && !errors.Is(err, ErrNothingHeard) {
		d.cues.Play(CueError)
	}
	msg := err.Error()
	if errors.Is(err, ErrNothingHeard) {
		msg = "Nothing heard"
	}
	d.finish(nil, msg, !errors.Is(err, ErrNothingHeard))
}

func (d *Dictation) finish(e *Entry, notice string, isErr bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.status.State = StateIdle
	d.status.Stage = ""
	d.cancel = nil
	if e != nil {
		d.status.Last = e
	}
	d.status.Notice, d.status.IsError, d.status.At = notice, isErr, d.clock.Now()
	d.status.Mode, _ = d.settings.ModeByKey(d.settings.DefaultMode)
}

// Notify shows a one-line message in the Status, e.g. a mode switch.
func (d *Dictation) Notify(msg string, isErr bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.status.Notice, d.status.IsError, d.status.At = msg, isErr, d.clock.Now()
}

func deliveredNotice(e Entry, pasted bool) string {
	verb := "Copied"
	if pasted {
		verb = "Pasted"
	}
	n := fmt.Sprintf("%s %d words · %s", verb, wordCount(e.Output), e.ModeName)
	if e.Error != "" {
		n = fmt.Sprintf("%s raw transcript. %s", verb, e.Error)
	}
	return n
}

// Level is a frame's loudness on a 0..1 scale that suits a meter.
func Level(f []float32) float32 {
	if len(f) == 0 {
		return 0
	}
	var sum float64
	for _, v := range f {
		sum += float64(v) * float64(v)
	}
	rms := math.Sqrt(sum / float64(len(f)))
	// -50 dBFS → 0, 0 dBFS → 1
	db := 20 * math.Log10(rms+1e-9)
	return float32(math.Max(0, math.Min(1, (db+50)/50)))
}

func wordCount(s string) int { return len(strings.Fields(s)) }

func shortName(path string) string {
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		return path[i+1:]
	}
	return path
}
