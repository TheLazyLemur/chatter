package fermion_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"chatter/internal/core"
	"chatter/internal/provider/fermion"
	"chatter/pkg/testing/tier"
)

type stubTranscriber struct{}

func (stubTranscriber) Transcribe(context.Context, core.Recording, core.TranscribeOptions) (string, error) {
	return "hello", nil
}

func TestSupervised_WaitsForServerStillStarting(t *testing.T) {
	tier.Set(t, tier.Unit)

	// given
	// ... a server that answers health checks only from the third probe, as while Phonon loads
	var probes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if probes.Add(1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer srv.Close()
	server := &fermion.Server{Command: []string{"sleep", "5"}, HealthURL: srv.URL}
	defer server.Stop()
	tr := &fermion.Supervised{Server: server, Transcriber: stubTranscriber{}}

	// when
	// ... a recording is transcribed before the server is healthy
	text, err := tr.Transcribe(context.Background(), core.Recording{}, core.TranscribeOptions{})

	// then
	// ... it waits for the server instead of failing
	if err != nil || text != "hello" {
		t.Fatalf("text %q err %v", text, err)
	}
}
