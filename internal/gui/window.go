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
	wheel   float32
	edits   []edit // text editing keys, in the order they arrived
	enter   bool
	escape  bool
	copyKey bool // ⌘C

	clipTop, clipBottom float32 // clicks outside this band hit nothing (scrolled-away content)
}

func (in *input) reset() {
	in.clicked, in.wheel, in.edits, in.enter, in.escape, in.copyKey = false, 0, in.edits[:0], false, false, false
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

func (in *input) hit(x, y, w, h float32) bool {
	inBand := in.clipBottom == 0 || (in.my >= in.clipTop && in.my < in.clipBottom)
	return in.clicked && inBand && in.over(x, y, w, h)
}

const (
	tabHistory = iota
	tabModes
	tabVocabulary
	tabSettings
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

// button draws a pill button and reports a click. style: 0 normal, 1 accent, 2 danger.
func (m *mainWindow) button(x, y float32, label string, style int) (bool, float32) {
	p := m.p
	w := p.measure(label, 13) + 24
	const h = 28
	bg := colPanelHi
	switch style {
	case 1:
		bg = colAccent
	case 2:
		bg = rgba{R: 90, G: 36, B: 40, A: 255}
	}
	if m.in.over(x, y, w, h) {
		bg = rgba{R: min(255, bg.R+18), G: min(255, bg.G+18), B: min(255, bg.B+18), A: 255}
	}
	p.round(x, y, w, h, 8, bg)
	p.text(x+12, y+6, label, 13, colText)
	return m.in.hit(x, y, w, h), w
}

func (m *mainWindow) chip(x, y float32, label string, on bool) (bool, float32) {
	p := m.p
	w := p.measure(label, 13) + 22
	const h = 26
	bg := colPanel
	fg := colDim
	if on {
		bg, fg = colAccent, colText
	} else if m.in.over(x, y, w, h) {
		bg = colPanelHi
	}
	p.round(x, y, w, h, 13, bg)
	p.text(x+11, y+5, label, 13, fg)
	return m.in.hit(x, y, w, h), w
}

func (m *mainWindow) draw(st core.Status) {
	p := m.p
	p.beginFrame()
	m.applyPosted()
	W, H := m.size()
	p.fill(0, 0, W, H, colBg)
	contentTop, contentBottom := float32(104), H-44
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
		m.tabHeight = end - y0 + 16
	}
	m.in.clipTop, m.in.clipBottom = 0, 0
	p.fill(0, 0, W, 95, colBg) // header and footer cover scrolled content
	m.header(st, W)
	m.footer(st, W, H)
	p.r.Present()
}

func (m *mainWindow) header(st core.Status, W float32) {
	p := m.p
	p.text(20, 16, "Chatter", 22, colText)
	s := m.deps.Dictation.Settings()
	x := float32(140)
	p.text(x, 22, "Mode", 12, colDim)
	x += 40
	if clicked, w := m.chip(x, 17, "Auto", s.DefaultMode == core.AutoMode); clicked {
		s.DefaultMode = core.AutoMode
		m.save(s)
	} else {
		x += w + 6
	}
	for _, md := range s.Modes {
		clicked, w := m.chip(x, 17, md.Name, md.Key == s.DefaultMode)
		if clicked {
			s.DefaultMode = md.Key
			m.save(s)
		}
		x += w + 6
	}

	if time.Since(m.healthAt) > 3*time.Second {
		m.healthAt = time.Now()
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			err := m.deps.Health(ctx)
			m.post(func() { m.health = err })
		}()
	}
	dot, label := colGreen, "Transcriber ready"
	if m.health != nil {
		dot, label = colAmber, "Transcriber starting…"
	}
	if st.State == core.StateRecording {
		dot, label = colRed, "Recording"
	}
	lw := p.measure(label, 12)
	p.circle(W-lw-36, 29, 4, dot)
	p.text(W-lw-24, 22, label, 12, colDim)

	tabs := []string{"History", "Modes", "Vocabulary", "Settings"}
	x = 20
	for i, t := range tabs {
		w := p.measure(t, 14)
		c := colDim
		if i == m.tab {
			c = colText
			p.fill(x, 92, w, 2, colAccent)
		}
		p.text(x, 66, t, 14, c)
		if m.in.hit(x-6, 60, w+12, 34) && m.tab != i {
			m.tab, m.tabScroll = i, 0
		}
		x += w + 28
	}
	p.fill(0, 94, W, 1, colBorder)
}

func (m *mainWindow) footer(st core.Status, W, H float32) {
	p := m.p
	p.fill(0, H-40, W, 40, colBg)
	p.fill(0, H-40, W, 1, colBorder)
	msg, c := "Drop audio or video files here to transcribe them.  "+m.hotkeyHint(), colDim
	switch {
	case time.Since(m.flashAt) < 6*time.Second:
		msg, c = m.flashMsg, colGreen
		if m.flashErr {
			c = colRed
		}
	case st.State == core.StateProcessing:
		msg, c = st.Stage+"…", colAmber
	case st.Notice != "" && time.Since(st.At) < 10*time.Second:
		msg, c = st.Notice, colGreen
		if st.IsError {
			c = colRed
		}
	}
	p.text(20, H-28, p.ellipsize(msg, 13, W-40), 13, c)
}

func (m *mainWindow) hotkeyHint() string {
	for _, h := range m.deps.Hotkeys {
		if h[0] == "Record" {
			return "Dictate anywhere with " + h[1] + "."
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
	const listW, rowH = 340, 62
	listTop := top + 48
	if m.in.over(20, listTop, listW, bottom-listTop) {
		m.scroll = max(0, min(m.scroll-m.in.wheel*30, float32(len(m.entries))*rowH-(bottom-listTop)))
	}
	if len(m.entries) == 0 {
		p.text(28, listTop+10, "No dictations yet.", 14, colDim)
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
		bg := colBg
		if e.ID == m.selected {
			bg = colPanelHi
		} else if m.in.over(20, y, listW, rowH-4) {
			bg = colPanel
		}
		p.round(20, y, listW, rowH-4, 8, bg)
		meta := e.CreatedAt.Local().Format("Jan 2 15:04") + " · " + e.ModeName + " · " + fmtDur(e.Duration)
		if e.App.Name != "" {
			meta += " · " + e.App.Name
		}
		p.text(32, y+8, p.ellipsize(meta, 11, listW-24), 11, colDim)
		line, c := e.Output, colText
		if line == "" {
			line, c = e.Error, colRed
		}
		p.text(32, y+28, p.ellipsize(line, 14, listW-24), 14, c)
		if m.in.hit(20, y, listW, rowH-4) && m.in.my >= listTop {
			m.selected = e.ID
		}
	}
	p.fill(0, top+1, listW+30, listTop-top-1, colBg) // rows scroll under the search box
	before := m.search
	m.field("search", 20, top+8, listW, 30, &m.search, false, "Search history")
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
	m.detail(*sel, 380, top+8, W-400, bottom)
}

func (m *mainWindow) detail(e core.Entry, x, y, w, bottom float32) {
	p := m.p
	meta := fmt.Sprintf("%s · %s · %s", e.CreatedAt.Local().Format("Mon 2 Jan 2006, 15:04:05"), e.ModeName, fmtDur(e.Duration))
	if e.App.Name != "" {
		meta += " · " + e.App.Name
	}
	if e.Source != "" && e.Source != "microphone" {
		meta += " · " + filepath.Base(e.Source)
	}
	p.text(x, y, p.ellipsize(meta, 12, w), 12, colDim)
	y += 28

	bx := x
	if ok, bw := m.button(bx, y, "Copy", 1); ok {
		m.copy(e.Output)
	} else {
		bx += bw + 8
	}
	if e.Transcript != e.Output {
		if ok, bw := m.button(bx, y, "Copy transcript", 0); ok {
			m.copy(e.Transcript)
		} else {
			bx += bw + 8
		}
	}
	playLabel := "Play"
	if m.playing == e.ID {
		playLabel = "Stop"
	}
	if ok, bw := m.button(bx, y, playLabel, 0); ok {
		m.togglePlay(e.ID)
	} else {
		bx += bw + 8
	}
	if ok, _ := m.button(bx, y, "Delete", 2); ok {
		if err := m.deps.History.Delete(e.ID); err != nil {
			m.flash(err.Error(), true)
		}
		m.selected, m.loadedKey = "", ""
	}
	if m.in.copyKey && m.focus == "" {
		m.copy(e.Output)
	}
	y += 40

	p.text(x, y+5, "Re-run as", 12, colDim)
	cx := x + 70
	for _, md := range m.deps.Dictation.Settings().Modes {
		clicked, cw := m.chip(cx, y, md.Name, false)
		if clicked {
			go func(id, key string) {
				_, _ = m.deps.Dictation.Reprocess(context.Background(), id, key)
			}(e.ID, md.Key)
		}
		cx += cw + 6
		if cx > x+w-60 {
			break
		}
	}
	y += 42

	if e.Error != "" {
		for _, l := range p.wrap(e.Error, 13, w) {
			p.text(x, y, l, 13, colRed)
			y += p.lineHeight(13)
		}
		y += 10
	}
	y = m.paragraph("Output", e.Output, x, y, w, bottom)
	if e.Transcript != e.Output {
		m.paragraph("Transcript", e.Transcript, x, y+14, w, bottom)
	}
}

func (m *mainWindow) paragraph(title, body string, x, y, w, bottom float32) float32 {
	p := m.p
	p.text(x, y, title, 12, colDim)
	y += 22
	lh := p.lineHeight(15) + 2
	for _, l := range p.wrap(body, 15, w) {
		if y+lh > bottom {
			p.text(x, y, "…", 15, colDim)
			return y + lh
		}
		p.text(x, y, l, 15, colText)
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

func (m *mainWindow) vocabulary(top, W float32) float32 {
	p := m.p
	s := m.deps.Dictation.Settings()
	y := top + 12
	p.text(20, y, "Names and terms the transcriber should favour, and the rewriter should spell exactly.", 13, colDim)
	y += 30
	add := m.field("vocab", 20, y, 320, 30, &m.vocabInput, false, "Add a word or name, then press Enter")
	if ok, _ := m.button(350, y+1, "Add", 1); ok || add {
		if w := strings.TrimSpace(m.vocabInput); w != "" && !slices.Contains(s.Vocabulary, w) {
			s.Vocabulary = append(s.Vocabulary, w)
			m.save(s)
		}
		m.vocabInput = ""
	}
	y += 44
	x := float32(20)
	for i, w := range s.Vocabulary {
		label := w + "  ×"
		cw := p.measure(label, 13) + 22
		if x+cw > W-20 {
			x, y = 20, y+34
		}
		clicked, _ := m.chip(x, y, label, false)
		if clicked {
			s.Vocabulary = slices.Delete(slices.Clone(s.Vocabulary), i, i+1)
			m.save(s)
			break
		}
		x += cw + 6
	}
	y += 56
	p.text(20, y, "Replacements", 16, colText)
	y += 26
	p.text(20, y, "Say the phrase on the left and get the text on the right. Edit them in the settings file.", 13, colDim)
	y += 28
	for _, r := range s.Replacements {
		p.text(28, y, "“"+r.From+"”", 14, colText)
		p.text(240, y, "→  "+visible(r.To), 14, colDim)
		y += 24
	}
	if ok, _ := m.button(20, y+8, "Open settings file", 0); ok {
		m.deps.OpenPath(m.deps.SettingsPath)
	}
	return y + 36
}

func (m *mainWindow) settings(top, W float32) float32 {
	p := m.p
	s := m.deps.Dictation.Settings()
	y := top + 14
	toggle := func(label, hint string, v *bool) {
		on := "Off"
		if *v {
			on = "On"
		}
		if ok, _ := m.chip(20, y, on, *v); ok {
			*v = !*v
			m.save(s)
		}
		p.text(80, y+4, label, 14, colText)
		p.text(80+p.measure(label, 14)+12, y+6, hint, 12, colDim)
		y += 36
	}
	toggle("Paste into the app", "otherwise the text is only copied", &s.AutoPaste)
	toggle("Restore the clipboard", "after pasting, put back what was there", &s.RestoreClipboard)
	toggle("Sound cues", "on start, stop, cancel and error", &s.SoundCues)

	mic := orDefault(s.Microphone, "System default")
	if ok, _ := m.chip(20, y, "Change", false); ok {
		devs := append([]string{""}, m.deps.Microphones()...)
		i := slices.Index(devs, s.Microphone)
		s.Microphone = devs[(i+1)%len(devs)]
		m.save(s)
	}
	p.text(100, y+4, "Microphone: "+mic, 14, colText)
	y += 36

	if ok, _ := m.chip(20, y, "−", false); ok && s.HoldThresholdMS >= 200 {
		s.HoldThresholdMS -= 100
		m.save(s)
	}
	if ok, _ := m.chip(52, y, "+", false); ok {
		s.HoldThresholdMS += 100
		m.save(s)
	}
	p.text(100, y+4, fmt.Sprintf("Hold the record key longer than %d ms for push-to-talk", s.HoldThresholdMS), 14, colText)
	y += 44

	acc := m.deps.Accessible(false)
	label, c := "Accessibility: granted (paste and selected text work)", colGreen
	if !acc {
		label, c = "Accessibility: not granted, so text is copied but not pasted", colAmber
	}
	p.circle(26, y+9, 4, c)
	p.text(38, y, label, 14, colText)
	if !acc {
		if ok, _ := m.button(W-150, y-6, "Grant access", 1); ok {
			m.deps.Accessible(true)
		}
	}
	y += 36

	p.text(20, y, "Hotkeys", 16, colText)
	y += 26
	for _, h := range m.deps.Hotkeys {
		p.text(28, y, h[0], 13, colDim)
		p.text(200, y, h[1], 13, colText)
		y += 20
	}
	y += 14
	for _, l := range m.deps.Info {
		p.text(20, y, p.ellipsize(l, 13, W-40), 13, colDim)
		y += 20
	}
	y += 12
	bx := float32(20)
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
	if ok, _ := m.button(bx, y, "Quit Chatter", 2); ok {
		sdl.PushEvent(&sdl.QuitEvent{Type: sdl.QUIT})
	}
	return y + 28
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
