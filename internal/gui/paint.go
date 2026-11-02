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

// Warm monochrome, with washed-out pastels reserved for status.
var (
	colCanvas  = rgba{R: 251, G: 251, B: 250, A: 255}
	colSurface = rgba{R: 255, G: 255, B: 255, A: 255}
	colBone    = rgba{R: 247, G: 246, B: 243, A: 255}
	colBorder  = rgba{R: 234, G: 234, B: 234, A: 255}
	colInk     = rgba{R: 17, G: 17, B: 17, A: 255}
	colInkHi   = rgba{R: 51, G: 51, B: 51, A: 255}
	colText    = rgba{R: 47, G: 52, B: 55, A: 255}
	colDim     = rgba{R: 120, G: 119, B: 116, A: 255}
	colFaint   = rgba{R: 172, G: 170, B: 165, A: 255}

	redBg, redFg       = rgba{R: 253, G: 235, B: 236, A: 255}, rgba{R: 159, G: 47, B: 45, A: 255}
	blueBg, blueFg     = rgba{R: 225, G: 243, B: 254, A: 255}, rgba{R: 31, G: 108, B: 159, A: 255}
	greenBg, greenFg   = rgba{R: 237, G: 243, B: 236, A: 255}, rgba{R: 52, G: 101, B: 56, A: 255}
	yellowBg, yellowFg = rgba{R: 251, G: 243, B: 219, A: 255}, rgba{R: 149, G: 100, B: 0, A: 255}
)

type family int

const (
	sans  family = iota // UI and body
	serif               // headings
	mono                // metadata and keys
)

var fontCandidates = [...][]string{
	sans: {
		"/System/Library/Fonts/SFNS.ttf",
		"/System/Library/Fonts/Helvetica.ttc",
		"/System/Library/Fonts/Supplemental/Arial.ttf",
		"/Library/Fonts/Arial Unicode.ttf",
	},
	serif: {
		"/System/Library/Fonts/NewYork.ttf",
		"/System/Library/Fonts/Supplemental/Charter.ttc",
		"/System/Library/Fonts/Supplemental/Georgia.ttf",
	},
	mono: {
		"/System/Library/Fonts/SFNSMono.ttf",
		"/System/Library/Fonts/Menlo.ttc",
	},
}

// findFonts resolves each family; serif and mono fall back to the sans font.
func findFonts(configured string) ([3]string, error) {
	var out [3]string
	for f, paths := range fontCandidates {
		if f == int(sans) {
			paths = append([]string{configured}, paths...)
		}
		for _, p := range paths {
			if _, err := os.Stat(p); p != "" && err == nil {
				out[f] = p
				break
			}
		}
	}
	if out[sans] == "" {
		return out, fmt.Errorf("no font found; set font_path in settings")
	}
	for f := range out {
		out[f] = orDefault(out[f], out[sans])
	}
	return out, nil
}

type fontKey struct {
	fam  family
	size int
}

type texKey struct {
	fontKey
	text  string
	color rgba
}

type cached struct {
	tex      *sdl.Texture
	w, h     int32
	lastUsed int
}

// painter draws in logical points on a HiDPI renderer, with a text texture cache.
type painter struct {
	r         *sdl.Renderer
	scale     float32
	fontPaths [3]string
	fonts     map[fontKey]*ttf.Font
	cache     map[texKey]*cached
	frame     int
}

func newPainter(r *sdl.Renderer, w *sdl.Window, fontPaths [3]string) *painter {
	p := &painter{r: r, fontPaths: fontPaths, fonts: map[fontKey]*ttf.Font{}, cache: map[texKey]*cached{}}
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

func (p *painter) font(k fontKey) *ttf.Font {
	if f, ok := p.fonts[k]; ok {
		return f
	}
	path := p.fontPaths[k.fam]
	f, err := ttf.OpenFont(path, int(float32(k.size)*p.scale))
	if err != nil {
		panic(fmt.Sprintf("gui: open font %s: %v", path, err))
	}
	p.fonts[k] = f
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

func (p *painter) texture(text string, fk fontKey, c rgba) *cached {
	k := texKey{fk, text, c}
	if t, ok := p.cache[k]; ok {
		t.lastUsed = p.frame
		return t
	}
	surf, err := p.font(fk).RenderUTF8Blended(text, c)
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

// text draws one line of sans at (x, y) and returns its logical width.
func (p *painter) text(x, y float32, s string, size int, c rgba) float32 {
	return p.textIn(sans, x, y, s, size, c)
}

func (p *painter) textIn(f family, x, y float32, s string, size int, c rgba) float32 {
	if s == "" {
		return 0
	}
	t := p.texture(s, fontKey{f, size}, c)
	if t == nil {
		return 0
	}
	w, h := float32(t.w)/p.scale, float32(t.h)/p.scale
	_ = p.r.CopyF(t.tex, nil, &sdl.FRect{X: x, Y: y, W: w, H: h})
	return w
}

func (p *painter) measure(s string, size int) float32 { return p.measureIn(sans, s, size) }

func (p *painter) measureIn(f family, s string, size int) float32 {
	if s == "" {
		return 0
	}
	w, _, err := p.font(fontKey{f, size}).SizeUTF8(s)
	if err != nil {
		return 0
	}
	return float32(w) / p.scale
}

func (p *painter) lineHeight(size int) float32 {
	return float32(p.font(fontKey{sans, size}).Height()) / p.scale
}

// caps draws s as small uppercase with wide tracking, rune by rune since SDL_ttf has no
// letter-spacing. With draw false it only measures.
func (p *painter) caps(x, y float32, s string, size int, c rgba, draw bool) float32 {
	start := x
	for _, r := range strings.ToUpper(s) {
		ch := string(r)
		if draw {
			p.text(x, y, ch, size, c)
		}
		x += p.measure(ch, size) + float32(size)*0.09
	}
	return x - start
}

// tag draws a pastel status badge and returns its width.
func (p *painter) tag(x, y float32, s string, bg, fg rgba) float32 {
	w := p.caps(0, 0, s, 10, fg, false) + 18
	p.round(x, y, w, 20, 10, bg)
	p.caps(x+9, y+4, s, 10, fg, true)
	return w
}

// box fills a rounded rectangle with a 1px border.
func (p *painter) box(x, y, w, h, rad float32, fill, border rgba) {
	p.round(x, y, w, h, rad, border)
	p.round(x+1, y+1, w-2, h-2, rad-1, fill)
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

// ellipsize trims sans s to fit width.
func (p *painter) ellipsize(s string, size int, width float32) string {
	return p.ellipsizeIn(sans, s, size, width)
}

func (p *painter) ellipsizeIn(f family, s string, size int, width float32) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if p.measureIn(f, s, size) <= width {
		return s
	}
	r := []rune(s)
	lo, hi := 0, len(r)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if p.measureIn(f, string(r[:mid])+"…", size) <= width {
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
