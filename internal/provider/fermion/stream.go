package fermion

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"chatter/internal/core"
)

var _ core.LivePreview = (*LivePreview)(nil)

// LivePreview streams to Phonon's /v1/audio/stream and reports finals plus the current partial.
type LivePreview struct {
	URL    string // e.g. ws://127.0.0.1:8000/v1/audio/stream
	APIKey string
}

type event struct {
	Type    string `json:"type"`
	Text    string `json:"text"`
	Message string `json:"message"`
}

func (l *LivePreview) Open(ctx context.Context, _ core.TranscribeOptions, onText func(string)) (core.LiveStream, error) {
	h := http.Header{}
	if l.APIKey != "" {
		h.Set("Authorization", "Bearer "+l.APIKey)
	}
	d := websocket.Dialer{HandshakeTimeout: 2 * time.Second}
	conn, _, err := d.DialContext(ctx, l.URL, h)
	if err != nil {
		return nil, err
	}
	if err := conn.WriteJSON(map[string]any{"sample_rate": core.SampleRate, "format": "pcm_f32le"}); err != nil {
		conn.Close()
		return nil, err
	}
	s := &stream{conn: conn, done: make(chan struct{})}
	go s.read(onText)
	return s, nil
}

type stream struct {
	conn *websocket.Conn
	mu   sync.Mutex // gorilla allows one concurrent writer
	done chan struct{}
	err  error
}

func (s *stream) read(onText func(string)) {
	defer close(s.done)
	var finals []string
	for {
		var ev event
		if err := s.conn.ReadJSON(&ev); err != nil {
			return
		}
		switch ev.Type {
		case "partial":
			onText(strings.TrimSpace(strings.Join(append(finals[:len(finals):len(finals)], ev.Text), " ")))
		case "final":
			finals = append(finals, ev.Text)
			onText(strings.Join(finals, " "))
		case "error":
			s.err = fmt.Errorf("stream: %s", ev.Message)
			return
		case "done":
			return
		}
	}
}

func (s *stream) Write(samples []float32) error {
	buf := make([]byte, 4*len(samples))
	for i, v := range samples {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-s.done:
		if s.err != nil {
			return s.err
		}
		return fmt.Errorf("stream closed")
	default:
	}
	return s.conn.WriteMessage(websocket.BinaryMessage, buf)
}

// Close ends the stream and waits briefly for the server to finish, so the next
// Dictation's stream is not refused as "engine busy".
func (s *stream) Close() error {
	s.mu.Lock()
	_ = s.conn.WriteJSON(map[string]string{"type": "end"})
	s.mu.Unlock()
	select {
	case <-s.done:
	case <-time.After(3 * time.Second):
	}
	return s.conn.Close()
}
