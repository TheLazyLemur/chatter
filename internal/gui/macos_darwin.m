// Cocoa glue for the menu bar item and the overlay window. Kept out of the cgo
// preamble because //export would otherwise compile the class twice.
#import <Cocoa/Cocoa.h>

extern void chatterMenuAction(int tag);

@interface ChatterMenuTarget : NSObject
- (void)act:(NSMenuItem *)sender;
@end

@implementation ChatterMenuTarget
- (void)act:(NSMenuItem *)sender { chatterMenuAction((int)sender.tag); }
@end

static NSStatusItem *statusItem;
static ChatterMenuTarget *menuTarget;

// menubarInit adds the status item. Accessory policy keeps Chatter out of the Dock while still
// letting its window become key, which the background-app default forbids.
void menubarInit(void) {
	statusItem = [[NSStatusBar systemStatusBar] statusItemWithLength:NSVariableStatusItemLength];
	menuTarget = [ChatterMenuTarget new];
	[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
}

void menubarSetTitle(const char *t) {
	statusItem.button.title = [NSString stringWithUTF8String:t];
}

// titles: "-" is a separator. tags < 0 are disabled labels.
void menubarSetMenu(const char **titles, int *tags, int *checked, int n) {
	NSMenu *menu = [NSMenu new];
	menu.autoenablesItems = NO;
	for (int i = 0; i < n; i++) {
		if (strcmp(titles[i], "-") == 0) { [menu addItem:[NSMenuItem separatorItem]]; continue; }
		NSMenuItem *it = [[NSMenuItem alloc] initWithTitle:[NSString stringWithUTF8String:titles[i]]
		                                            action:@selector(act:) keyEquivalent:@""];
		it.target = menuTarget;
		it.tag = tags[i];
		it.enabled = tags[i] >= 0;
		it.state = checked[i] ? NSControlStateValueOn : NSControlStateValueOff;
		[menu addItem:it];
	}
	statusItem.menu = menu;
}

// overlayStyle keeps the overlay above full-screen apps on every Space and click-through.
void overlayStyle(void *w) {
	NSWindow *win = (__bridge NSWindow *)w;
	win.level = NSStatusWindowLevel;
	win.collectionBehavior = NSWindowCollectionBehaviorCanJoinAllSpaces | NSWindowCollectionBehaviorFullScreenAuxiliary |
		NSWindowCollectionBehaviorStationary | NSWindowCollectionBehaviorIgnoresCycle;
	win.ignoresMouseEvents = YES;
	win.hasShadow = YES;
}

// overlayShow orders the overlay in without activating Chatter, so the Target app keeps focus.
void overlayShow(void *w, int show) {
	NSWindow *win = (__bridge NSWindow *)w;
	if (show) [win orderFrontRegardless]; else [win orderOut:nil];
}

// windowFront brings a window forward with keyboard focus. Chatter is a regular app (Dock icon)
// while its main window is open, because macOS refuses to activate an accessory app here.
void windowFront(void *w) {
	[NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
	if (@available(macOS 14, *)) {
		[NSApp activate];
	} else {
		[NSApp activateIgnoringOtherApps:YES];
	}
	NSWindow *win = (__bridge NSWindow *)w;
	[win makeKeyAndOrderFront:nil];
	[win orderFrontRegardless]; // visible on top even when macOS refuses activation (launched in the background)
}

// windowClosed drops back to a menu bar app with no Dock icon.
void windowClosed(void) {
	[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
}
