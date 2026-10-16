// Package hotkey registers system-wide hotkeys (macOS Carbon) and reports press and release.
package hotkey

import (
	"fmt"
	"strings"
)

// Carbon modifier masks.
const (
	cmdKey     = 0x0100
	shiftKey   = 0x0200
	optionKey  = 0x0800
	controlKey = 0x1000
)

// Combo is a parsed hotkey: a macOS virtual key code plus Carbon modifier mask.
type Combo struct {
	Key  uint32
	Mods uint32
}

var modifiers = map[string]uint32{
	"cmd": cmdKey, "command": cmdKey, "shift": shiftKey,
	"option": optionKey, "opt": optionKey, "alt": optionKey,
	"control": controlKey, "ctrl": controlKey,
}

// virtual key codes (kVK_*) from Carbon's Events.h
var keys = map[string]uint32{
	"a": 0x00, "s": 0x01, "d": 0x02, "f": 0x03, "h": 0x04, "g": 0x05, "z": 0x06, "x": 0x07, "c": 0x08, "v": 0x09,
	"b": 0x0B, "q": 0x0C, "w": 0x0D, "e": 0x0E, "r": 0x0F, "y": 0x10, "t": 0x11, "1": 0x12, "2": 0x13, "3": 0x14,
	"4": 0x15, "6": 0x16, "5": 0x17, "=": 0x18, "9": 0x19, "7": 0x1A, "-": 0x1B, "8": 0x1C, "0": 0x1D, "]": 0x1E,
	"o": 0x1F, "u": 0x20, "[": 0x21, "i": 0x22, "p": 0x23, "l": 0x25, "j": 0x26, "'": 0x27, "k": 0x28, ";": 0x29,
	"\\": 0x2A, ",": 0x2B, "/": 0x2C, "n": 0x2D, "m": 0x2E, ".": 0x2F, "`": 0x32,
	"return": 0x24, "enter": 0x24, "tab": 0x30, "space": 0x31, "delete": 0x33, "escape": 0x35, "esc": 0x35,
	"f1": 0x7A, "f2": 0x78, "f3": 0x63, "f4": 0x76, "f5": 0x60, "f6": 0x61, "f7": 0x62, "f8": 0x64,
	"f9": 0x65, "f10": 0x6D, "f11": 0x67, "f12": 0x6F, "f13": 0x69, "f14": 0x6B, "f15": 0x71,
	"f16": 0x6A, "f17": 0x40, "f18": 0x4F, "f19": 0x50, "f20": 0x5A,
	"left": 0x7B, "right": 0x7C, "down": 0x7D, "up": 0x7E, "home": 0x73, "end": 0x77,
	"pageup": 0x74, "pagedown": 0x79,
}

// Parse reads "control+option+m": any modifiers, then exactly one key, joined with "+".
func Parse(s string) (Combo, error) {
	var c Combo
	parts := strings.Split(strings.ToLower(strings.ReplaceAll(s, " ", "")), "+")
	for i, p := range parts {
		if m, ok := modifiers[p]; ok && i < len(parts)-1 {
			c.Mods |= m
			continue
		}
		k, ok := keys[p]
		if !ok || i != len(parts)-1 {
			return Combo{}, fmt.Errorf("hotkey %q: unknown key %q", s, p)
		}
		c.Key = k
		return c, nil
	}
	return Combo{}, fmt.Errorf("hotkey %q: no key", s)
}
