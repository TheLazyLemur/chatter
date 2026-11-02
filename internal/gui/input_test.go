package gui

import (
	"testing"

	"chatter/pkg/testing/tier"
)

func TestHit_OneClickActsOnOneControl(t *testing.T) {
	tier.Set(t, tier.Unit)

	// given
	// ... a click at (10, 10) and two controls laid out over the same spot, as when a clicked
	// button does not advance the next one's x
	in := input{mx: 10, my: 10, clicked: true}

	// when
	// ... both controls test for a hit, then the next frame starts
	first := in.hit(0, 0, 50, 30)
	second := in.hit(0, 0, 50, 30)
	in.reset()
	in.mx, in.my, in.clicked = 10, 10, true
	nextFrame := in.hit(0, 0, 50, 30)

	// then
	// ... only the first control gets the click, and the next frame's click is fresh
	if !first || second || !nextFrame {
		t.Fatalf("first %v, second %v, next frame %v", first, second, nextFrame)
	}
}
