package hotkey

/*
#cgo LDFLAGS: -framework Carbon
#include <Carbon/Carbon.h>

extern void chatterHotkey(int id, int pressed);

static OSStatus onHotkey(EventHandlerCallRef next, EventRef ev, void *ud) {
	EventHotKeyID hk;
	GetEventParameter(ev, kEventParamDirectObject, typeEventHotKeyID, NULL, sizeof(hk), NULL, &hk);
	chatterHotkey((int)hk.id, GetEventKind(ev) == kEventHotKeyPressed);
	return noErr;
}

static int installHandler(void) {
	EventTypeSpec types[2] = {{kEventClassKeyboard, kEventHotKeyPressed}, {kEventClassKeyboard, kEventHotKeyReleased}};
	return (int)InstallApplicationEventHandler(&onHotkey, 2, types, NULL, NULL);
}

static void *registerHotkey(int id, int key, int mods) {
	EventHotKeyRef ref = NULL;
	EventHotKeyID hid = {'chtr', (UInt32)id};
	if (RegisterEventHotKey((UInt32)key, (UInt32)mods, hid, GetApplicationEventTarget(), 0, &ref) != noErr) return NULL;
	return ref;
}

static void unregisterHotkey(void *ref) { UnregisterEventHotKey((EventHotKeyRef)ref); }
*/
import "C"

import (
	"fmt"
	"sync"
	"unsafe"
)

type binding struct {
	ref          unsafe.Pointer
	onDown, onUp func()
}

var (
	mu        sync.Mutex
	installed bool
	nextID    = 1
	bindings  = map[int]*binding{}
)

//export chatterHotkey
func chatterHotkey(id C.int, pressed C.int) {
	mu.Lock()
	b := bindings[int(id)]
	mu.Unlock()
	if b == nil {
		return
	}
	if pressed != 0 && b.onDown != nil {
		b.onDown()
	} else if pressed == 0 && b.onUp != nil {
		b.onUp()
	}
}

// Register binds combo system-wide. Callbacks run on the main thread while it pumps
// events (SDL's event loop does), so they must not block. Call from the main thread.
func Register(combo string, onDown, onUp func()) (int, error) {
	c, err := Parse(combo)
	if err != nil {
		return 0, err
	}
	mu.Lock()
	defer mu.Unlock()
	if !installed {
		if st := C.installHandler(); st != 0 {
			return 0, fmt.Errorf("hotkey: install handler: OSStatus %d", st)
		}
		installed = true
	}
	id := nextID
	ref := C.registerHotkey(C.int(id), C.int(c.Key), C.int(c.Mods))
	if ref == nil {
		return 0, fmt.Errorf("hotkey %q is taken by another app or the system", combo)
	}
	nextID++
	bindings[id] = &binding{ref: ref, onDown: onDown, onUp: onUp}
	return id, nil
}

func Unregister(id int) {
	mu.Lock()
	defer mu.Unlock()
	if b, ok := bindings[id]; ok {
		C.unregisterHotkey(b.ref)
		delete(bindings, id)
	}
}

// UnregisterAll drops every binding, e.g. before re-registering after a settings change.
func UnregisterAll() {
	mu.Lock()
	defer mu.Unlock()
	for id, b := range bindings {
		C.unregisterHotkey(b.ref)
		delete(bindings, id)
	}
}
