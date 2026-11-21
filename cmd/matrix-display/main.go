// Command matrix-display renders frames for the LED strip and sends them to
// matrix-display-sink. See https://github.com/corygyarmathy/matrix-display/issues/1.
package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "matrix-display:", err)
		os.Exit(1)
	}
}

const usage = `usage: matrix-display <command>

commands:
  help    show this message
`

// run is main without the process: it takes the arguments after the program
// name and writes to out, so tests can drive it.
func run(args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		_, err := io.WriteString(out, usage)
		return err
	}
	return fmt.Errorf("unknown command %q", args[0])
}
