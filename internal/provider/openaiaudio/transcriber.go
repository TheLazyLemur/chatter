// Package openaiaudio is a Transcriber for any server with OpenAI's
// /v1/audio/transcriptions shape: fermion serve (Phonon), OpenAI, Groq, whisper.cpp, speaches.
package openaiaudio

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"chatter/internal/core"
	"chatter/pkg/wav"
)

var _ core.Transcriber = (*Transcriber)(nil)

type Config struct {
	BaseURL string // e.g. http://127.0.0.1:8000/v1
	Model   string
	APIKey  string
}

type Transcriber struct {
	cfg    Config
	client *http.Client
}

func New(cfg Config, client *http.Client) *Transcriber {
	if cfg.BaseURL == "" {
		panic("openaiaudio: BaseURL is required")
	}
	return &Transcriber{cfg: cfg, client: client}
}

func (t *Transcriber) Transcribe(ctx context.Context, rec core.Recording, opts core.TranscribeOptions) (string, error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", "dictation.wav")
	if err != nil {
		return "", err
	}
	if _, err := fw.Write(wav.Encode(rec.Samples, core.SampleRate)); err != nil {
		return "", err
	}
	fields := map[string]string{"model": t.cfg.Model, "response_format": "json"}
	if len(opts.Vocabulary) > 0 {
		// OpenAI and Whisper read prompt as spelling hints; Phonon reads a comma list as hotwords
		fields["prompt"] = strings.Join(opts.Vocabulary, ", ")
	}
	for k, v := range fields {
		if v != "" {
			if err := mw.WriteField(k, v); err != nil {
				return "", err
			}
		}
	}
	if err := mw.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(t.cfg.BaseURL, "/")+"/audio/transcriptions", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if t.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+t.cfg.APIKey)
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s: %s", resp.Status, errorMessage(raw))
	}
	var out struct {
		Text *string `json:"text"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.Text == nil {
		return "", fmt.Errorf("unexpected response: %.200s", raw)
	}
	return *out.Text, nil
}

// Health reports whether the server answers at all.
func (t *Transcriber) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(t.cfg.BaseURL, "/")+"/models", nil)
	if err != nil {
		return err
	}
	if t.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+t.cfg.APIKey)
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s", resp.Status)
	}
	return nil
}

func errorMessage(raw []byte) string {
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &e) == nil && e.Error.Message != "" {
		return e.Error.Message
	}
	return fmt.Sprintf("%.200s", raw)
}
