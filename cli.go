package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
)

func usageError(msg string) int {
	fmt.Fprintln(os.Stderr, "error:", msg)
	flag.Usage()
	return 2
}

// fail prints err and returns the exit code for a failed run.
func fail(err error) int {
	fmt.Fprintln(os.Stderr, "error:", err)
	return 1
}

// writeJSON prints v as indented JSON.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("writing JSON: %w", err)
	}
	return nil
}

// cliList prints the source templates.
func cliList(ctx context.Context, conn Connection, opts cliOptions) int {
	c, err := connect(ctx, conn)
	if err != nil {
		return fail(err)
	}
	infos := make([]TemplateInfo, 0, len(c.srcTemplates))
	for _, t := range c.srcTemplates {
		infos = append(infos, TemplateInfo{ID: t.ID, Name: t.Name, Generation: t.Generation, Versions: len(t.Versions)})
	}
	sort.Slice(infos, func(i, j int) bool { return strings.ToLower(infos[i].Name) < strings.ToLower(infos[j].Name) })
	if err := printTemplates(os.Stdout, infos, opts.jsonOutput); err != nil {
		return fail(err)
	}
	return 0
}

// printTemplates writes infos as a table, or as JSON.
func printTemplates(w io.Writer, infos []TemplateInfo, asJSON bool) error {
	if asJSON {
		return writeJSON(w, infos)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	// tabwriter buffers until Flush, which reports any write error.
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tGENERATION\tVERSIONS")
	for _, t := range infos {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%d\n", t.ID, t.Name, t.Generation, t.Versions)
	}
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("writing template list: %w", err)
	}
	return nil
}

// cliExport saves templates to a backup file.
func cliExport(ctx context.Context, conn Connection, opts cliOptions) int {
	c, err := connect(ctx, conn)
	if err != nil {
		return fail(err)
	}
	ids := []string(opts.templateIDs)
	if len(ids) == 0 {
		ids = make([]string, 0, len(c.srcTemplates))
		for _, t := range c.srcTemplates {
			ids = append(ids, t.ID)
		}
	}
	b, err := ExportTemplates(ctx, c.src, ids)
	if err != nil {
		return fail(err)
	}
	if err := WriteBackup(opts.exportPath, b); err != nil {
		return fail(err)
	}
	fmt.Fprintf(os.Stderr, "Saved %s to %s\n", plural(len(b.Templates), "template"), opts.exportPath)
	return 0
}

// cliCopy copies templates to the destination, printing each result as it
// finishes (or all of them as JSON at the end).
func cliCopy(ctx context.Context, conn Connection, opts cliOptions) int {
	ids := []string(opts.templateIDs)
	switch {
	case len(ids) == 0 && conn.SourceFile == "":
		return usageError("at least one -template is required")
	case opts.copy.Name != "" && len(ids) != 1:
		return usageError("-name can only be used with a single -template")
	}

	c, err := connect(ctx, conn)
	if err != nil {
		return fail(err)
	}
	if len(ids) == 0 { // copy everything in the backup file
		ids = make([]string, 0, len(c.srcTemplates))
		for _, t := range c.srcTemplates {
			ids = append(ids, t.ID)
		}
	}

	m := newMigration(conn, c.src, c.dst, c.dstTemplates)
	var report func(int, int, CopyResult)
	if !opts.jsonOutput {
		report = func(_, _ int, r CopyResult) { printResult(os.Stdout, os.Stderr, r, opts.copy.DryRun) }
	}
	results := m.copy(ctx, ids, opts.copy, nil, report)
	if opts.jsonOutput {
		if err := writeJSON(os.Stdout, results); err != nil {
			return fail(err)
		}
	}
	for _, r := range results {
		if r.Error != "" {
			return 1
		}
	}
	return 0
}

// printResult prints one copy result: successes to out, failures to errOut.
func printResult(out, errOut io.Writer, r CopyResult, dryRun bool) {
	w, line := out, ""
	switch {
	case r.Error != "":
		w, line = errOut, fmt.Sprintf("FAIL %s: %s\n", r.ID, r.Error)
		if r.PartialID != "" {
			line += fmt.Sprintf("     partially created destination template %s\n", r.PartialID)
		}
	case r.Action == ActionSkipped:
		line = fmt.Sprintf("SKIP %s: %q already exists as %s\n", r.ID, r.NewName, r.NewID)
	case r.Action == ActionAdded && dryRun:
		line = fmt.Sprintf("ADD  %s -> %s (%q, would add %s)\n", r.ID, r.NewID, r.NewName, plural(r.Versions, "version"))
	case r.Action == ActionAdded:
		line = fmt.Sprintf("ADD  %s -> %s (%q, added %s)\n", r.ID, r.NewID, r.NewName, plural(r.Versions, "version"))
	case dryRun:
		line = fmt.Sprintf("OK   %s -> new template (%q, would copy %s)\n", r.ID, r.NewName, plural(r.Versions, "version"))
	default:
		line = fmt.Sprintf("OK   %s -> %s (%q, %s)\n", r.ID, r.NewID, r.NewName, plural(r.Versions, "version"))
	}
	// Progress output to the terminal; a failed write leaves nothing to do,
	// and the exit code still reflects the results.
	_, _ = io.WriteString(w, line)
}

// plural returns e.g. "1 version" or "3 versions".
func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}
