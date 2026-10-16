// Command chatter is the desktop dictation app.
package main

import (
	"context"
	"fmt"
	"os"
	"runtime"

	"chatter/cmd/chatter/app"
)

// SDL and Cocoa need the main OS thread.
func init() { runtime.LockOSThread() }

func main() {
	if err := app.Run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "chatter:", err)
		os.Exit(1)
	}
}
