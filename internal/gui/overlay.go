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
	p.fill(0, 0, overlayW, overlayH, colBg)
	p.round(1, 1, overlayW-2, overlayH-2, 14, colOverlay)

	switch st.State {
	case core.StateRecording:
		pulse := float32(0.55 + 0.45*math.Sin(float64(now.UnixMilli())/180))
		p.circle(22, 22, 6, rgba{R: colRed.R, G: colRed.G, B: colRed.B, A: uint8(255 * pulse)})
		p.text(36, 12, st.Mode.Name, 15, colText)
		el := now.Sub(st.Started)
		p.text(overlayW-62, 12, fmt.Sprintf("%d:%02d", int(el.Minutes()), int(el.Seconds())%60), 15, colDim)
		o.waveform(st.Levels)
		if st.Preview != "" {
			p.text(16, 64, p.tail(st.Preview, 13, overlayW-32), 13, colDim)
		} else {
			p.text(16, 64, "Listening…  tap again or release to finish · esc cancels", 12, colDim)
		}
	case core.StateProcessing:
		spin := int(now.UnixMilli()/120) % 8
		for i := 0; i < 8; i++ {
			a := float64(i) / 8 * 2 * math.Pi
			alpha := uint8(60 + 195*((i-spin+8)%8)/7)
			p.circle(24+float32(8*math.Cos(a)), 30+float32(8*math.Sin(a)), 2, rgba{R: 200, G: 196, B: 255, A: alpha})
		}
		stage := st.Stage
		if stage == "" {
			stage = "Working"
		}
		p.text(46, 20, stage+"…", 16, colText)
		p.text(46, 48, st.Mode.Name+" mode", 13, colDim)
	default:
		c := colGreen
		if st.IsError {
			c = colRed
		}
		p.circle(24, overlayH/2, 7, c)
		lines := p.wrap(st.Notice, 14, overlayW-64)
		y := float32(overlayH/2) - float32(min(len(lines), 3))*p.lineHeight(14)/2
		for i, l := range lines {
			if i == 3 {
				break
			}
			p.text(44, y, l, 14, colText)
			y += p.lineHeight(14)
		}
	}
	p.r.Present()
}

// waveform draws the recent input levels as mirrored bars, newest on the right.
func (o *overlay) waveform(levels []float32) {
	const bars, gap = 48, 2
	bw := float32(overlayW-32-gap*(bars-1)) / bars
	mid := float32(42)
	for i := 0; i < bars; i++ {
		var l float32
		if j := len(levels) - bars + i; j >= 0 {
			l = levels[j]
		}
		h := max(2, l*36)
		o.p.round(16+float32(i)*(bw+gap), mid-h/2, bw, h, bw/2, colWaveform)
	}
}
