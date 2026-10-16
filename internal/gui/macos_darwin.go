package gui

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa
#include <stdlib.h>

void menubarInit(void);
void menubarSetTitle(const char *t);
void menubarSetMenu(const char **titles, int *tags, int *checked, int n);
void overlayStyle(void *w);
void overlayShow(void *w, int show);
void windowFront(void *w);
void windowClosed(void);
*/
import "C"

import (
	"unsafe"

	"github.com/veandco/go-sdl2/sdl"
)

type menuItem struct {
	title   string
	tag     int
	checked bool
}

var menuActions = make(chan int, 16)

//export chatterMenuAction
func chatterMenuAction(tag C.int) {
	select {
	case menuActions <- int(tag):
	default:
	}
}

func menubarInit() { C.menubarInit() }

func menubarSetTitle(t string) {
	cs := C.CString(t)
	defer C.free(unsafe.Pointer(cs))
	C.menubarSetTitle(cs)
}

func menubarSetMenu(items []menuItem) {
	n := len(items)
	titles := C.malloc(C.size_t(n) * C.size_t(unsafe.Sizeof(uintptr(0))))
	defer C.free(titles)
	tags := make([]C.int, n)
	checked := make([]C.int, n)
	ts := unsafe.Slice((**C.char)(titles), n)
	for i, it := range items {
		ts[i] = C.CString(it.title)
		tags[i] = C.int(it.tag)
		if it.checked {
			checked[i] = 1
		}
	}
	C.menubarSetMenu((**C.char)(titles), &tags[0], &checked[0], C.int(n))
	for i := range ts {
		C.free(unsafe.Pointer(ts[i]))
	}
}

func nsWindow(w *sdl.Window) unsafe.Pointer {
	info, err := w.GetWMInfo()
	if err != nil {
		panic("gui: no window manager info: " + err.Error())
	}
	return info.GetCocoaInfo().Window
}

func overlayStyle(w *sdl.Window) { C.overlayStyle(nsWindow(w)) }
func overlayShow(w *sdl.Window, show bool) {
	s := 0
	if show {
		s = 1
	}
	C.overlayShow(nsWindow(w), C.int(s))
}
func windowFront(w *sdl.Window) { C.windowFront(nsWindow(w)) }
func windowClosed()             { C.windowClosed() }
