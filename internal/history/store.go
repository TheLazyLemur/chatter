// Package history stores History entries as JSON beside their WAV audio.
package history

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"chatter/internal/core"
	"chatter/pkg/wav"
)

var _ core.History = (*Store)(nil)

type Store struct {
	Dir string
}

var validID = regexp.MustCompile(`^[0-9A-Za-z.\-]+$`)

func (s *Store) path(id, ext string) (string, error) {
	if !validID.MatchString(id) { // ids come back from the UI; never let one escape Dir
		return "", fmt.Errorf("history: invalid id %q", id)
	}
	return filepath.Join(s.Dir, id+ext), nil
}

func (s *Store) Save(e core.Entry, rec core.Recording) error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	jp, err := s.path(e.ID, ".json")
	if err != nil {
		return err
	}
	wp, _ := s.path(e.ID, ".wav")
	if err := os.WriteFile(wp, wav.Encode(rec.Samples, core.SampleRate), 0o600); err != nil {
		return err
	}
	data, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(jp, data, 0o600)
}

// List returns entries newest first whose Transcript or Output contains query (case-insensitive).
// shortcut: reads every entry's JSON per call; fine for thousands, index it if History grows past that.
func (s *Store) List(query string, limit int) ([]core.Entry, error) {
	files, err := filepath.Glob(filepath.Join(s.Dir, "*.json"))
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(query)
	var out []core.Entry
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var e core.Entry
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("history: %s: %w", filepath.Base(f), err)
		}
		if q == "" || strings.Contains(strings.ToLower(e.Transcript+"\n"+e.Output+"\n"+e.ModeName+"\n"+e.App.Name), q) {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *Store) Get(id string) (core.Entry, core.Recording, error) {
	jp, err := s.path(id, ".json")
	if err != nil {
		return core.Entry{}, core.Recording{}, err
	}
	wp, _ := s.path(id, ".wav")
	data, err := os.ReadFile(jp)
	if err != nil {
		return core.Entry{}, core.Recording{}, err
	}
	var e core.Entry
	if err := json.Unmarshal(data, &e); err != nil {
		return core.Entry{}, core.Recording{}, err
	}
	raw, err := os.ReadFile(wp)
	if err != nil {
		return e, core.Recording{}, fmt.Errorf("history: audio for %s: %w", id, err)
	}
	samples, rate, err := wav.Decode(raw)
	if err != nil {
		return e, core.Recording{}, err
	}
	if rate != core.SampleRate {
		return e, core.Recording{}, fmt.Errorf("history: %s is %d Hz, want %d", id, rate, core.SampleRate)
	}
	return e, core.Recording{Samples: samples}, nil
}

func (s *Store) Delete(id string) error {
	var errs []error
	for _, ext := range []string{".json", ".wav"} {
		p, err := s.path(id, ext)
		if err != nil {
			return err
		}
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
