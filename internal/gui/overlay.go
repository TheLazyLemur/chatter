package gui

import (
	"fmt"
	"math"
	"time"

	"github.com/veandco/go-sdl2/sdl"

	"chatter/internal/core"
)

// overlay is the floating pill that shows a Dictation in progress and its outcome.
type overlay struct {
	win   *sdl.Window
	p     *painter
	shown bool
}

func (o *overlay) setShown(show bool) {
	if show == o.shown {
		return
	}
	o.shown = show
	if show {
		o.place()
	}
	overlayShow(o.win, show)
}

// place puts the overlay at the bottom centre of the display under the mouse.
func (o *overlay) place() {
	x, y, _ := sdl.GetGlobalMouseState()
	idx := 0
	for i := 0; i < numDisplays(); i++ {
		b, err := sdl.GetDisplayBounds(i)
		if err == nil && x >= b.X && x < b.X+b.W && y >= b.Y && y < b.Y+b.H {
			idx = i
		}
	}
	b, err := sdl.GetDisplayUsableBounds(idx)
	if err != nil {
		return
	}
	o.win.SetPosition(b.X+(b.W-overlayW)/2, b.Y+b.H-overlayH-48)
}

func numDisplays() int {
	n, err := sdl.GetNumVideoDisplays()
	if err != nil {
		return 1
	}
	return n
}

func (o *overlay) draw(st core.Status) {
	now := time.Now()
	recentNotice := st.Notice != "" && now.Sub(st.At) < noticeFor
	o.setShown(st.State != core.StateIdle || recentNotice)
	if !o.shown {
		return
	}
	p := o.p
	p.beginFrame()
	// The window's corners are rounded by Cocoa (overlayStyle); the border follows them.
	p.box(0, 0, overlayW, overlayH, 12, colSurface, colBorder)

	switch st.State {
	case core.StateRecording:
		pulse := float32(0.55 + 0.45*math.Sin(float64(now.UnixMilli())/180))
		p.circle(24, 24, 5, rgba{R: redFg.R, G: redFg.G, B: redFg.B, A: uint8(255 * pulse)})
		p.text(38, 15, st.Mode.Name, 14, colInk)
		el := now.Sub(st.Started)
		t := fmt.Sprintf("%d:%02d", int(el.Minutes()), int(el.Seconds())%60)
		p.textIn(mono, overlayW-18-p.measureIn(mono, t, 12), 16, t, 12, colDim)
		o.waveform(st.Levels)
		if st.Preview != "" {
			p.text(18, 66, p.tail(st.Preview, 12, overlayW-36), 12, colDim)
		} else {
			p.text(18, 66, "Tap again or release to finish. Esc cancels.", 12, colDim)
		}
	case core.StateProcessing:
		spin := int(now.UnixMilli()/120) % 8
		for i := 0; i < 8; i++ {
			a := float64(i) / 8 * 2 * math.Pi
			alpha := uint8(40 + 215*((i-spin+8)%8)/7)
			p.circle(30+float32(8*math.Cos(a)), overlayH/2+float32(8*math.Sin(a)), 1.8, rgba{R: colInk.R, G: colInk.G, B: colInk.B, A: alpha})
		}
		stage := st.Stage
		if stage == "" {
			stage = "Working"
		}
		p.text(56, 24, stage+"…", 15, colInk)
		p.text(56, 48, st.Mode.Name+" mode", 12, colDim)
	default:
		bg, fg := greenBg, greenFg
		if st.IsError {
			bg, fg = redBg, redFg
		}
		p.circle(30, overlayH/2, 12, bg)
		p.circle(30, overlayH/2, 4, fg)
		lines := p.wrap(st.Notice, 14, overlayW-74)
		y := float32(overlayH/2) - float32(min(len(lines), 3))*p.lineHeight(14)/2
		for i, l := range lines {
			if i == 3 {
				break
			}
			p.text(56, y, l, 14, colInk)
			y += p.lineHeight(14)
		}
	}
	p.r.Present()
}

// waveform draws the recent input levels as mirrored bars, newest on the right.
func (o *overlay) waveform(levels []float32) {
	const bars, gap = 64, 2
	bw := float32(overlayW-36-gap*(bars-1)) / bars
	mid := float32(45)
	for i := 0; i < bars; i++ {
		var l float32
		if j := len(levels) - bars + i; j >= 0 {
			l = levels[j]
		}
		h := max(2, l*28)
		o.p.round(18+float32(i)*(bw+gap), mid-h/2, bw, h, bw/2, colText)
	}
}
