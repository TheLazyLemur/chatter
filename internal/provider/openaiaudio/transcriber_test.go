package openaiaudio_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"chatter/internal/core"
	"chatter/internal/provider/openaiaudio"
	"chatter/pkg/testing/tier"
	"chatter/pkg/wav"
)

func TestTranscribe_SendsWavAndVocabulary(t *testing.T) {
	tier.Set(t, tier.Unit)

	// given
	// ... a server that records the multipart form and answers like fermion serve
	var form map[string][]string
	var rate, samples int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(1 << 20)
		form = r.MultipartForm.Value
		f, _, _ := r.FormFile("file")
		buf := make([]byte, 1<<20)
		n, _ := f.Read(buf)
		s, rt, _ := wav.Decode(buf[:n])
		rate, samples = rt, len(s)
		_, _ = w.Write([]byte(`{"text": "Hello Quillon."}`))
	}))
	defer srv.Close()
	tr := openaiaudio.New(openaiaudio.Config{BaseURL: srv.URL + "/v1", Model: "phonon-2"}, srv.Client())

	// when
	// ... half a second is transcribed with two vocabulary terms
	text, err := tr.Transcribe(context.Background(), core.Recording{Samples: make([]float32, 8000)},
		core.TranscribeOptions{Vocabulary: []string{"Ada Lovelace", "Quillon"}})

	// then
	// ... the text comes back and the request carried the model, a 16 kHz wav and a comma list prompt
	if err != nil || text != "Hello Quillon." {
		t.Fatalf("text %q err %v", text, err)
	}
	if form["model"][0] != "phonon-2" || form["prompt"][0] != "Ada Lovelace, Quillon" || rate != 16000 || samples != 8000 {
		t.Fatalf("form %v rate %d samples %d", form, rate, samples)
	}
}

func TestTranscribe_SurfacesServerError(t *testing.T) {
	tier.Set(t, tier.Unit)

	// given
	// ... a server that rejects the model with the OpenAI error envelope
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error": {"message": "model whisper-1 is not served here", "code": "model_not_found"}}`))
	}))
	defer srv.Close()
	tr := openaiaudio.New(openaiaudio.Config{BaseURL: srv.URL + "/v1", Model: "whisper-1"}, srv.Client())

	// when
	// ... a recording is transcribed
	_, err := tr.Transcribe(context.Background(), core.Recording{Samples: make([]float32, 160)}, core.TranscribeOptions{})

	// then
	// ... the error carries the server's message
	if err == nil || !strings.Contains(err.Error(), "not served here") {
		t.Fatalf("err %v", err)
	}
}
