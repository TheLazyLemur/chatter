package core_test

import (
	"strings"
	"testing"
	"time"

	"chatter/internal/core"
	"chatter/pkg/testing/tier"
)

func TestTrigger_TapTogglesAndHoldIsPushToTalk(t *testing.T) {
	tier.Set(t, tier.Unit)

	// given
	// ... a trigger with a 400 ms hold threshold
	var tr core.Trigger
	t0 := time.Unix(0, 0)
	threshold := 400 * time.Millisecond

	// when
	// ... the key is tapped, tapped again, then pressed and held for a second
	tapDown := tr.Down(t0, false)
	tapUp := tr.Up(t0.Add(100*time.Millisecond), true, threshold)
	secondTap := tr.Down(t0.Add(time.Second), true)
	tr.Up(t0.Add(1100*time.Millisecond), false, threshold)
	holdDown := tr.Down(t0.Add(2*time.Second), false)
	repeat := tr.Down(t0.Add(2100*time.Millisecond), true)
	holdUp := tr.Up(t0.Add(3*time.Second), true, threshold)

	// then
	// ... a tap starts and the next tap finishes; a hold finishes on release; key repeat is ignored
	got := []core.Action{tapDown, tapUp, secondTap, holdDown, repeat, holdUp}
	want := []core.Action{core.StartDictation, core.NoAction, core.FinishDictation, core.StartDictation, core.NoAction, core.FinishDictation}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("step %d: got %v, want %v", i, got[i], want[i])
		}
	}
}

func TestApplyReplacements_MatchesSpokenPhrasesLoosely(t *testing.T) {
	tier.Set(t, tier.Unit)

	// given
	// ... replacements for a line break and a product name
	rs := []core.Replacement{{From: "new line", To: "\n"}, {From: "chat er", To: "Chatter"}}

	// when
	// ... a transcript with odd casing and punctuation attached to the phrases is processed
	got := core.ApplyReplacements("First point. New line. Then Chat er, obviously", rs)

	// then
	// ... both are replaced and the line break has no stray spaces
	if want := "First point.\nThen Chatter obviously"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestBuildPrompt_IncludesOnlyContextTheModeAsksFor(t *testing.T) {
	tier.Set(t, tier.Unit)

	// given
	// ... a mode that uses the clipboard but not the selection
	m := core.Mode{Name: "Super", Instructions: "Fix it.", UseClipboard: true}
	c := core.Context{App: core.App{Name: "Slack"}, SelectedText: "private selection", Clipboard: "ticket ABC-12"}

	// when
	// ... the prompt is built
	p := core.BuildPrompt(m, "see ticket abc twelve", c, []string{"Ada"})

	// then
	// ... the clipboard and vocabulary are in, the selection and app are out, and the transcript is fenced
	if !strings.Contains(p.System, "ticket ABC-12") || !strings.Contains(p.System, "Ada") {
		t.Fatalf("missing context:\n%s", p.System)
	}
	if strings.Contains(p.System, "private selection") || strings.Contains(p.System, "Slack") {
		t.Fatalf("leaked context:\n%s", p.System)
	}
	if p.User != "<transcript>\nsee ticket abc twelve\n</transcript>" {
		t.Fatalf("user %q", p.User)
	}
}

func TestSettingsValidate_RejectsUnknownDefaultMode(t *testing.T) {
	tier.Set(t, tier.Unit)

	// given
	// ... settings whose default mode does not exist
	s := core.Settings{Modes: []core.Mode{{Key: "voice", Name: "Voice"}}, DefaultMode: "email"}

	// when
	// ... they are validated
	err := s.Validate()

	// then
	// ... validation fails naming the mode
	if err == nil || !strings.Contains(err.Error(), "email") {
		t.Fatalf("err %v", err)
	}
}

func TestSelectMode_ExplicitPickBeatsAppAndAutoFollowsApp(t *testing.T) {
	tier.Set(t, tier.Unit)

	// given
	// ... Email auto-activates in Mail, and the user has picked Voice
	s := core.Settings{Modes: []core.Mode{{Key: "voice", Name: "Voice"}, {Key: "email", Name: "Email", Apps: []string{"Mail"}}}, DefaultMode: "voice"}
	auto := s
	auto.DefaultMode = core.AutoMode
	mail, other := core.App{Name: "Mail"}, core.App{Name: "TextEdit"}

	// when
	// ... a mode is selected for Mail with Voice picked, with Auto, for an unlisted app with Auto, and via an Email hotkey
	picked, _ := core.SelectMode(s, "", mail)
	autoMail, _ := core.SelectMode(auto, "", mail)
	autoOther, _ := core.SelectMode(auto, "", other)
	hotkey, _ := core.SelectMode(s, "email", other)

	// then
	// ... the pick wins, Auto follows the app or falls back to the first mode, and a mode hotkey wins over both
	got := []string{picked.Key, autoMail.Key, autoOther.Key, hotkey.Key}
	if strings.Join(got, ",") != "voice,email,voice,email" {
		t.Fatalf("selected %v", got)
	}
}

func TestNextMode_CyclesThroughAuto(t *testing.T) {
	tier.Set(t, tier.Unit)

	// given
	// ... two modes
	s := core.Settings{Modes: []core.Mode{{Key: "voice", Name: "Voice"}, {Key: "email", Name: "Email"}}}

	// when
	// ... the next mode is taken from Auto, Voice and Email
	got := []string{core.NextMode(s, core.AutoMode), core.NextMode(s, "voice"), core.NextMode(s, "email")}

	// then
	// ... Auto leads into the modes and the last mode wraps back to Auto
	if strings.Join(got, ",") != "voice,email,auto" {
		t.Fatalf("cycle %v", got)
	}
}
