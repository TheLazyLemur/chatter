// Package audiofile decodes any audio or video file into a Recording with ffmpeg.
package audiofile

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"os/exec"
	"strconv"

	"chatter/internal/core"
)

var _ core.AudioDecoder = (*FFmpeg)(nil)

type FFmpeg struct {
	Path string // ffmpeg binary; "ffmpeg" uses PATH
}

func (f *FFmpeg) Decode(ctx context.Context, path string) (core.Recording, error) {
	bin := f.Path
	if bin == "" {
		bin = "ffmpeg"
	}
	cmd := exec.CommandContext(ctx, bin, "-nostdin", "-loglevel", "error", "-i", path,
		"-f", "f32le", "-ac", "1", "-ar", strconv.Itoa(core.SampleRate), "-")
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		return core.Recording{}, fmt.Errorf("ffmpeg: %w: %s", err, bytes.TrimSpace(stderr.Bytes()))
	}
	raw := out.Bytes()
	samples := make([]float32, len(raw)/4)
	for i := range samples {
		samples[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[i*4:]))
	}
	return core.Recording{Samples: samples}, nil
}
