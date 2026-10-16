package history_test

import (
	"testing"
	"time"

	"chatter/internal/core"
	"chatter/internal/history"
	"chatter/pkg/testing/tier"
)

func TestStore_SavesListsAndDeletes(t *testing.T) {
	tier.Set(t, tier.Integration)

	// given
	// ... a store with two entries an hour apart
	s := &history.Store{Dir: t.TempDir()}
	t0 := time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)
	old := core.Entry{ID: "a1", CreatedAt: t0, Output: "Buy milk"}
	newer := core.Entry{ID: "b2", CreatedAt: t0.Add(time.Hour), Output: "Email Ada about Quillon"}
	for _, e := range []core.Entry{old, newer} {
		if err := s.Save(e, core.Recording{Samples: []float32{0.25, -0.25}}); err != nil {
			t.Fatal(err)
		}
	}

	// when
	// ... all are listed, one is searched for, one is fetched, and the other is deleted
	all, _ := s.List("", 0)
	found, _ := s.List("quillon", 0)
	_, rec, err := s.Get("a1")
	delErr := s.Delete("b2")
	left, _ := s.List("", 0)

	// then
	// ... newest comes first, search matches case-insensitively, audio survives, delete removes
	if len(all) != 2 || all[0].ID != "b2" || len(found) != 1 || found[0].ID != "b2" {
		t.Fatalf("all %v found %v", all, found)
	}
	if err != nil || len(rec.Samples) != 2 || delErr != nil || len(left) != 1 {
		t.Fatalf("get err %v samples %d del %v left %d", err, len(rec.Samples), delErr, len(left))
	}
}

func TestStore_RejectsPathEscapingIDs(t *testing.T) {
	tier.Set(t, tier.Unit)

	// given
	// ... a store
	s := &history.Store{Dir: t.TempDir()}

	// when
	// ... an id that climbs out of the directory is deleted
	err := s.Delete("../settings")

	// then
	// ... it is refused
	if err == nil {
		t.Fatal("path traversal accepted")
	}
}
