package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"chatter/internal/config"
	"chatter/pkg/testing/tier"
)

func TestLoad_WritesDefaultsThenKeepsUserLists(t *testing.T) {
	tier.Set(t, tier.Integration)

	// given
	// ... no settings file, then a user edit that trims the modes to one
	path := filepath.Join(t.TempDir(), "settings.json")
	first, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(`{"modes":[{"key":"raw","name":"Raw"}],"default_mode":"raw"}`, "\n", "", -1)
	if err := os.WriteFile(path, []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}

	// when
	// ... the file is loaded again
	second, err := config.Load(path)

	// then
	// ... the first load wrote valid defaults; the second keeps only the user's mode and defaults the rest
	if err != nil || len(first.Modes) < 2 || len(second.Modes) != 1 || second.Hotkeys.Record != "control+option+space" {
		t.Fatalf("err %v first %d modes, second %+v", err, len(first.Modes), second.Modes)
	}
}
