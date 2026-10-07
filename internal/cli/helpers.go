package cli

import (
	"fmt"
	"io"
)

func printF(w io.Writer, format string, a ...any) {
	_, _ = fmt.Fprintf(w, format, a...)
}

func printLn(w io.Writer, a ...any) {
	_, _ = fmt.Fprintln(w, a...)
}

func printStr(w io.Writer, s string) {
	_, _ = fmt.Fprint(w, s)
}
