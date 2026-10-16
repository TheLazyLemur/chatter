// Package sdlaudio captures the microphone and plays sound through SDL2.
// The caller owns sdl.Init(sdl.INIT_AUDIO).
package sdlaudio

import (
	"encoding/binary"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/veandco/go-sdl2/sdl"

	"chatter/internal/core"
)

var (
	_ core.Microphone = (*Microphone)(nil)
	_ core.Cues       = (*Speaker)(nil)
)

func spec() sdl.AudioSpec {
	return sdl.AudioSpec{Freq: core.SampleRate, Format: sdl.AUDIO_F32SYS, Channels: 1, Samples: 512}
}

// Microphone polls an SDL capture device; SDL converts whatever the hardware gives to 16 kHz mono f32.
type Microphone struct {
	mu   sync.Mutex
	dev  sdl.AudioDeviceID
	stop chan struct{}
	done chan struct{}
}

func (m *Microphone) Start(device string, onFrames func([]float32)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.dev != 0 {
		return fmt.Errorf("microphone already open")
	}
	want := spec()
	dev, err := sdl.OpenAudioDevice(device, true, &want, nil, 0)
	if err != nil {
		return fmt.Errorf("open %q: %w", deviceLabel(device), err)
	}
	m.dev, m.stop, m.done = dev, make(chan struct{}), make(chan struct{})
	sdl.PauseAudioDevice(dev, false)
	go m.poll(dev, onFrames, m.stop, m.done)
	return nil
}

func (m *Microphone) poll(dev sdl.AudioDeviceID, onFrames func([]float32), stop, done chan struct{}) {
	defer close(done)
	t := time.NewTicker(20 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-stop:
			drain(dev, onFrames)
			return
		case <-t.C:
			drain(dev, onFrames)
		}
	}
}

func drain(dev sdl.AudioDeviceID, onFrames func([]float32)) {
	n := sdl.GetQueuedAudioSize(dev)
	if n < 4 {
		return
	}
	buf := make([]byte, n-n%4)
	got, err := sdl.DequeueAudio(dev, buf)
	if err != nil || got < 4 {
		return
	}
	f := make([]float32, got/4)
	for i := range f {
		f[i] = math.Float32frombits(binary.NativeEndian.Uint32(buf[i*4:]))
	}
	onFrames(f)
}

func (m *Microphone) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.dev == 0 {
		return nil
	}
	close(m.stop)
	<-m.done
	sdl.CloseAudioDevice(m.dev)
	m.dev = 0
	return nil
}

// Devices lists capture devices by name; "" means the system default.
func Devices() []string {
	var out []string
	for i := 0; i < sdl.GetNumAudioDevices(true); i++ {
		out = append(out, sdl.GetAudioDeviceName(i, true))
	}
	return out
}

func deviceLabel(d string) string {
	if d == "" {
		return "default microphone"
	}
	return d
}

// Speaker plays sound cues and History audio on the default output.
type Speaker struct {
	mu  sync.Mutex
	dev sdl.AudioDeviceID
}

func OpenSpeaker() (*Speaker, error) {
	want := spec()
	dev, err := sdl.OpenAudioDevice("", false, &want, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("open speaker: %w", err)
	}
	sdl.PauseAudioDevice(dev, false)
	return &Speaker{dev: dev}, nil
}

func (s *Speaker) Close() { sdl.CloseAudioDevice(s.dev) }

// PlaySamples replaces whatever is playing with samples.
func (s *Speaker) PlaySamples(samples []float32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sdl.ClearQueuedAudio(s.dev)
	buf := make([]byte, 4*len(samples))
	for i, v := range samples {
		binary.NativeEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	_ = sdl.QueueAudio(s.dev, buf)
}

func (s *Speaker) StopPlayback() {
	s.mu.Lock()
	defer s.mu.Unlock()
	sdl.ClearQueuedAudio(s.dev)
}

func (s *Speaker) Play(c core.Cue) {
	var tones [][2]float64 // frequency Hz, duration s
	switch c {
	case core.CueStart:
		tones = [][2]float64{{660, 0.06}, {990, 0.08}}
	case core.CueStop:
		tones = [][2]float64{{990, 0.06}, {660, 0.08}}
	case core.CueCancel:
		tones = [][2]float64{{440, 0.1}}
	case core.CueError:
		tones = [][2]float64{{220, 0.12}, {0, 0.05}, {220, 0.12}}
	}
	s.PlaySamples(synth(tones))
}

// synth renders sine tones with a short fade at each end so they do not click.
func synth(tones [][2]float64) []float32 {
	var out []float32
	for _, t := range tones {
		n := int(t[1] * core.SampleRate)
		fade := n / 5
		for i := 0; i < n; i++ {
			env := 1.0
			if i < fade {
				env = float64(i) / float64(fade)
			} else if i > n-fade {
				env = float64(n-i) / float64(fade)
			}
			out = append(out, float32(0.18*env*math.Sin(2*math.Pi*t[0]*float64(i)/core.SampleRate)))
		}
	}
	return out
}
