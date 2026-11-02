package gui

import (
	"strings"
	"time"

	"github.com/veandco/go-sdl2/sdl"
)

// field is an editable text box. Single-line fields report Enter; multi-line ones insert a
// line break. Supports typing, ⌘V, backspace, ←/→ and ⌘←/⌘→ (line start and end), applied in
// the order the keys arrived; the cursor starts at the end.
func (m *mainWindow) field(id string, x, y, w, h float32, value *string, multiline bool, placeholder string) bool {
	p := m.p
	if m.in.clicked && m.in.inBand() {
		if m.in.over(x, y, w, h) {
			if m.focus != id {
				m.focus = id
				m.cursor = len([]rune(*value))
			}
			sdl.StartTextInput()
		} else if m.focus == id {
			m.focus = ""
		}
	}
	focused := m.focus == id
	submit := false
	r := []rune(*value)
	if focused {
		m.cursor = max(0, min(m.cursor, len(r)))
		for _, e := range m.in.edits {
			switch e.op {
			case opInsert, opEnter:
				t := e.text
				if e.op == opEnter {
					t = "\n"
				}
				if !multiline {
					t = strings.ReplaceAll(t, "\n", " ")
					if e.op == opEnter {
						continue
					}
				}
				ins := []rune(t)
				r = append(r[:m.cursor], append(ins, r[m.cursor:]...)...)
				m.cursor += len(ins)
			case opBackspace:
				if m.cursor > 0 {
					r = append(r[:m.cursor-1], r[m.cursor:]...)
					m.cursor--
				}
			case opLeft:
				m.cursor = max(0, m.cursor-1)
			case opRight:
				m.cursor = min(len(r), m.cursor+1)
			case opHome:
				m.cursor = lineStart(r, m.cursor)
			case opEnd:
				m.cursor = lineEnd(r, m.cursor)
			}
		}
		*value = string(r)
		submit = !multiline && m.in.enter
		if m.in.escape {
			m.focus = ""
		}
	}

	border := colBorder
	if focused {
		border = colInk
	}
	p.box(x, y, w, h, 6, colSurface, border)
	const size = 13
	lh := float32(size) * 1.6
	ty := y + 9 // first line's top
	if len(r) == 0 {
		p.text(x+12, ty, placeholder, size, colFaint)
	}
	blink := focused && time.Now().UnixMilli()/500%2 == 0
	clip := sdl.Rect{X: int32(x + 2), Y: int32(y + 2), W: int32(w - 4), H: int32(h - 4)}
	_ = p.r.SetClipRect(&clip)
	defer func() { _ = p.r.SetClipRect(nil) }()

	if !multiline {
		cx := p.measure(string(r[:min(m.cursorFor(id, len(r)), len(r))]), size)
		shift := max(0, cx-(w-24)) // keep the cursor in view
		p.text(x+12-shift, ty, string(r), size, colInk)
		if blink {
			p.fill(x+12-shift+cx, ty, 1.5, 16, colInk)
		}
		return submit
	}

	lines := p.spans(r, size, w-24)
	cur := 0
	c := m.cursorFor(id, len(r))
	for i, sp := range lines {
		if sp[0] <= c {
			cur = i
		}
	}
	visible := max(1, int((h-16)/lh))
	first := 0
	if focused {
		first = max(0, cur-visible+1)
	}
	for i := first; i < len(lines) && i < first+visible; i++ {
		ly := ty + float32(i-first)*lh
		p.text(x+12, ly, string(r[lines[i][0]:lines[i][1]]), size, colInk)
		if blink && i == cur {
			p.fill(x+12+p.measure(string(r[lines[i][0]:c]), size), ly, 1.5, 16, colInk)
		}
	}
	return submit
}

type editOp int

const (
	opInsert editOp = iota
	opEnter
	opBackspace
	opLeft
	opRight
	opHome // ⌘←
	opEnd  // ⌘→
)

type edit struct {
	op   editOp
	text string
}

func lineStart(r []rune, c int) int {
	for c > 0 && r[c-1] != '\n' {
		c--
	}
	return c
}

func lineEnd(r []rune, c int) int {
	for c < len(r) && r[c] != '\n' {
		c++
	}
	return c
}

// cursorFor is the cursor of the focused field, or the end for the others.
func (m *mainWindow) cursorFor(id string, n int) int {
	if m.focus == id {
		return m.cursor
	}
	return n
}

func (in *input) inBand() bool {
	return in.clipBottom == 0 || (in.my >= in.clipTop && in.my < in.clipBottom)
}

// spans wraps r into lines no wider than width, as rune ranges that keep every character,
// so a cursor index maps to a line. Explicit line breaks end a line.
func (p *painter) spans(r []rune, size int, width float32) [][2]int {
	var out [][2]int
	for start := 0; start <= len(r); {
		end := start
		for end < len(r) && r[end] != '\n' {
			end++
		}
		ls := start
		for i := start; i < end; {
			j := i
			for j < end && r[j] != ' ' {
				j++
			}
			for j < end && r[j] == ' ' {
				j++
			}
			if i > ls && p.measure(strings.TrimRight(string(r[ls:j]), " "), size) > width {
				out = append(out, [2]int{ls, i})
				ls = i
			}
			i = j
		}
		out = append(out, [2]int{ls, end})
		start = end + 1
	}
	return out
}
