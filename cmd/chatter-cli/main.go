// Command chatter-cli transcribes files and reads History from the terminal.
package main

import (
	"context"
	"fmt"
	"os"

	"chatter/cmd/chatter-cli/cli"
)

func main() {
	if err := cli.Run(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "chatter-cli:", err)
		os.Exit(1)
	}
}
