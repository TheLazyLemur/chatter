package hotkey

import (
	"testing"

	"chatter/pkg/testing/tier"
)

func TestParse_ReadsModifiersAndKey(t *testing.T) {
	tier.Set(t, tier.Unit)

	// given
	// ... a combo with mixed case, spaces and alias names, and one with two keys
	good, bad := "Ctrl + Alt + M", "a+b"

	// when
	// ... both are parsed
	c, err := Parse(good)
	_, badErr := Parse(bad)

	// then
	// ... the first maps to kVK_ANSI_M with control and option; the second is refused
	if err != nil || c != (Combo{Key: 0x2E, Mods: controlKey | optionKey}) {
		t.Fatalf("combo %+v err %v", c, err)
	}
	if badErr == nil {
		t.Fatal("two keys accepted")
	}
}
