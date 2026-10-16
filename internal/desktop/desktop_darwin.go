// Package desktop reads the Context and delivers Output on macOS: frontmost app,
// selected text (Accessibility), clipboard (NSPasteboard), and paste (a synthetic ⌘V).
package desktop

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework ApplicationServices -framework Carbon
#import <Cocoa/Cocoa.h>
#import <ApplicationServices/ApplicationServices.h>
#import <Carbon/Carbon.h>

typedef struct { char *name; char *bundle; int pid; } frontApp;

static char *dupstr(NSString *s) { return s ? strdup([s UTF8String]) : NULL; }

static frontApp frontmost(void) {
	frontApp out = {NULL, NULL, 0};
	@autoreleasepool {
		NSRunningApplication *a = [[NSWorkspace sharedWorkspace] frontmostApplication];
		if (a) {
			out.name = dupstr(a.localizedName);
			out.bundle = dupstr(a.bundleIdentifier);
			out.pid = a.processIdentifier;
		}
	}
	return out;
}

static int activate(int pid) {
	@autoreleasepool {
		NSRunningApplication *a = [NSRunningApplication runningApplicationWithProcessIdentifier:pid];
		if (!a) return 0;
		if (a.active) return 1;
		return [a activateWithOptions:0] ? 2 : 0;
	}
}

static char *clipboardText(void) {
	@autoreleasepool {
		return dupstr([[NSPasteboard generalPasteboard] stringForType:NSPasteboardTypeString]);
	}
}

static int setClipboardText(const char *s) {
	@autoreleasepool {
		NSPasteboard *pb = [NSPasteboard generalPasteboard];
		[pb clearContents];
		return [pb setString:[NSString stringWithUTF8String:s] forType:NSPasteboardTypeString] ? 1 : 0;
	}
}

static long clipboardChangeCount(void) { return [[NSPasteboard generalPasteboard] changeCount]; }

static int trusted(int prompt) {
	NSDictionary *opts = @{(__bridge NSString *)kAXTrustedCheckOptionPrompt: @(prompt != 0)};
	return AXIsProcessTrustedWithOptions((__bridge CFDictionaryRef)opts) ? 1 : 0;
}

static char *selectedText(void) {
	char *out = NULL;
	AXUIElementRef sys = AXUIElementCreateSystemWide();
	CFTypeRef focused = NULL;
	if (AXUIElementCopyAttributeValue(sys, kAXFocusedUIElementAttribute, &focused) == kAXErrorSuccess && focused) {
		CFTypeRef sel = NULL;
		if (AXUIElementCopyAttributeValue((AXUIElementRef)focused, kAXSelectedTextAttribute, &sel) == kAXErrorSuccess && sel) {
			if (CFGetTypeID(sel) == CFStringGetTypeID()) out = dupstr((__bridge NSString *)sel);
			CFRelease(sel);
		}
		CFRelease(focused);
	}
	CFRelease(sys);
	return out;
}

static void pressCmdV(void) {
	CGEventSourceRef src = CGEventSourceCreate(kCGEventSourceStateCombinedSessionState);
	CGEventRef down = CGEventCreateKeyboardEvent(src, kVK_ANSI_V, true);
	CGEventRef up = CGEventCreateKeyboardEvent(src, kVK_ANSI_V, false);
	CGEventSetFlags(down, kCGEventFlagMaskCommand);
	CGEventSetFlags(up, kCGEventFlagMaskCommand);
	CGEventPost(kCGHIDEventTap, down);
	CGEventPost(kCGHIDEventTap, up);
	CFRelease(down);
	CFRelease(up);
	CFRelease(src);
}

static void openPath(const char *p) {
	@autoreleasepool {
		[[NSWorkspace sharedWorkspace] openURL:[NSURL fileURLWithPath:[NSString stringWithUTF8String:p]]];
	}
}
*/
import "C"

import (
	"context"
	"errors"
	"os"
	"time"
	"unsafe"

	"chatter/internal/core"
)

var (
	_ core.ContextReader = (*Desktop)(nil)
	_ core.Deliverer     = (*Desktop)(nil)
)

type Desktop struct {
	Self int // this process's pid: never treated as a Target app
}

func New() *Desktop { return &Desktop{Self: os.Getpid()} }

func gostr(p *C.char) string {
	if p == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(p))
	return C.GoString(p)
}

func (d *Desktop) Frontmost() core.App {
	a := C.frontmost()
	return core.App{Name: gostr(a.name), BundleID: gostr(a.bundle), PID: int(a.pid)}
}

func (d *Desktop) Capture(selection, clipboard bool) (core.Context, error) {
	c := core.Context{App: d.Frontmost()}
	if c.App.PID == 0 {
		return c, errors.New("no frontmost app")
	}
	if selection && c.App.PID != d.Self && Trusted(false) {
		c.SelectedText = gostr(C.selectedText())
	}
	if clipboard {
		c.Clipboard = Clipboard()
	}
	return c, nil
}

// Deliver copies text and, when asked, pastes it into target with ⌘V, then restores the
// previous clipboard. Pasting needs the Accessibility permission.
func (d *Desktop) Deliver(ctx context.Context, text string, target core.App, o core.DeliveryOptions) error {
	paste := o.Paste && target.PID != 0 && target.PID != d.Self
	prev := ""
	if paste && o.RestoreClipboard {
		prev = Clipboard()
	}
	if err := SetClipboard(text); err != nil {
		return err
	}
	if !paste {
		return nil
	}
	if !Trusted(true) {
		return errors.New("text copied, but pasting needs Accessibility access: System Settings → Privacy & Security → Accessibility")
	}
	switch C.activate(C.int(target.PID)) {
	case 0:
		return errors.New("text copied, but " + target.Name + " is gone")
	case 2:
		time.Sleep(200 * time.Millisecond) // let the app take focus before the keystroke
	}
	C.pressCmdV()
	if o.RestoreClipboard && prev != "" {
		// shortcut: restores plain text only; images or files on the clipboard are not kept
		changed := C.clipboardChangeCount()
		go func() {
			time.Sleep(600 * time.Millisecond) // the target reads the clipboard asynchronously
			if C.clipboardChangeCount() == changed {
				_ = SetClipboard(prev)
			}
		}()
	}
	return nil
}

func Clipboard() string { return gostr(C.clipboardText()) }

func SetClipboard(s string) error {
	cs := C.CString(s)
	defer C.free(unsafe.Pointer(cs))
	if C.setClipboardText(cs) == 0 {
		return errors.New("could not write the clipboard")
	}
	return nil
}

// Trusted reports the Accessibility permission; prompt shows the system dialog when missing.
func Trusted(prompt bool) bool {
	p := 0
	if prompt {
		p = 1
	}
	return C.trusted(C.int(p)) == 1
}

// Open opens a file or folder with its default app.
func Open(path string) {
	cs := C.CString(path)
	defer C.free(unsafe.Pointer(cs))
	C.openPath(cs)
}
