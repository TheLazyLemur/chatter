package wav_test

import (
	"math"
	"testing"

	"chatter/pkg/testing/tier"
	"chatter/pkg/wav"
)

func TestEncodeDecode_RoundTrips(t *testing.T) {
	tier.Set(t, tier.Unit)

	// given
	// ... samples spanning the full range, including out-of-range values that must clip
	in := []float32{0, 0.5, -0.5, 1, -1, 2}

	// when
	// ... they are encoded and decoded
	out, rate, err := wav.Decode(wav.Encode(in, 16000))

	// then
	// ... the rate and samples survive within 16-bit precision
	if err != nil || rate != 16000 || len(out) != len(in) {
		t.Fatalf("err %v rate %d len %d", err, rate, len(out))
	}
	want := []float32{0, 0.5, -0.5, 1, -1, 1}
	for i := range want {
		if math.Abs(float64(out[i]-want[i])) > 1e-4 {
			t.Fatalf("sample %d: %v, want %v", i, out[i], want[i])
		}
	}
}
