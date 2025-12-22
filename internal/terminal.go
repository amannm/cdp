package internal

import (
	"fmt"
	"io"
	"os"
)

type Terminal struct {
	Out io.Writer
	Err io.Writer
}

var Term = Terminal{Out: os.Stdout, Err: os.Stderr}

func (t *Terminal) Info(format string, a ...any) {
	if Verbose {
		_, _ = fmt.Fprintf(t.Err, format, a...)
	}
}

func (t *Terminal) Error(format string, a ...any) {
	_, _ = fmt.Fprintf(t.Err, format, a...)
}

func (t *Terminal) Text(format string, a ...any) {
	_, _ = fmt.Fprintf(t.Out, format, a...)
}
