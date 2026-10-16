// Package fermion holds the adapters specific to Fermion Research's Phonon runtime:
// a Transcriber that runs the `fermion` CLI, the server supervisor, and Live preview over
// Phonon's WebSocket stream.
package fermion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"chatter/internal/core"
	"chatter/pkg/wav"
)

var _ core.Transcriber = (*CLITranscriber)(nil)

// CLITranscriber runs `fermion transcribe <model> <file> --json` per Recording. No server
// needed, but the model loads on every call (a second or two on Apple silicon).
type CLITranscriber struct {
	Command []string // e.g. ["fermion", "transcribe", "phonon-2"]
}

func (c *CLITranscriber) Transcribe(ctx context.Context, rec core.Recording, opts core.TranscribeOptions) (string, error) {
	if len(c.Command) == 0 {
		panic("fermion: CLITranscriber.Command is empty")
	}
	f, err := os.CreateTemp("", "chatter-*.wav")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(wav.Encode(rec.Samples, core.SampleRate)); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}

	args := append(append([]string{}, c.Command[1:]...), f.Name(), "--json")
	if len(opts.Vocabulary) > 0 {
		args = append(args, "--hotwords", strings.Join(opts.Vocabulary, ", "))
	}
	cmd := exec.CommandContext(ctx, c.Command[0], args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s: %w: %s", c.Command[0], err, lastLine(stderr.String()))
	}
	var out struct {
		Text *string `json:"text"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil || out.Text == nil {
		return "", fmt.Errorf("unexpected fermion output: %.200s", stdout.String())
	}
	return *out.Text, nil
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}
