// Command matrix-display renders frames for the LED strip and sends them to
// matrix-display-sink. See https://github.com/corygyarmathy/matrix-display/issues/1.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "matrix-display:", err)
		os.Exit(1)
	}
}

const usage = `usage: matrix-display <command>

commands:
  help          show this message
  serve         render frames and send them to the sink
  preview       render simulated time to a video or image, without the sink
  test-pattern  send a test pattern to the sink, for checking the panels

Run a command with -h for its flags.
`

// run is main without the process: it takes the arguments after the program
// name and writes to out, so tests can drive it. It returns when ctx ends.
func run(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		_, err := io.WriteString(out, usage)
		return err
	}
	switch args[0] {
	case "serve":
		return serve(ctx, args[1:], out)
	case "preview":
		return preview(ctx, args[1:], out)
	case "test-pattern":
		return testPattern(ctx, args[1:], out)
	}
	return fmt.Errorf("unknown command %q", args[0])
}
