package fermion_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/websocket"

	"chatter/internal/core"
	"chatter/internal/provider/fermion"
	"chatter/pkg/testing/tier"
)

func TestLivePreview_ShowsFinalsThenCurrentPartial(t *testing.T) {
	tier.Set(t, tier.Unit)

	// given
	// ... a Phonon-like stream that answers the first audio frame with partial, final, partial, done
	var config map[string]any
	var gotBytes int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, _ := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		defer c.Close()
		_ = c.ReadJSON(&config)
		_, data, _ := c.ReadMessage()
		gotBytes = len(data)
		for _, ev := range []string{`{"type":"partial","text":"hello"}`, `{"type":"final","text":"Hello there.","segment":1}`,
			`{"type":"partial","text":"how are"}`} {
			_ = c.WriteMessage(websocket.TextMessage, []byte(ev))
		}
		_, _, _ = c.ReadMessage() // {"type":"end"}
		_ = c.WriteMessage(websocket.TextMessage, []byte(`{"type":"done","text":"Hello there. How are you?"}`))
	}))
	defer srv.Close()
	var mu sync.Mutex
	var previews []string
	lp := &fermion.LivePreview{URL: "ws" + strings.TrimPrefix(srv.URL, "http")}

	// when
	// ... 160 samples are streamed and the stream is closed
	s, err := lp.Open(context.Background(), core.TranscribeOptions{}, func(p string) {
		mu.Lock()
		previews = append(previews, p)
		mu.Unlock()
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Write(make([]float32, 160))
	_ = s.Close()

	// then
	// ... the config asked for 16 kHz f32, the audio went as 4-byte samples, and each preview is finals plus the partial
	mu.Lock()
	defer mu.Unlock()
	if config["format"] != "pcm_f32le" || config["sample_rate"] != float64(16000) || gotBytes != 640 {
		t.Fatalf("config %v bytes %d", config, gotBytes)
	}
	want := []string{"hello", "Hello there.", "Hello there. how are"}
	if strings.Join(previews, "|") != strings.Join(want, "|") {
		t.Fatalf("previews %q", previews)
	}
}
