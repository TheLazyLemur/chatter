package fermion

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// Server starts `fermion serve` when nothing answers at HealthURL, and stops what it started.
type Server struct {
	Command   []string // e.g. ["fermion", "serve", "phonon-2", "--port", "8000"]
	HealthURL string   // e.g. http://127.0.0.1:8000/health
	Log       io.Writer

	mu     sync.Mutex
	cmd    *exec.Cmd
	exited chan struct{}
}

// EnsureRunning returns once the server answers, starting it if needed. The first start
// downloads the model and compiles shaders, so allow minutes, not seconds.
func (s *Server) EnsureRunning(ctx context.Context, wait time.Duration) error {
	if s.healthy(ctx) {
		return nil
	}
	if len(s.Command) == 0 {
		return fmt.Errorf("no transcription server at %s and no autostart command configured", s.HealthURL)
	}
	s.mu.Lock()
	if s.cmd == nil {
		cmd := exec.Command(s.Command[0], s.Command[1:]...)
		cmd.Stdout, cmd.Stderr = s.Log, s.Log
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := cmd.Start(); err != nil {
			s.mu.Unlock()
			return fmt.Errorf("start %s: %w", s.Command[0], err)
		}
		s.cmd, s.exited = cmd, make(chan struct{})
		go func(exited chan struct{}) { _ = cmd.Wait(); close(exited) }(s.exited)
	}
	s.mu.Unlock()

	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		if s.healthy(ctx) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return errors.New("transcription server did not become healthy in time")
}

func (s *Server) healthy(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.HealthURL, nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// Stop interrupts the server this Server started, if any.
func (s *Server) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cmd == nil || s.cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGINT)
	select {
	case <-s.exited:
	case <-time.After(3 * time.Second):
		_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL)
	}
	s.cmd = nil
}
