package gui

import (
	"fmt"
	"slices"
	"strings"
	"unicode"

	"chatter/internal/core"
)

// modeDraft is a Mode being edited, with list fields as comma-separated text.
type modeDraft struct {
	orig                  string // key of the Mode being edited; "" for a new one
	name, instructions    string
	apps, vocab, hotkey   string
	useApp, useSel, useCB bool
	confirmDelete         bool
}

func draftOf(md core.Mode) *modeDraft {
	return &modeDraft{orig: md.Key, name: md.Name, instructions: md.Instructions,
		apps: strings.Join(md.Apps, ", "), vocab: strings.Join(md.Vocabulary, ", "), hotkey: md.Hotkey,
		useApp: md.UseApp, useSel: md.UseSelection, useCB: md.UseClipboard}
}

func (m *mainWindow) modes(top, W float32) float32 {
	if m.draft != nil {
		return m.modeEditor(top, W)
	}
	p := m.p
	s := m.deps.Dictation.Settings()
	y := top + 12
	for _, l := range p.wrap("Click a mode to use it everywhere. Choose Auto (the chip at the top) to let the app pick: "+
		"a mode switches on in the apps it lists, and other apps get the first mode.", 13, W-200) {
		p.text(20, y, l, 13, colDim)
		y += 18
	}
	if ok, _ := m.button(W-130, top+10, "New mode", 1); ok {
		m.draft, m.focus, m.tabScroll = &modeDraft{useApp: true}, "", 0
	}
	y += 14
	for _, md := range s.Modes {
		lines := p.wrap(orDefault(md.Instructions, "No rewriting: you get exactly what the transcriber heard."), 13, W-80)
		h := float32(64 + len(lines)*18)
		picked := md.Key == s.DefaultMode
		bg := colPanel
		if picked {
			bg = colPanelHi
		}
		p.round(20, y, W-40, h-8, 10, bg)
		if picked {
			p.round(20, y, 4, h-8, 2, colAccent)
		}
		p.text(36, y+12, md.Name, 16, colText)
		p.text(36+p.measure(md.Name, 16)+14, y+15, p.ellipsize(modeInfo(md), 12, W-210-p.measure(md.Name, 16)), 12, colDim)
		ly := y + 40
		for _, l := range lines {
			p.text(36, ly, l, 13, colDim)
			ly += 18
		}
		edit, _ := m.button(W-90, y+8, "Edit", 0)
		switch {
		case edit:
			m.draft, m.focus, m.tabScroll = draftOf(md), "", 0
		case m.in.hit(20, y, W-40, h-8) && !picked:
			s.DefaultMode = md.Key
			m.save(s)
		}
		y += h
	}
	return y
}

func modeInfo(md core.Mode) string {
	var ctx []string
	if md.UseApp {
		ctx = append(ctx, "app")
	}
	if md.UseSelection {
		ctx = append(ctx, "selected text")
	}
	if md.UseClipboard {
		ctx = append(ctx, "clipboard")
	}
	var parts []string
	if len(ctx) > 0 {
		parts = append(parts, "context: "+strings.Join(ctx, ", "))
	}
	if len(md.Apps) > 0 {
		parts = append(parts, "auto in: "+strings.Join(md.Apps, ", "))
	}
	if md.Hotkey != "" {
		parts = append(parts, "hotkey: "+md.Hotkey)
	}
	return strings.Join(parts, " · ")
}

func (m *mainWindow) modeEditor(top, W float32) float32 {
	p := m.p
	d := m.draft
	s := m.deps.Dictation.Settings()
	y := top + 14
	title := "New mode"
	if d.orig != "" {
		title = "Edit mode"
	}
	p.text(20, y, title, 18, colText)
	y += 36

	label := func(text, hint string) {
		p.text(20, y, text, 13, colText)
		p.text(20+p.measure(text, 13)+10, y+1, hint, 12, colDim)
		y += 22
	}
	label("Name", "")
	m.field("mode-name", 20, y, 320, 30, &d.name, false, "e.g. Jira ticket")
	y += 44
	label("Instructions", "what the rewriter should do. Leave empty for the raw transcript, no rewriting.")
	m.field("mode-instructions", 20, y, W-40, 168, &d.instructions, true, "e.g. Format as a Jira ticket: a title, a description, then acceptance criteria as bullets.")
	y += 182
	label("Context the rewriter sees", "")
	x := float32(20)
	for _, t := range []struct {
		text string
		v    *bool
	}{{"Target app", &d.useApp}, {"Selected text", &d.useSel}, {"Clipboard", &d.useCB}} {
		if ok, w := m.chip(x, y, t.text, *t.v); ok {
			*t.v = !*t.v
		} else {
			x += w + 8
		}
	}
	y += 42
	label("Apps", "comma-separated app names or bundle IDs; with Auto picked, this mode switches on in them")
	m.field("mode-apps", 20, y, W-40, 30, &d.apps, false, "e.g. Linear, Jira, com.atlassian.jira")
	y += 44
	label("Vocabulary", "comma-separated; added to your global vocabulary for this mode")
	m.field("mode-vocab", 20, y, W-40, 30, &d.vocab, false, "")
	y += 44
	label("Hotkey", "records straight into this mode, e.g. control+option+j")
	m.field("mode-hotkey", 20, y, 320, 30, &d.hotkey, false, "none")
	y += 48

	if ok, w := m.button(20, y, "Save", 1); ok {
		if err := m.saveDraft(s); err != nil {
			m.flash(err.Error(), true)
		} else {
			m.flash("Saved "+strings.TrimSpace(d.name), false)
			m.draft, m.focus, m.tabScroll = nil, "", 0
			return y
		}
	} else if ok, _ := m.button(20+w+8, y, "Cancel", 0); ok {
		m.draft, m.focus, m.tabScroll = nil, "", 0
		return y
	}
	if d.orig != "" {
		label := "Delete mode"
		if d.confirmDelete {
			label = "Click again to delete"
		}
		if ok, _ := m.button(W-20-p.measure(label, 13)-24, y, label, 2); ok {
			if !d.confirmDelete {
				d.confirmDelete = true
			} else if err := m.deleteMode(s, d.orig); err != nil {
				m.flash(err.Error(), true)
			} else {
				m.draft, m.focus, m.tabScroll = nil, "", 0
			}
		}
	}
	return y + 36
}

func (m *mainWindow) saveDraft(s core.Settings) error {
	d := m.draft
	md := core.Mode{
		Key: d.orig, Name: strings.TrimSpace(d.name), Instructions: strings.TrimSpace(d.instructions),
		UseApp: d.useApp, UseSelection: d.useSel, UseClipboard: d.useCB,
		Apps: splitList(d.apps), Vocabulary: splitList(d.vocab), Hotkey: strings.TrimSpace(d.hotkey),
	}
	if md.Name == "" {
		return fmt.Errorf("a mode needs a name")
	}
	s.Modes = slices.Clone(s.Modes)
	if d.orig == "" {
		md.Key = uniqueKey(s, md.Name)
		s.Modes = append(s.Modes, md)
	} else {
		i := slices.IndexFunc(s.Modes, func(x core.Mode) bool { return x.Key == d.orig })
		if i < 0 {
			return fmt.Errorf("mode %q no longer exists", d.orig)
		}
		s.Modes[i] = md
	}
	return m.deps.SaveSettings(s)
}

func (m *mainWindow) deleteMode(s core.Settings, key string) error {
	if len(s.Modes) == 1 {
		return fmt.Errorf("keep at least one mode")
	}
	s.Modes = slices.DeleteFunc(slices.Clone(s.Modes), func(x core.Mode) bool { return x.Key == key })
	if s.DefaultMode == key {
		s.DefaultMode = core.AutoMode
	}
	return m.deps.SaveSettings(s)
}

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if t := strings.TrimSpace(part); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// uniqueKey derives a stable key from a new mode's name; History refers to modes by key.
func uniqueKey(s core.Settings, name string) string {
	base := strings.Trim(strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return '-'
	}, name), "-")
	if base == "" || base == core.AutoMode {
		base = "mode"
	}
	key := base
	for i := 2; ; i++ {
		if _, taken := s.ModeByKey(key); !taken {
			return key
		}
		key = fmt.Sprintf("%s-%d", base, i)
	}
}
