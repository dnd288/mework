package main

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
)

// jsonFlag is bound per-command to toggle JSON output.
var jsonFlag bool

// printJSON writes v as indented JSON to stdout.
func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// newTable returns a tabwriter writing to stdout; caller writes rows then Flush.
func newTable() *tabwriter.Writer {
	return tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
}

// row writes a tab-separated row to a tabwriter.
func row(w *tabwriter.Writer, cols ...string) {
	for i, c := range cols {
		if i > 0 {
			fmt.Fprint(w, "\t")
		}
		fmt.Fprint(w, c)
	}
	fmt.Fprintln(w)
}
