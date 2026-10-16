package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrNothingHeard = errors.New("nothing heard")
	ErrTooShort     = errors.New("recording too short")
	ErrBusy         = errors.New("a dictation is already in progress")
)

// Processor turns a Recording into a History entry: transcribe, replace, rewrite, store.
type Processor struct {
	Transcriber Transcriber
	Rewriter    Rewriter
	History     History
	Clock       Clock
}

// Process runs the pipeline. A failed rewrite is not an error: the entry keeps the
// Transcript as its Output and records why in Entry.Error, so no words are lost.
func (p *Processor) Process(ctx context.Context, rec Recording, m Mode, c Context, s Settings, source string, stage func(string)) (Entry, error) {
	if len(rec.Samples) == 0 {
		return Entry{}, ErrTooShort
	}
	now := p.Clock.Now()
	e := Entry{
		ID:        now.UTC().Format("20060102T150405.000000"),
		CreatedAt: now,
		ModeKey:   m.Key,
		ModeName:  m.Name,
		App:       c.App,
		Duration:  rec.Duration(),
		Source:    source,
	}
	vocab := VocabularyFor(s, m)

	stage("Transcribing")
	text, err := p.Transcriber.Transcribe(ctx, rec, TranscribeOptions{Vocabulary: vocab})
	if err != nil {
		// keep the audio so the user can re-run it once the Transcriber is back
		e.Error = "transcribe: " + err.Error()
		if serr := p.History.Save(e, rec); serr != nil {
			return e, fmt.Errorf("transcribe: %w (and saving to history failed: %v)", err, serr)
		}
		return e, fmt.Errorf("transcribe: %w", err)
	}
	e.Transcript = ApplyReplacements(strings.TrimSpace(text), s.Replacements)
	if e.Transcript == "" {
		return e, ErrNothingHeard
	}
	e.Output = e.Transcript

	if strings.TrimSpace(m.Instructions) != "" {
		stage("Rewriting")
		out, err := p.Rewriter.Rewrite(ctx, BuildPrompt(m, e.Transcript, c, vocab))
		out = strings.TrimSpace(out)
		switch {
		case err != nil:
			e.Error = "rewrite: " + err.Error()
		case out == "":
			e.Error = "rewrite: empty reply"
		default:
			e.Output = out
		}
	}

	if err := p.History.Save(e, rec); err != nil {
		return e, fmt.Errorf("save history: %w", err)
	}
	return e, nil
}

// Reprocess runs a stored History entry's Recording through a Mode again, as a new entry.
func (p *Processor) Reprocess(ctx context.Context, id string, m Mode, s Settings, stage func(string)) (Entry, error) {
	old, rec, err := p.History.Get(id)
	if err != nil {
		return Entry{}, err
	}
	return p.Process(ctx, rec, m, Context{App: old.App}, s, old.Source, stage)
}
