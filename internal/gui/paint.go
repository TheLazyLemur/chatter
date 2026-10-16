package gui

import (
	"fmt"
	"math"
	"os"
	"strings"

	"github.com/veandco/go-sdl2/sdl"
	"github.com/veandco/go-sdl2/ttf"
)

type rgba = sdl.Color

var (
	colBg       = rgba{R: 24, G: 24, B: 28, A: 255}
	colPanel    = rgba{R: 34, G: 34, B: 40, A: 255}
	colPanelHi  = rgba{R: 48, G: 48, B: 58, A: 255}
	colBorder   = rgba{R: 60, G: 60, B: 72, A: 255}
	colText     = rgba{R: 236, G: 236, B: 240, A: 255}
	colDim      = rgba{R: 150, G: 150, B: 162, A: 255}
	colAccent   = rgba{R: 124, G: 108, B: 255, A: 255}
	colRed      = rgba{R: 255, G: 82, B: 82, A: 255}
	colGreen    = rgba{R: 80, G: 210, B: 130, A: 255}
	colAmber    = rgba{R: 255, G: 190, B: 70, A: 255}
	colOverlay  = rgba{R: 18, G: 18, B: 22, A: 255}
	colWaveform = rgba{R: 200, G: 196, B: 255, A: 255}
)

var fontCandidates = []string{
	"/System/Library/Fonts/SFNS.ttf",
	"/System/Library/Fonts/Helvetica.ttc",
	"/System/Library/Fonts/Supplemental/Arial.ttf",
	"/Library/Fonts/Arial Unicode.ttf",
}

func findFont(configured string) (string, error) {
	for _, p := range append([]string{configured}, fontCandidates...) {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("no font found; set font_path in settings")
}

type texKey struct {
	text  string
	size  int
	color rgba
}

type cached struct {
	tex      *sdl.Texture
	w, h     int32
	lastUsed int
}

// painter draws in logical points on a HiDPI renderer, with a text texture cache.
type painter struct {
	r        *sdl.Renderer
	scale    float32
	fontPath string
	fonts    map[int]*ttf.Font
	cache    map[texKey]*cached
	frame    int
}

func newPainter(r *sdl.Renderer, w *sdl.Window, fontPath string) *painter {
	p := &painter{r: r, fontPath: fontPath, fonts: map[int]*ttf.Font{}, cache: map[texKey]*cached{}}
	p.updateScale(w)
	_ = r.SetDrawBlendMode(sdl.BLENDMODE_BLEND)
	return p
}

func (p *painter) updateScale(w *sdl.Window) {
	ww, _ := w.GetSize()
	ow, _, err := p.r.GetOutputSize()
	s := float32(1)
	if err == nil && ww > 0 {
		s = float32(ow) / float32(ww)
	}
	if s != p.scale {
		p.scale = s
		_ = p.r.SetScale(s, s)
		p.flush()
		for k, f := range p.fonts {
			f.Close()
			delete(p.fonts, k)
		}
	}
}

func (p *painter) font(size int) *ttf.Font {
	if f, ok := p.fonts[size]; ok {
		return f
	}
	f, err := ttf.OpenFont(p.fontPath, int(float32(size)*p.scale))
	if err != nil {
		panic(fmt.Sprintf("gui: open font %s: %v", p.fontPath, err))
	}
	p.fonts[size] = f
	return f
}

// beginFrame evicts text textures unused for a few seconds.
func (p *painter) beginFrame() {
	p.frame++
	if p.frame%120 != 0 {
		return
	}
	for k, c := range p.cache {
		if p.frame-c.lastUsed > 240 {
			c.tex.Destroy()
			delete(p.cache, k)
		}
	}
}

func (p *painter) flush() {
	for k, c := range p.cache {
		c.tex.Destroy()
		delete(p.cache, k)
	}
}

func (p *painter) texture(text string, size int, c rgba) *cached {
	k := texKey{text, size, c}
	if t, ok := p.cache[k]; ok {
		t.lastUsed = p.frame
		return t
	}
	surf, err := p.font(size).RenderUTF8Blended(text, c)
	if err != nil {
		return nil
	}
	defer surf.Free()
	tex, err := p.r.CreateTextureFromSurface(surf)
	if err != nil {
		return nil
	}
	t := &cached{tex: tex, w: surf.W, h: surf.H, lastUsed: p.frame}
	p.cache[k] = t
	return t
}

// text draws one line at (x, y) and returns its logical width.
func (p *painter) text(x, y float32, s string, size int, c rgba) float32 {
	if s == "" {
		return 0
	}
	t := p.texture(s, size, c)
	if t == nil {
		return 0
	}
	w, h := float32(t.w)/p.scale, float32(t.h)/p.scale
	_ = p.r.CopyF(t.tex, nil, &sdl.FRect{X: x, Y: y, W: w, H: h})
	return w
}

func (p *painter) measure(s string, size int) float32 {
	if s == "" {
		return 0
	}
	w, _, err := p.font(size).SizeUTF8(s)
	if err != nil {
		return 0
	}
	return float32(w) / p.scale
}

func (p *painter) lineHeight(size int) float32 {
	return float32(p.font(size).Height()) / p.scale
}

// wrap breaks s into lines no wider than width, keeping explicit line breaks.
func (p *painter) wrap(s string, size int, width float32) []string {
	var out []string
	for _, para := range strings.Split(s, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			out = append(out, "")
			continue
		}
		line := words[0]
		for _, w := range words[1:] {
			if p.measure(line+" "+w, size) > width {
				out = append(out, line)
				line = w
			} else {
				line += " " + w
			}
		}
		out = append(out, line)
	}
	return out
}

// ellipsize trims s to fit width.
func (p *painter) ellipsize(s string, size int, width float32) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if p.measure(s, size) <= width {
		return s
	}
	r := []rune(s)
	lo, hi := 0, len(r)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if p.measure(string(r[:mid])+"…", size) <= width {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return string(r[:lo]) + "…"
}

// tail keeps the end of s that fits width, for a Live preview that grows to the right.
func (p *painter) tail(s string, size int, width float32) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if p.measure(s, size) <= width {
		return s
	}
	r := []rune(s)
	lo, hi := 0, len(r)
	for lo < hi {
		mid := (lo + hi) / 2
		if p.measure("…"+string(r[mid:]), size) <= width {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	return "…" + string(r[lo:])
}

func (p *painter) fill(x, y, w, h float32, c rgba) {
	_ = p.r.SetDrawColor(c.R, c.G, c.B, c.A)
	_ = p.r.FillRectF(&sdl.FRect{X: x, Y: y, W: w, H: h})
}

// round fills a rounded rectangle row by row, at physical-pixel resolution.
func (p *painter) round(x, y, w, h, rad float32, c rgba) {
	rad = min(rad, w/2, h/2)
	_ = p.r.SetDrawColor(c.R, c.G, c.B, c.A)
	step := 1 / p.scale
	for i := float32(0); i < h; i += step {
		var d float32
		switch {
		case i < rad:
			d = rad - i - step/2
		case i > h-rad:
			d = i - (h - rad) + step/2
		}
		inset := float32(0)
		if d > 0 {
			inset = rad - float32(math.Sqrt(float64(max(0, rad*rad-d*d))))
		}
		_ = p.r.FillRectF(&sdl.FRect{X: x + inset, Y: y + i, W: w - 2*inset, H: step})
	}
}

func (p *painter) circle(cx, cy, rad float32, c rgba) {
	p.round(cx-rad, cy-rad, 2*rad, 2*rad, rad, c)
}
