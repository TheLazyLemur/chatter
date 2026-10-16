// Package gui is the SDL front end: the recording overlay, the main window and the menu bar item.
// It drives core.Dictation and must run on the main OS thread.
package gui

import (
	"context"
	"fmt"
	"time"

	"github.com/veandco/go-sdl2/sdl"
	"github.com/veandco/go-sdl2/ttf"

	"chatter/internal/core"
)

// Deps is what the composition root hands the GUI.
type Deps struct {
	Dictation    *core.Dictation
	History      core.History
	SaveSettings func(core.Settings) error // persist and apply
	Play         func([]float32)
	StopPlayback func()
	Microphones  func() []string
	OpenPath     func(path string)
	Accessible   func(prompt bool) bool
	Health       func(ctx context.Context) error // the Transcriber
	Tick         func() string                   // per-frame hook; a non-empty return is flashed
	Info         []string                        // read-only lines for the Settings tab
	Hotkeys      [][2]string                     // action, combo
	SettingsPath string
	HistoryDir   string
	FontPath     string
	ShowWindow   chan struct{} // receive to raise the main window
}

const (
	mainW, mainH       = 900, 620
	overlayW, overlayH = 420, 92
	noticeFor          = 2500 * time.Millisecond
)

// Run owns the event loop until the user quits.
func Run(ctx context.Context, d Deps) error {
	if err := ttf.Init(); err != nil {
		return fmt.Errorf("ttf: %w", err)
	}
	defer ttf.Quit()
	fontPath, err := findFont(d.FontPath)
	if err != nil {
		return err
	}

	ow, err := sdl.CreateWindow("Chatter overlay", sdl.WINDOWPOS_CENTERED, sdl.WINDOWPOS_CENTERED, overlayW, overlayH,
		sdl.WINDOW_BORDERLESS|sdl.WINDOW_ALWAYS_ON_TOP|sdl.WINDOW_SKIP_TASKBAR|sdl.WINDOW_ALLOW_HIGHDPI|sdl.WINDOW_UTILITY|sdl.WINDOW_SHOWN)
	if err != nil {
		return fmt.Errorf("overlay window: %w", err)
	}
	defer ow.Destroy()
	overlayStyle(ow)
	overlayShow(ow, false)
	or, err := sdl.CreateRenderer(ow, -1, sdl.RENDERER_ACCELERATED|sdl.RENDERER_PRESENTVSYNC)
	if err != nil {
		return fmt.Errorf("overlay renderer: %w", err)
	}
	defer or.Destroy()
	_ = ow.SetWindowOpacity(0.96)

	mw, err := sdl.CreateWindow("Chatter", sdl.WINDOWPOS_CENTERED, sdl.WINDOWPOS_CENTERED, mainW, mainH,
		sdl.WINDOW_ALLOW_HIGHDPI|sdl.WINDOW_RESIZABLE|sdl.WINDOW_SHOWN)
	if err != nil {
		return fmt.Errorf("main window: %w", err)
	}
	defer mw.Destroy()
	mw.SetMinimumSize(760, 520)
	mr, err := sdl.CreateRenderer(mw, -1, sdl.RENDERER_ACCELERATED|sdl.RENDERER_PRESENTVSYNC)
	if err != nil {
		return fmt.Errorf("main renderer: %w", err)
	}
	defer mr.Destroy()

	menubarInit()
	sdl.EventState(sdl.DROPFILE, sdl.ENABLE)

	a := &app{
		deps:    d,
		ctx:     ctx,
		overlay: &overlay{win: ow, p: newPainter(or, ow, fontPath)},
		main:    newMainWindow(mw, newPainter(mr, mw, fontPath), d),
	}
	a.main.visible = true
	windowFront(mw)
	return a.loop()
}

type app struct {
	deps      Deps
	ctx       context.Context
	overlay   *overlay
	main      *mainWindow
	menuState string
	quit      bool
}

func (a *app) loop() error {
	mainID, _ := a.main.win.GetID()
	for !a.quit {
		if a.ctx.Err() != nil {
			return nil
		}
		st := a.deps.Dictation.Status()
		timeout := 250
		if st.State != core.StateIdle || a.overlay.shown || a.main.visible {
			timeout = 16
		}
		a.main.in.reset()
		for ev := sdl.WaitEventTimeout(timeout); ev != nil; ev = sdl.PollEvent() {
			a.handle(ev, mainID)
		}
		for drained := false; !drained; {
			select {
			case tag := <-menuActions:
				a.menu(tag)
			case <-a.deps.ShowWindow:
				a.showMain()
			default:
				drained = true
			}
		}
		if msg := a.deps.Tick(); msg != "" {
			a.main.flash(msg, true)
		}
		st = a.deps.Dictation.Status()
		a.updateMenubar(st)
		a.overlay.draw(st)
		if a.main.visible {
			a.main.draw(st)
		}
	}
	return nil
}

func (a *app) handle(ev sdl.Event, mainID uint32) {
	switch e := ev.(type) {
	case *sdl.QuitEvent:
		a.quit = true
	case *sdl.WindowEvent:
		if e.WindowID == mainID && e.Event == sdl.WINDOWEVENT_CLOSE {
			a.main.visible = false
			a.main.win.Hide()
			windowClosed()
		}
		if e.WindowID == mainID && (e.Event == sdl.WINDOWEVENT_SIZE_CHANGED || e.Event == sdl.WINDOWEVENT_DISPLAY_CHANGED) {
			a.main.p.updateScale(a.main.win)
		}
	case *sdl.DropEvent:
		if e.Type == sdl.DROPFILE {
			a.main.dropFile(e.File)
		}
	default:
		a.main.in.feed(ev, mainID)
	}
}

func (a *app) showMain() {
	a.main.visible = true
	a.main.win.Show()
	windowFront(a.main.win)
}

const (
	tagOpen = iota + 1
	tagToggle
	tagPasteLast
	tagQuit
	tagAuto
	tagModeBase = 100
)

func (a *app) menu(tag int) {
	d := a.deps.Dictation
	switch {
	case tag == tagOpen:
		a.showMain()
	case tag == tagToggle:
		if d.Status().State == core.StateRecording {
			go d.Finish(context.Background()) //nolint:errcheck // outcome lands in Status
		} else if err := d.Start(""); err != nil {
			a.main.flash(err.Error(), true)
		}
	case tag == tagPasteLast:
		if err := d.PasteLast(a.ctx); err != nil {
			a.main.flash(err.Error(), true)
		}
	case tag == tagQuit:
		a.quit = true
	case tag == tagAuto:
		s := d.Settings()
		s.DefaultMode = core.AutoMode
		if err := a.deps.SaveSettings(s); err != nil {
			a.main.flash(err.Error(), true)
		}
	case tag >= tagModeBase:
		s := d.Settings()
		if i := tag - tagModeBase; i < len(s.Modes) {
			s.DefaultMode = s.Modes[i].Key
			if err := a.deps.SaveSettings(s); err != nil {
				a.main.flash(err.Error(), true)
			}
		}
	}
}

func (a *app) updateMenubar(st core.Status) {
	s := a.deps.Dictation.Settings()
	title := "🎙"
	switch st.State {
	case core.StateRecording:
		title = "🔴"
	case core.StateProcessing:
		title = "⏳"
	}
	key := title + s.DefaultMode
	for _, m := range s.Modes {
		key += "\x00" + m.Name
	}
	if key == a.menuState {
		return
	}
	a.menuState = key
	menubarSetTitle(title)
	toggle := "Start dictation"
	if st.State == core.StateRecording {
		toggle = "Stop dictation"
	}
	items := []menuItem{{title: "Open Chatter", tag: tagOpen}, {title: toggle, tag: tagToggle},
		{title: "Paste last dictation", tag: tagPasteLast}, {title: "-"}, {title: "Mode", tag: -1},
		{title: "    Auto (follows the app)", tag: tagAuto, checked: s.DefaultMode == core.AutoMode}}
	for i, m := range s.Modes {
		items = append(items, menuItem{title: "    " + m.Name, tag: tagModeBase + i, checked: m.Key == s.DefaultMode})
	}
	items = append(items, menuItem{title: "-"}, menuItem{title: "Quit Chatter", tag: tagQuit})
	menubarSetMenu(items)
}
