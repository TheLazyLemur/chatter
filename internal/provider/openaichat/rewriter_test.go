package openaichat_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"chatter/internal/core"
	"chatter/internal/provider/openaichat"
	"chatter/pkg/testing/tier"
)

func TestRewrite_SendsPromptAndDropsThinking(t *testing.T) {
	tier.Set(t, tier.Unit)

	// given
	// ... a reasoning model that wraps its answer in a think block
	var got struct {
		Model    string `json:"model"`
		Messages []struct{ Role, Content string }
	}
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"<think>hmm</think>\n\nHi Ada,"}}]}`))
	}))
	defer srv.Close()
	rw := openaichat.New(openaichat.Config{BaseURL: srv.URL + "/v1", Model: "qwen3", APIKey: "k"}, srv.Client())

	// when
	// ... a prompt is rewritten
	out, err := rw.Rewrite(context.Background(), core.Prompt{System: "sys", User: "usr"})

	// then
	// ... only the answer is returned, and the request carried model, roles and key
	if err != nil || out != "Hi Ada," {
		t.Fatalf("out %q err %v", out, err)
	}
	if got.Model != "qwen3" || got.Messages[0].Role != "system" || got.Messages[1].Content != "usr" || auth != "Bearer k" {
		t.Fatalf("request %+v auth %q", got, auth)
	}
}
