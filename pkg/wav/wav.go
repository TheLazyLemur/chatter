// Package wav reads and writes 16-bit PCM mono WAV.
package wav

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

// Encode writes samples (-1..1) as 16-bit mono PCM WAV at rate.
func Encode(samples []float32, rate int) []byte {
	var b bytes.Buffer
	n := len(samples) * 2
	b.WriteString("RIFF")
	_ = binary.Write(&b, binary.LittleEndian, uint32(36+n))
	b.WriteString("WAVEfmt ")
	for _, v := range []any{uint32(16), uint16(1), uint16(1), uint32(rate), uint32(rate * 2), uint16(2), uint16(16)} {
		_ = binary.Write(&b, binary.LittleEndian, v)
	}
	b.WriteString("data")
	_ = binary.Write(&b, binary.LittleEndian, uint32(n))
	pcm := make([]int16, len(samples))
	for i, s := range samples {
		pcm[i] = int16(math.Round(float64(max(-1, min(1, s))) * math.MaxInt16))
	}
	_ = binary.Write(&b, binary.LittleEndian, pcm)
	return b.Bytes()
}

// Decode reads a 16-bit PCM WAV written by Encode (mono or the first channel of many).
func Decode(data []byte) (samples []float32, rate int, err error) {
	r := bytes.NewReader(data)
	var hdr [12]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil || string(hdr[:4]) != "RIFF" || string(hdr[8:]) != "WAVE" {
		return nil, 0, errors.New("wav: not a RIFF/WAVE file")
	}
	var channels, bits uint16
	for {
		var id [4]byte
		var size uint32
		if _, err := io.ReadFull(r, id[:]); err != nil {
			return nil, 0, errors.New("wav: no data chunk")
		}
		if err := binary.Read(r, binary.LittleEndian, &size); err != nil {
			return nil, 0, err
		}
		chunk := make([]byte, size+size%2)
		if _, err := io.ReadFull(r, chunk); err != nil && string(id[:]) != "data" {
			return nil, 0, err
		}
		chunk = chunk[:size]
		switch string(id[:]) {
		case "fmt ":
			if size < 16 || binary.LittleEndian.Uint16(chunk[0:]) != 1 {
				return nil, 0, errors.New("wav: only PCM is supported")
			}
			channels = binary.LittleEndian.Uint16(chunk[2:])
			rate = int(binary.LittleEndian.Uint32(chunk[4:]))
			bits = binary.LittleEndian.Uint16(chunk[14:])
		case "data":
			if bits != 16 || channels == 0 {
				return nil, 0, fmt.Errorf("wav: want 16-bit PCM, got %d-bit, %d channels", bits, channels)
			}
			frame := int(channels) * 2
			samples = make([]float32, len(chunk)/frame)
			for i := range samples {
				samples[i] = float32(int16(binary.LittleEndian.Uint16(chunk[i*frame:]))) / math.MaxInt16
			}
			return samples, rate, nil
		}
	}
}
