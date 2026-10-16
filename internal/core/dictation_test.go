package core_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"chatter/internal/core"
	"chatter/pkg/testing/tier"
)

type rig struct {
	d    *core.Dictation
	mic  *core.FakeMic
	tr   *core.FakeTranscriber
	rw   *core.FakeRewriter
	out  *core.FakeDeliverer
	hist *core.MemoryHistory
	ctx  *core.FakeContextReader
}

func settings() core.Settings {
	return core.Settings{
		Modes: []core.Mode{
			{Key: "voice", Name: "Voice"},
			{Key: "email", Name: "Email", Instructions: "Write an email.", UseApp: true, UseSelection: true,
				Apps: []string{"com.apple.mail"}, Vocabulary: []string{"Quillon"}},
		},
		DefaultMode:      core.AutoMode,
		Vocabulary:       []string{"Ada"},
		AutoPaste:        true,
		RestoreClipboard: true,
		MinRecordingMS:   250,
		HoldThresholdMS:  400,
	}
}

func newRig(s core.Settings) rig {
	r := rig{
		mic:  &core.FakeMic{},
		tr:   &core.FakeTranscriber{Text: "hello ada"},
		rw:   &core.FakeRewriter{Out: "Hi Ada,"},
		out:  &core.FakeDeliverer{},
		hist: core.NewMemoryHistory(),
		ctx: &core.FakeContextReader{Ctx: core.Context{
			App: core.App{Name: "Mail", BundleID: "com.apple.mail", PID: 42}, SelectedText: "Dear team", Clipboard: "secret"}},
	}
	clock := &core.FakeClock{T: time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)}
	p := &core.Processor{Transcriber: r.tr, Rewriter: r.rw, History: r.hist, Clock: clock}
	r.d = core.NewDictation(p, r.mic, core.NoLivePreview{}, r.ctx, r.out, &core.FakeCues{}, core.FakeDecoder{}, clock, s)
	return r
}

func second() []float32 { return make([]float32, core.SampleRate) }

func TestDictation_DeliversRewrittenOutputToTargetApp(t *testing.T) {
	tier.Set(t, tier.Unit)

	// given
	// ... Mail is frontmost, which auto-selects the Email mode, and a second of audio is captured
	r := newRig(settings())
	if err := r.d.Start(""); err != nil {
		t.Fatal(err)
	}
	r.mic.Emit(second())

	// when
	// ... the dictation is finished
	e, err := r.d.Finish(context.Background())

	// then
	// ... the rewrite is pasted into Mail and stored, with the mode's vocabulary and context used
	if err != nil {
		t.Fatal(err)
	}
	want := []core.Delivered{{Text: "Hi Ada,", Target: r.ctx.Ctx.App, Opts: core.DeliveryOptions{Paste: true, RestoreClipboard: true}}}
	if !reflect.DeepEqual(r.out.Got, want) {
		t.Fatalf("delivered %+v, want %+v", r.out.Got, want)
	}
	if got := r.tr.Got[0].Vocabulary; !reflect.DeepEqual(got, []string{"Ada", "Quillon"}) {
		t.Fatalf("vocabulary %v", got)
	}
	sys := r.rw.Got[0].System
	if !strings.Contains(sys, "Mail") || !strings.Contains(sys, "Dear team") || strings.Contains(sys, "secret") {
		t.Fatalf("prompt context wrong:\n%s", sys)
	}
	if _, _, err := r.hist.Get(e.ID); err != nil || e.Output != "Hi Ada," || e.Transcript != "hello ada" {
		t.Fatalf("history entry %+v, err %v", e, err)
	}
	if r.mic.Running {
		t.Fatal("microphone left running")
	}
}

func TestDictation_FailedRewriteDeliversTranscript(t *testing.T) {
	tier.Set(t, tier.Unit)

	// given
	// ... an Email dictation whose rewriter is down
	r := newRig(settings())
	r.rw.Err = errors.New("connection refused")
	_ = r.d.Start("email")
	r.mic.Emit(second())

	// when
	// ... the dictation is finished
	e, err := r.d.Finish(context.Background())

	// then
	// ... the transcript is pasted, and the failure is recorded and shown as an error
	if err != nil {
		t.Fatal(err)
	}
	if r.out.Got[0].Text != "hello ada" || !strings.Contains(e.Error, "connection refused") {
		t.Fatalf("delivered %q, entry error %q", r.out.Got[0].Text, e.Error)
	}
	if st := r.d.Status(); !st.IsError || st.State != core.StateIdle {
		t.Fatalf("status %+v", st)
	}
}

func TestDictation_FailedTranscriptionKeepsAudio(t *testing.T) {
	tier.Set(t, tier.Unit)

	// given
	// ... a dictation whose transcriber is down
	r := newRig(settings())
	r.tr.Err = errors.New("server down")
	_ = r.d.Start("")
	r.mic.Emit(second())

	// when
	// ... the dictation is finished
	e, err := r.d.Finish(context.Background())

	// then
	// ... nothing is delivered, but the recording is in history for a re-run
	if err == nil || len(r.out.Got) != 0 {
		t.Fatalf("err %v, delivered %+v", err, r.out.Got)
	}
	if _, rec, gerr := r.hist.Get(e.ID); gerr != nil || len(rec.Samples) != core.SampleRate {
		t.Fatalf("audio not kept: %v", gerr)
	}
}

func TestDictation_TooShortIsDiscarded(t *testing.T) {
	tier.Set(t, tier.Unit)

	// given
	// ... a dictation with 100 ms of audio, under the 250 ms minimum
	r := newRig(settings())
	_ = r.d.Start("")
	r.mic.Emit(make([]float32, core.SampleRate/10))

	// when
	// ... the dictation is finished
	_, err := r.d.Finish(context.Background())

	// then
	// ... it is discarded without transcribing
	if !errors.Is(err, core.ErrTooShort) || len(r.tr.Got) != 0 {
		t.Fatalf("err %v, transcribed %d times", err, len(r.tr.Got))
	}
}

func TestDictation_CancelDiscardsRecording(t *testing.T) {
	tier.Set(t, tier.Unit)

	// given
	// ... a dictation in progress
	r := newRig(settings())
	_ = r.d.Start("")
	r.mic.Emit(second())

	// when
	// ... it is cancelled
	r.d.Cancel()

	// then
	// ... capture stops, nothing is transcribed, and a new dictation can start
	if r.mic.Running || len(r.tr.Got) != 0 {
		t.Fatal("cancel did not discard")
	}
	if err := r.d.Start(""); err != nil {
		t.Fatalf("restart: %v", err)
	}
}

func TestDictation_ReprocessRunsStoredAudioInAnotherMode(t *testing.T) {
	tier.Set(t, tier.Unit)

	// given
	// ... a Voice dictation in history
	r := newRig(settings())
	r.ctx.Ctx.App = core.App{Name: "Notes"}
	_ = r.d.Start("")
	r.mic.Emit(second())
	first, _ := r.d.Finish(context.Background())

	// when
	// ... it is re-run in Email mode
	e, err := r.d.Reprocess(context.Background(), first.ID, "email")

	// then
	// ... the rewrite is copied, not pasted, and stored as its own entry
	if err != nil || e.Output != "Hi Ada," || e.ModeKey != "email" {
		t.Fatalf("entry %+v, err %v", e, err)
	}
	if last := r.out.Got[len(r.out.Got)-1]; last.Opts.Paste {
		t.Fatal("re-run pasted into an app")
	}
}
