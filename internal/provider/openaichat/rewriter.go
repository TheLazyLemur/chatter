// Package openaichat is a Rewriter for any OpenAI-compatible chat completions API:
// Ollama, LM Studio, fermion serve neutrino, OpenAI, OpenRouter, Groq.
package openaichat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"chatter/internal/core"
)

var _ core.Rewriter = (*Rewriter)(nil)

type Config struct {
	BaseURL string // e.g. http://127.0.0.1:11434/v1
	Model   string
	APIKey  string
}

type Rewriter struct {
	cfg    Config
	client *http.Client
}

func New(cfg Config, client *http.Client) *Rewriter {
	if cfg.BaseURL == "" || cfg.Model == "" {
		panic("openaichat: BaseURL and Model are required")
	}
	return &Rewriter{cfg: cfg, client: client}
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (r *Rewriter) Rewrite(ctx context.Context, p core.Prompt) (string, error) {
	body, err := json.Marshal(map[string]any{
		"model":       r.cfg.Model,
		"temperature": 0.2,
		"messages":    []message{{"system", p.System}, {"user", p.User}},
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(r.cfg.BaseURL, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if r.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+r.cfg.APIKey)
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s: %.300s", resp.Status, raw)
	}
	var out struct {
		Choices []struct {
			Message message `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || len(out.Choices) == 0 {
		return "", fmt.Errorf("unexpected response: %.300s", raw)
	}
	return stripThinking(out.Choices[0].Message.Content), nil
}

// stripThinking drops a reasoning model's <think> block, which is never part of the Output.
func stripThinking(s string) string {
	if i := strings.Index(s, "</think>"); i >= 0 && strings.HasPrefix(strings.TrimSpace(s), "<think>") {
		s = s[i+len("</think>"):]
	}
	return strings.TrimSpace(s)
}
