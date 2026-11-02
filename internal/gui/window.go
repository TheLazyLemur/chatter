package gui

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/veandco/go-sdl2/sdl"

	"chatter/internal/core"
)

// input is one frame's worth of main-window input.
type input struct {
	mx, my  float32
	clicked bool
	taken   bool // a control already took this frame's click
	wheel   float32
	edits   []edit // text editing keys, in the order they arrived
	enter   bool
	escape  bool
	copyKey bool // ⌘C

	clipTop, clipBottom float32 // clicks outside this band hit nothing (scrolled-away content)
}

func (in *input) reset() {
	in.clicked, in.taken, in.wheel, in.edits, in.enter, in.escape, in.copyKey = false, false, 0, in.edits[:0], false, false, false
}

func (in *input) feed(ev sdl.Event, mainID uint32) {
	switch e := ev.(type) {
	case *sdl.MouseMotionEvent:
		if e.WindowID == mainID {
			in.mx, in.my = float32(e.X), float32(e.Y)
		}
	case *sdl.MouseButtonEvent:
		if e.WindowID == mainID && e.Type == sdl.MOUSEBUTTONUP && e.Button == sdl.BUTTON_LEFT {
			in.mx, in.my, in.clicked = float32(e.X), float32(e.Y), true
		}
	case *sdl.MouseWheelEvent:
		if e.WindowID == mainID {
			in.wheel += e.PreciseY
		}
	case *sdl.TextInputEvent:
		if e.WindowID == mainID {
			in.edits = append(in.edits, edit{op: opInsert, text: e.GetText()})
		}
	case *sdl.KeyboardEvent:
		if e.WindowID != mainID || e.Type != sdl.KEYDOWN {
			return
		}
		cmd := e.Keysym.Mod&sdl.KMOD_GUI != 0
		switch e.Keysym.Sym {
		case sdl.K_BACKSPACE:
			in.edits = append(in.edits, edit{op: opBackspace})
		case sdl.K_LEFT:
			op := opLeft
			if cmd {
				op = opHome
			}
			in.edits = append(in.edits, edit{op: op})
		case sdl.K_RIGHT:
			op := opRight
			if cmd {
				op = opEnd
			}
			in.edits = append(in.edits, edit{op: op})
		case sdl.K_RETURN, sdl.K_KP_ENTER:
			in.enter = true
			in.edits = append(in.edits, edit{op: opEnter})
		case sdl.K_ESCAPE:
			in.escape = true
		case sdl.K_c:
			in.copyKey = cmd
		case sdl.K_v:
			if cmd {
				t, _ := sdl.GetClipboardText()
				in.edits = append(in.edits, edit{op: opInsert, text: t})
			}
		}
	}
}

func (in *input) over(x, y, w, h float32) bool {
	return in.mx >= x && in.mx < x+w && in.my >= y && in.my < y+h
}

// hit reports a click on the area and takes it, so a control laid out after it at the same spot
// cannot act on the same click.
func (in *input) hit(x, y, w, h float32) bool {
	inBand := in.clipBottom == 0 || (in.my >= in.clipTop && in.my < in.clipBottom)
	if in.taken || !in.clicked || !inBand || !in.over(x, y, w, h) {
		return false
	}
	in.taken = true
	return true
}

const (
	tabHistory = iota
	tabModes
	tabVocabulary
	tabSettings
)

// Layout, in points.
const (
	pad        = 28  // window gutter
	headerH    = 108 // title row, tabs and the rule under them
	footerH    = 44
	readingW   = 680 // widest a single column of settings reads well
	btnPrimary = 1
	btnDanger  = 2
)

type mainWindow struct {
	win     *sdl.Window
	p       *painter
	deps    Deps
	in      input
	visible bool

	tab        int
	draft      *modeDraft // the mode being edited in the Modes tab
	focus      string     // which text field has keyboard focus
	cursor     int        // rune index in the focused field
	search     string
	vocabInput string
	entries    []core.Entry
	loadedAt   time.Time
	loadedKey  string
	selected   string
	scroll     float32 // History list
	tabScroll  float32 // Modes, Vocabulary, Settings
	tabHeight  float32
	playing    string

	flashMsg string
	flashErr bool
	flashAt  time.Time

	health   error
	healthAt time.Time

	posted chan func() // work from goroutines, applied on the main thread
}

func newMainWindow(w *sdl.Window, p *painter, d Deps) *mainWindow {
	return &mainWindow{win: w, p: p, deps: d, posted: make(chan func(), 64)}
}

// post runs f on the main thread at the next frame.
func (m *mainWindow) post(f func()) { m.posted <- f }

func (m *mainWindow) applyPosted() {
	for {
		select {
		case f := <-m.posted:
			f()
		default:
			return
		}
	}
}

func (m *mainWindow) flash(msg string, isErr bool) {
	m.flashMsg, m.flashErr, m.flashAt = msg, isErr, time.Now()
}

func (m *mainWindow) size() (float32, float32) {
	w, h := m.win.GetSize()
	return float32(w), float32(h)
}

// button draws a flat button and reports a click. style: 0 secondary, btnPrimary, btnDanger.
func (m *mainWindow) button(x, y float32, label string, style int) (bool, float32) {
	p := m.p
	w := p.measure(label, 13) + 28
	const h = 30
	hover := m.in.over(x, y, w, h)
	switch style {
	case btnPrimary:
		bg := colInk
		if hover {
			bg = colInkHi
		}
		p.round(x, y, w, h, 6, bg)
		p.text(x+14, y+7, label, 13, colSurface)
	case btnDanger:
		bg := colSurface
		if hover {
			bg = redBg
		}
		p.box(x, y, w, h, 6, bg, colBorder)
		p.text(x+14, y+7, label, 13, redFg)
	default:
		bg := colSurface
		if hover {
			bg = colBone
		}
		p.box(x, y, w, h, 6, bg, colBorder)
		p.text(x+14, y+7, label, 13, colInk)
	}
	return m.in.hit(x, y, w, h), w
}

func (m *mainWindow) chip(x, y float32, label string, on bool) (bool, float32) {
	p := m.p
	w := p.measure(label, 13) + 24
	const h = 28
	switch {
	case on:
		p.round(x, y, w, h, 6, colInk)
		p.text(x+12, y+6, label, 13, colSurface)
	case m.in.over(x, y, w, h):
		p.box(x, y, w, h, 6, colBone, colBorder)
		p.text(x+12, y+6, label, 13, colInk)
	default:
		p.box(x, y, w, h, 6, colSurface, colBorder)
		p.text(x+12, y+6, label, 13, colText)
	}
	return m.in.hit(x, y, w, h), w
}

// toggle draws a switch at (x, y) and flips *v on click.
func (m *mainWindow) toggle(x, y float32, v *bool) bool {
	p := m.p
	const w, h = 36, 20
	track, knob := colBorder, x+2
	if *v {
		track, knob = colInk, x+w-h+2
	}
	p.round(x, y, w, h, h/2, track)
	p.circle(knob+h/2-2, y+h/2, h/2-2, colSurface)
	if m.in.hit(x, y, w, h) {
		*v = !*v
		return true
	}
	return false
}

// keys draws a hotkey combo such as "control+option+space" as keycaps and returns their width.
func (m *mainWindow) keys(x, y float32, combo string, draw bool) float32 {
	p := m.p
	start := x
	for _, k := range strings.Split(combo, "+") {
		label := keyLabel(k)
		w := max(20, p.measure(label, 11)+12)
		if draw {
			p.box(x, y, w, 20, 4, colBone, colBorder)
			p.text(x+(w-p.measure(label, 11))/2, y+3, label, 11, colText)
		}
		x += w + 4
	}
	return x - start - 4
}

func keyLabel(k string) string {
	switch strings.ToLower(strings.TrimSpace(k)) {
	case "control", "ctrl":
		return "⌃"
	case "option", "alt":
		return "⌥"
	case "command", "cmd":
		return "⌘"
	case "shift":
		return "⇧"
	case "escape", "esc":
		return "esc"
	case "return", "enter":
		return "↩"
	}
	if len([]rune(k)) == 1 {
		return strings.ToUpper(k)
	}
	return strings.ToLower(k)
}

func (m *mainWindow) draw(st core.Status) {
	p := m.p
	p.beginFrame()
	m.applyPosted()
	W, H := m.size()
	p.fill(0, 0, W, H, colCanvas)
	contentTop, contentBottom := float32(headerH+1), H-footerH
	m.in.clipTop, m.in.clipBottom = contentTop, contentBottom
	if m.tab == tabHistory {
		m.history(st, contentTop, contentBottom, W)
	} else {
		if m.in.over(0, contentTop, W, contentBottom-contentTop) {
			m.tabScroll = max(0, min(m.tabScroll-m.in.wheel*30, m.tabHeight-(contentBottom-contentTop)))
		}
		y0 := contentTop - m.tabScroll
		var end float32
		switch m.tab {
		case tabModes:
			end = m.modes(y0, W)
		case tabVocabulary:
			end = m.vocabulary(y0, W)
		case tabSettings:
			end = m.settings(y0, W)
		}
		m.tabHeight = end - y0 + 32
	}
	m.in.clipTop, m.in.clipBottom = 0, 0
	p.fill(0, 0, W, headerH, colCanvas) // header and footer cover scrolled content
	m.header(st, W)
	m.footer(st, W, H)
	p.r.Present()
}

func (m *mainWindow) header(st core.Status, W float32) {
	p := m.p
	x := pad + p.textIn(serif, pad, 16, "Chatter", 30, colInk) + 32

	if time.Since(m.healthAt) > 3*time.Second {
		m.healthAt = time.Now()
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			err := m.deps.Health(ctx)
			m.post(func() { m.health = err })
		}()
	}
	bg, fg, label := greenBg, greenFg, "Ready"
	if m.health != nil {
		bg, fg, label = yellowBg, yellowFg, "Starting"
	}
	if st.State == core.StateRecording {
		bg, fg, label = redBg, redFg, "Recording"
	}
	tw := p.caps(0, 0, label, 10, fg, false) + 18
	p.tag(W-pad-tw, 28, label, bg, fg)

	s := m.deps.Dictation.Settings()
	x += p.caps(x, 31, "Mode", 10, colDim, true) + 12
	if clicked, w := m.chip(x, 23, "Auto", s.DefaultMode == core.AutoMode); clicked {
		s.DefaultMode = core.AutoMode
		m.save(s)
	} else {
		x += w + 6
	}
	for _, md := range s.Modes {
		if x+p.measure(md.Name, 13)+24 > W-pad-tw-16 {
			break
		}
		clicked, w := m.chip(x, 23, md.Name, md.Key == s.DefaultMode)
		if clicked {
			s.DefaultMode = md.Key
			m.save(s)
		}
		x += w + 6
	}

	x = pad
	for i, t := range []string{"History", "Modes", "Vocabulary", "Settings"} {
		w := p.measure(t, 14)
		c := colDim
		switch {
		case i == m.tab:
			c = colInk
			p.fill(x, headerH-2, w, 2, colInk)
		case m.in.over(x-8, 70, w+16, 38):
			c = colText
		}
		p.text(x, 78, t, 14, c)
		if m.in.hit(x-8, 70, w+16, 38) && m.tab != i {
			m.tab, m.tabScroll = i, 0
		}
		x += w + 32
	}
	p.fill(0, headerH, W, 1, colBorder)
}

func (m *mainWindow) footer(st core.Status, W, H float32) {
	p := m.p
	y := H - footerH
	p.fill(0, y, W, footerH, colCanvas)
	p.fill(0, y, W, 1, colBorder)
	msg, c := "", colDim
	switch {
	case time.Since(m.flashAt) < 6*time.Second:
		msg, c = m.flashMsg, greenFg
		if m.flashErr {
			c = redFg
		}
	case st.State == core.StateProcessing:
		msg, c = st.Stage+"…", yellowFg
	case st.Notice != "" && time.Since(st.At) < 10*time.Second:
		msg, c = st.Notice, greenFg
		if st.IsError {
			c = redFg
		}
	}
	if msg != "" {
		p.text(pad, y+14, p.ellipsize(msg, 12, W-2*pad), 12, c)
		return
	}
	p.text(pad, y+14, "Drop audio or video files here to transcribe them.", 12, colDim)
	if combo := m.recordCombo(); combo != "" {
		kw := m.keys(0, 0, combo, false)
		m.keys(W-pad-kw, y+12, combo, true)
		lw := p.measure("Dictate anywhere", 12)
		p.text(W-pad-kw-10-lw, y+14, "Dictate anywhere", 12, colDim)
	}
}

func (m *mainWindow) recordCombo() string {
	for _, h := range m.deps.Hotkeys {
		if h[0] == "Record" {
			return h[1]
		}
	}
	return ""
}

func (m *mainWindow) save(s core.Settings) {
	if err := m.deps.SaveSettings(s); err != nil {
		m.flash(err.Error(), true)
	}
}

func (m *mainWindow) reload(st core.Status) {
	key := m.search
	if st.Last != nil {
		key += "\x00" + st.Last.ID
	}
	if key == m.loadedKey && time.Since(m.loadedAt) < 3*time.Second {
		return
	}
	entries, err := m.deps.History.List(m.search, 500)
	if err != nil {
		m.flash("history: "+err.Error(), true)
		return
	}
	if st.Last != nil && (m.loadedKey == "" || !strings.HasSuffix(m.loadedKey, st.Last.ID)) {
		m.selected = st.Last.ID // follow the newest dictation
	}
	m.entries, m.loadedKey, m.loadedAt = entries, key, time.Now()
}

func (m *mainWindow) history(st core.Status, top, bottom, W float32) {
	p := m.p
	m.reload(st)
	const listW, rowH = 320, 68
	listTop := top + 70
	if m.in.over(pad, listTop, listW, bottom-listTop) {
		m.scroll = max(0, min(m.scroll-m.in.wheel*30, float32(len(m.entries))*rowH-(bottom-listTop)))
	}
	if len(m.entries) == 0 {
		title, hint := "Nothing here yet", "Your dictations will collect here."
		if m.search != "" {
			title, hint = "No matches", "Nothing in your history mentions that."
		}
		p.textIn(serif, pad, listTop+12, title, 20, colInk)
		p.text(pad, listTop+44, hint, 13, colDim)
	}
	var sel *core.Entry
	for i := range m.entries {
		e := &m.entries[i]
		if e.ID == m.selected {
			sel = e
		}
		y := listTop + float32(i)*rowH - m.scroll
		if y+rowH < listTop || y > bottom {
			continue
		}
		switch {
		case e.ID == m.selected:
			p.round(pad-12, y, listW+24, rowH-1, 8, colBone)
		case m.in.over(pad, y, listW, rowH):
			p.round(pad-12, y, listW+24, rowH-1, 8, colSurface)
		}
		p.fill(pad, y+rowH-1, listW, 1, colBorder)
		meta := shortTime(e.CreatedAt) + "  " + e.ModeName + " · " + fmtDur(e.Duration)
		if e.App.Name != "" {
			meta += " · " + e.App.Name
		}
		p.textIn(mono, pad, y+15, p.ellipsizeIn(mono, meta, 11, listW), 11, colDim)
		line, c := e.Output, colText
		if line == "" {
			line, c = e.Error, redFg
		}
		p.text(pad, y+35, p.ellipsize(line, 14, listW), 14, c)
		if m.in.hit(pad, y, listW, rowH) && m.in.my >= listTop {
			m.selected = e.ID
		}
	}
	p.fill(0, top, pad+listW+16, listTop-top, colCanvas) // rows scroll under the search box
	before := m.search
	m.field("search", pad, top+22, listW, 34, &m.search, false, "Search history")
	if m.search != before {
		m.scroll = 0
	}
	if sel == nil && len(m.entries) > 0 {
		sel = &m.entries[0]
		m.selected = sel.ID
	}
	if sel == nil {
		return
	}
	cx := float32(pad + listW + 36)
	cy := top + 22
	p.box(cx, cy, W-pad-cx, bottom-22-cy, 12, colSurface, colBorder)
	m.detail(*sel, cx+32, cy+28, W-pad-cx-64, bottom-40)
}

func (m *mainWindow) detail(e core.Entry, x, y, w, bottom float32) {
	p := m.p
	nw := p.textIn(serif, x, y, e.ModeName, 26, colInk)
	if e.App.Name != "" {
		p.tag(x+nw+14, y+9, e.App.Name, blueBg, blueFg)
	}
	y += 46
	meta := e.CreatedAt.Local().Format("Mon 2 Jan 2006  15:04:05") + "  ·  " + fmtDur(e.Duration)
	if e.Source != "" && e.Source != "microphone" {
		meta += "  ·  " + filepath.Base(e.Source)
	}
	p.textIn(mono, x, y, p.ellipsizeIn(mono, meta, 11, w), 11, colDim)
	y += 30

	bx := x
	ok, bw := m.button(bx, y, "Copy", btnPrimary)
	if ok {
		m.copy(e.Output)
	}
	bx += bw + 8
	if e.Transcript != e.Output {
		ok, bw = m.button(bx, y, "Copy transcript", 0)
		if ok {
			m.copy(e.Transcript)
		}
		bx += bw + 8
	}
	playLabel := "Play"
	if m.playing == e.ID {
		playLabel = "Stop"
	}
	ok, bw = m.button(bx, y, playLabel, 0)
	if ok {
		m.togglePlay(e.ID)
	}
	bx += bw + 8
	if ok, _ := m.button(bx, y, "Delete", btnDanger); ok {
		if err := m.deps.History.Delete(e.ID); err != nil {
			m.flash(err.Error(), true)
		}
		m.selected, m.loadedKey = "", ""
	}
	if m.in.copyKey && m.focus == "" {
		m.copy(e.Output)
	}
	y += 46

	cx := x + p.caps(x, y+9, "Re-run as", 10, colDim, true) + 14
	for _, md := range m.deps.Dictation.Settings().Modes {
		if cx+p.measure(md.Name, 13)+24 > x+w {
			break
		}
		clicked, cw := m.chip(cx, y, md.Name, false)
		if clicked {
			go func(id, key string) {
				_, _ = m.deps.Dictation.Reprocess(context.Background(), id, key)
			}(e.ID, md.Key)
		}
		cx += cw + 6
	}
	y += 50
	p.fill(x, y, w, 1, colBorder)
	y += 24

	if e.Error != "" {
		lines := p.wrap(e.Error, 13, w-32)
		h := float32(len(lines))*20 + 24
		p.round(x, y, w, h, 8, redBg)
		for i, l := range lines {
			p.text(x+16, y+12+float32(i)*20, l, 13, redFg)
		}
		y += h + 20
	}
	y = m.paragraph("Output", e.Output, colInk, x, y, w, bottom)
	if e.Transcript != e.Output {
		m.paragraph("Transcript", e.Transcript, colDim, x, y+20, w, bottom)
	}
}

// paragraph draws a small-caps label over body text at a 1.6 line height.
func (m *mainWindow) paragraph(title, body string, c rgba, x, y, w, bottom float32) float32 {
	p := m.p
	if y+40 > bottom {
		return y
	}
	p.caps(x, y, title, 10, colDim, true)
	y += 26
	const lh = 15 * 1.6
	for _, l := range p.wrap(body, 15, w) {
		if y+lh > bottom {
			p.text(x, y, "…", 15, colDim)
			return y + lh
		}
		if l == "" {
			y += lh / 2 // a paragraph break, not a full empty line
			continue
		}
		p.text(x, y, l, 15, c)
		y += lh
	}
	return y
}

func (m *mainWindow) copy(text string) {
	if err := sdl.SetClipboardText(text); err != nil {
		m.flash(err.Error(), true)
		return
	}
	m.flash(fmt.Sprintf("Copied %d words", len(strings.Fields(text))), false)
}

func (m *mainWindow) togglePlay(id string) {
	if m.playing == id {
		m.deps.StopPlayback()
		m.playing = ""
		return
	}
	_, rec, err := m.deps.History.Get(id)
	if err != nil {
		m.flash(err.Error(), true)
		return
	}
	m.deps.Play(rec.Samples)
	m.playing = id
	go func() {
		time.Sleep(rec.Duration())
		m.post(func() {
			if m.playing == id {
				m.playing = ""
			}
		})
	}()
}

func (m *mainWindow) dropFile(path string) {
	m.flash("Transcribing "+filepath.Base(path)+"…", false)
	go func() {
		e, err := m.deps.Dictation.TranscribeFile(context.Background(), path, "")
		if err == nil {
			m.post(func() { m.selected, m.loadedKey = e.ID, "" })
		}
	}()
}

// heading draws a serif section title and its muted description, returning the y below them.
func (m *mainWindow) heading(x, y, w float32, title, desc string) float32 {
	p := m.p
	p.textIn(serif, x, y, title, 24, colInk)
	y += 42
	if desc == "" {
		return y + 4
	}
	for _, l := range p.wrap(desc, 13, w) {
		p.text(x, y, l, 13, colDim)
		y += 21
	}
	return y + 18
}

func (m *mainWindow) vocabulary(top, W float32) float32 {
	p := m.p
	s := m.deps.Dictation.Settings()
	x, cw := float32(pad), min(W-2*pad, readingW)
	y := m.heading(x, top+30, cw, "Vocabulary",
		"Names and terms the transcriber should favour, and the rewriter should spell exactly.")
	add := m.field("vocab", x, y, 360, 34, &m.vocabInput, false, "Add a word or name, then press Enter")
	if ok, _ := m.button(x+372, y+2, "Add", btnPrimary); ok || add {
		if w := strings.TrimSpace(m.vocabInput); w != "" && !slices.Contains(s.Vocabulary, w) {
			s.Vocabulary = append(s.Vocabulary, w)
			m.save(s)
		}
		m.vocabInput = ""
	}
	y += 54
	cx := x
	for i, w := range s.Vocabulary {
		label := w + "  ×"
		chipW := p.measure(label, 13) + 24
		if cx+chipW > x+cw {
			cx, y = x, y+36
		}
		clicked, _ := m.chip(cx, y, label, false)
		if clicked {
			s.Vocabulary = slices.Delete(slices.Clone(s.Vocabulary), i, i+1)
			m.save(s)
			break
		}
		cx += chipW + 6
	}
	y = m.heading(x, y+72, cw, "Replacements",
		"Say the phrase on the left and get the text on the right. Edit them in the settings file.")
	p.fill(x, y, cw, 1, colBorder)
	for _, r := range s.Replacements {
		p.text(x, y+14, "“"+r.From+"”", 14, colInk)
		p.text(x+260, y+14, "→", 14, colFaint)
		to := visible(r.To)
		p.box(x+290, y+11, p.measure(to, 13)+16, 24, 4, colBone, colBorder)
		p.text(x+298, y+14, to, 13, colText)
		y += 46
		p.fill(x, y, cw, 1, colBorder)
	}
	if ok, _ := m.button(x, y+24, "Open settings file", 0); ok {
		m.deps.OpenPath(m.deps.SettingsPath)
	}
	return y + 54
}

// row draws a settings line, a label with a hint under it and a rule beneath, and returns
// the y below it. Callers draw the control on the right first.
func (m *mainWindow) row(x, y, w float32, label, hint string) float32 {
	p := m.p
	p.text(x, y+14, label, 14, colInk)
	p.text(x, y+35, p.ellipsize(hint, 12, w-200), 12, colDim)
	p.fill(x, y+63, w, 1, colBorder)
	return y + 64
}

func (m *mainWindow) settings(top, W float32) float32 {
	p := m.p
	s := m.deps.Dictation.Settings()
	x, cw := float32(pad), min(W-2*pad, readingW)
	right := x + cw
	y := m.heading(x, top+30, cw, "Dictation", "How Chatter records, and where the text goes.")
	p.fill(x, y, cw, 1, colBorder)

	for _, t := range []struct {
		label, hint string
		v           *bool
	}{
		{"Paste into the app", "Otherwise the text is only copied.", &s.AutoPaste},
		{"Restore the clipboard", "After pasting, put back what was there.", &s.RestoreClipboard},
		{"Sound cues", "On start, stop, cancel and error.", &s.SoundCues},
	} {
		if m.toggle(right-36, y+22, t.v) {
			m.save(s)
		}
		y = m.row(x, y, cw, t.label, t.hint)
	}

	if ok, _ := m.button(right-p.measure("Change", 13)-28, y+17, "Change", 0); ok {
		devs := append([]string{""}, m.deps.Microphones()...)
		i := slices.Index(devs, s.Microphone)
		s.Microphone = devs[(i+1)%len(devs)]
		m.save(s)
	}
	y = m.row(x, y, cw, "Microphone", orDefault(s.Microphone, "System default"))

	val := fmt.Sprintf("%d ms", s.HoldThresholdMS)
	vw := p.measureIn(mono, val, 12)
	pw, mw := p.measure("+", 13)+28, p.measure("−", 13)+28
	if ok, _ := m.button(right-pw, y+17, "+", 0); ok {
		s.HoldThresholdMS += 100
		m.save(s)
	}
	p.textIn(mono, right-pw-12-vw, y+24, val, 12, colInk)
	if ok, _ := m.button(right-pw-24-vw-mw, y+17, "−", 0); ok && s.HoldThresholdMS >= 200 {
		s.HoldThresholdMS -= 100
		m.save(s)
	}
	y = m.row(x, y, cw, "Push-to-talk", "Hold the record key longer than this to talk while it is held.")

	if m.deps.Accessible(false) {
		tw := p.caps(0, 0, "Granted", 10, greenFg, false) + 18
		p.tag(right-tw, y+22, "Granted", greenBg, greenFg)
		y = m.row(x, y, cw, "Accessibility", "Pasting into apps and reading selected text both work.")
	} else {
		if ok, _ := m.button(right-p.measure("Grant access", 13)-28, y+17, "Grant access", btnPrimary); ok {
			m.deps.Accessible(true)
		}
		y = m.row(x, y, cw, "Accessibility", "Not granted, so text is copied but not pasted.")
	}

	y = m.heading(x, y+48, cw, "Hotkeys", "Change them in the settings file.")
	p.fill(x, y, cw, 1, colBorder)
	for _, h := range m.deps.Hotkeys {
		p.text(x, y+13, h[0], 14, colText)
		kw := m.keys(0, 0, h[1], false)
		m.keys(right-kw, y+12, h[1], true)
		y += 44
		p.fill(x, y, cw, 1, colBorder)
	}

	y = m.heading(x, y+48, cw, "About", "")
	for _, l := range m.deps.Info {
		p.textIn(mono, x, y, p.ellipsizeIn(mono, l, 11, cw), 11, colDim)
		y += 22
	}
	y += 16
	bx := x
	if ok, w := m.button(bx, y, "Open settings file", 0); ok {
		m.deps.OpenPath(m.deps.SettingsPath)
	} else {
		bx += w + 8
	}
	if ok, w := m.button(bx, y, "Open history folder", 0); ok {
		m.deps.OpenPath(m.deps.HistoryDir)
	} else {
		bx += w + 8
	}
	if ok, _ := m.button(bx, y, "Quit Chatter", btnDanger); ok {
		sdl.PushEvent(&sdl.QuitEvent{Type: sdl.QUIT})
	}
	return y + 30
}

// shortTime is the clock time for today's entries and the date for older ones.
func shortTime(t time.Time) string {
	t = t.Local()
	if t.Format(time.DateOnly) == time.Now().Format(time.DateOnly) {
		return t.Format("15:04")
	}
	return t.Format("2 Jan")
}

func fmtDur(d time.Duration) string {
	s := int(d.Round(time.Second).Seconds())
	if s < 60 {
		return fmt.Sprintf("%ds", s)
	}
	return fmt.Sprintf("%dm%02ds", s/60, s%60)
}

func orDefault(s, d string) string {
	if strings.TrimSpace(s) == "" {
		return d
	}
	return s
}

func visible(s string) string {
	return strings.NewReplacer("\n", "⏎", "\t", "⇥").Replace(s)
}
