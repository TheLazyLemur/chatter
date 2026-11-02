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
	x, cw := float32(pad), W-2*pad
	bw := p.measure("New mode", 13) + 28
	if ok, _ := m.button(x+cw-bw, top+32, "New mode", btnPrimary); ok {
		m.draft, m.focus, m.tabScroll = &modeDraft{useApp: true}, "", 0
	}
	y := m.heading(x, top+30, min(cw-bw-32, 620), "Modes", "Click a mode to use it everywhere. Choose Auto at the top "+
		"to let the app pick: a mode switches on in the apps it lists, and other apps get the first mode.")

	// A bento grid: pairs of cards whose split alternates 58/42 and 42/58; an odd last card spans the row.
	const gap = 16
	for i := 0; i < len(s.Modes); i += 2 {
		row := s.Modes[i:min(i+2, len(s.Modes))]
		ws := []float32{cw}
		if len(row) == 2 {
			left := (cw - gap) * 0.58
			if i/2%2 == 1 {
				left = (cw - gap) * 0.42
			}
			ws = []float32{left, cw - gap - left}
		}
		var h float32
		for j, md := range row {
			h = max(h, m.modeCard(md, 0, 0, ws[j], 0, s, false))
		}
		cx := x
		for j, md := range row {
			m.modeCard(md, cx, y, ws[j], h, s, true)
			cx += ws[j] + gap
		}
		y += h + gap
	}
	return y
}

// modeCard lays out one Mode as a card and returns the height it needs. With draw false it
// only measures; drawn, a click picks the Mode and Edit opens it in the editor.
func (m *mainWindow) modeCard(md core.Mode, x, y, w, h float32, s core.Settings, draw bool) float32 {
	p := m.p
	info := modeInfo(md)
	body, bodyC := md.Instructions, colText
	if body == "" {
		body, bodyC = "No rewriting: you get exactly what the transcriber heard.", colDim
	}
	lines := p.wrap(body, 13, w-48)
	ly := y + 66
	if info != "" {
		ly += 26
	}
	need := ly - y + float32(len(lines))*21 + 20
	if !draw {
		return need
	}

	picked := md.Key == s.DefaultMode
	border := colBorder
	switch {
	case picked:
		border = colInk
	case m.in.over(x, y, w, h):
		border = colFaint
	}
	p.box(x, y, w, h, 12, colSurface, border)
	nw := p.textIn(serif, x+24, y+20, md.Name, 22, colInk)
	if picked {
		p.tag(x+24+nw+12, y+25, "In use", greenBg, greenFg)
	}
	if info != "" {
		p.textIn(mono, x+24, y+62, p.ellipsizeIn(mono, info, 11, w-48), 11, colDim)
	}
	for _, l := range lines {
		p.text(x+24, ly, l, 13, bodyC)
		ly += 21
	}
	edit, _ := m.button(x+w-24-p.measure("Edit", 13)-28, y+20, "Edit", 0)
	switch {
	case edit:
		m.draft, m.focus, m.tabScroll = draftOf(md), "", 0
	case m.in.hit(x, y, w, h) && !picked:
		s.DefaultMode = md.Key
		m.save(s)
	}
	return need
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
	x, cw := float32(pad), min(W-2*pad, 760)
	y := top + 30
	title := "New mode"
	if d.orig != "" {
		title = "Edit mode"
	}
	p.textIn(serif, x, y, title, 26, colInk)
	y += 58

	label := func(text, hint string) {
		p.text(x, y, text, 13, colInk)
		p.text(x+p.measure(text, 13)+10, y+1, p.ellipsize(hint, 12, cw-p.measure(text, 13)-10), 12, colDim)
		y += 24
	}
	label("Name", "")
	m.field("mode-name", x, y, 320, 34, &d.name, false, "e.g. Jira ticket")
	y += 54
	label("Instructions", "What the rewriter should do. Leave empty for the raw transcript, no rewriting.")
	m.field("mode-instructions", x, y, cw, 168, &d.instructions, true, "e.g. Format as a Jira ticket: a title, a description, then acceptance criteria as bullets.")
	y += 188
	label("Context the rewriter sees", "")
	cx := x
	for _, t := range []struct {
		text string
		v    *bool
	}{{"Target app", &d.useApp}, {"Selected text", &d.useSel}, {"Clipboard", &d.useCB}} {
		if ok, w := m.chip(cx, y, t.text, *t.v); ok {
			*t.v = !*t.v
		} else {
			cx += w + 8
		}
	}
	y += 50
	label("Apps", "Comma-separated app names or bundle IDs. With Auto picked, this mode switches on in them.")
	m.field("mode-apps", x, y, cw, 34, &d.apps, false, "e.g. Linear, Jira, com.atlassian.jira")
	y += 54
	label("Vocabulary", "Comma-separated. Added to your global vocabulary for this mode.")
	m.field("mode-vocab", x, y, cw, 34, &d.vocab, false, "")
	y += 54
	label("Hotkey", "Records straight into this mode, e.g. control+option+j")
	m.field("mode-hotkey", x, y, 320, 34, &d.hotkey, false, "none")
	y += 58
	p.fill(x, y, cw, 1, colBorder)
	y += 24

	if ok, w := m.button(x, y, "Save", btnPrimary); ok {
		if err := m.saveDraft(s); err != nil {
			m.flash(err.Error(), true)
		} else {
			m.flash("Saved "+strings.TrimSpace(d.name), false)
			m.draft, m.focus, m.tabScroll = nil, "", 0
			return y
		}
	} else if ok, _ := m.button(x+w+8, y, "Cancel", 0); ok {
		m.draft, m.focus, m.tabScroll = nil, "", 0
		return y
	}
	if d.orig != "" {
		label := "Delete mode"
		if d.confirmDelete {
			label = "Click again to delete"
		}
		if ok, _ := m.button(x+cw-p.measure(label, 13)-28, y, label, btnDanger); ok {
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
