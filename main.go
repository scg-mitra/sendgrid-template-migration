// Command sendgrid-template-migration copies SendGrid templates (and all of
// their versions) from one account to another, e.g. from a parent account to
// a subuser, using an API key for each side.
//
// With no -template flag it opens the app window.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

// listFlag is a flag that can be repeated or given comma-separated values.
type listFlag []string

func (l *listFlag) String() string { return strings.Join(*l, ",") }

func (l *listFlag) Set(s string) error {
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			*l = append(*l, p)
		}
	}
	return nil
}

// stripPSNArgs drops the -psn_* argument older macOS versions pass when
// launching an .app bundle, which would otherwise fail flag parsing.
func stripPSNArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for i, a := range args {
		if i == 0 || !strings.HasPrefix(a, "-psn_") {
			out = append(out, a)
		}
	}
	return out
}

func main() {
	os.Exit(run())
}

// cliOptions are the command-line settings for a run without the window.
type cliOptions struct {
	templateIDs listFlag
	list        bool
	exportPath  string
	jsonOutput  bool
	copy        CopyOptions
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	os.Args = stripPSNArgs(os.Args)

	var opts cliOptions
	srcKey := flag.String("src-key", os.Getenv("SENDGRID_SRC_API_KEY"), "API key of the source (parent) account [env SENDGRID_SRC_API_KEY]")
	srcSubuser := flag.String("src-subuser", os.Getenv("SENDGRID_SRC_SUBUSER"), "copy from this subuser, using -src-key as its parent [env SENDGRID_SRC_SUBUSER]")
	importPath := flag.String("import", "", "copy from this backup `file` (written by -export) instead of an account")
	dstKey := flag.String("dst-key", os.Getenv("SENDGRID_DST_API_KEY"), "API key of the destination account [env SENDGRID_DST_API_KEY]")
	subuser := flag.String("dst-subuser", os.Getenv("SENDGRID_DST_SUBUSER"), "copy to this subuser using the source (parent) key instead of -dst-key [env SENDGRID_DST_SUBUSER]")
	flag.Var(&opts.templateIDs, "template", "template ID to copy (repeatable or comma-separated)")
	flag.BoolVar(&opts.list, "list", false, "list the source templates and exit")
	flag.StringVar(&opts.exportPath, "export", "", "save the -template templates (default: all) with every version to this backup `file`, instead of copying")
	flag.StringVar(&opts.copy.Name, "name", "", "name for the copied template (default: same as source; only valid with a single -template)")
	flag.StringVar(&opts.copy.Prefix, "prefix", "", "add this to the start of each copy's name")
	flag.StringVar(&opts.copy.Suffix, "suffix", "", "add this to the end of each copy's name")
	flag.BoolVar(&opts.copy.ActiveOnly, "active-only", false, "copy only the active version instead of all versions")
	onExisting := flag.String("on-existing", string(ExistingDuplicate), "when the destination already has a template with the copy's name: duplicate, skip, or add (the versions to it)")
	flag.BoolVar(&opts.copy.DryRun, "dry-run", false, "show what would be copied without changing the destination")
	flag.BoolVar(&opts.jsonOutput, "json", false, "print results as JSON")
	baseURL := flag.String("base-url", defaultBaseURL, "SendGrid API base URL (use https://api.eu.sendgrid.com for EU)")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return 0
	}
	conn := Connection{
		BaseURL: *baseURL, SourceKey: *srcKey, SourceSubuser: *srcSubuser, SourceFile: *importPath,
		DestKey: *dstKey, Subuser: *subuser, ExportOnly: opts.list || opts.exportPath != "",
	}

	// Nothing to do on the command line: open the app window.
	cliTask := len(opts.templateIDs) > 0 || opts.list || opts.exportPath != "" || *importPath != ""
	if !cliTask && hasDisplay() {
		if err := runApp(ctx, conn); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		return 0
	}

	var err error
	if opts.copy.OnExisting, err = ParseExistingPolicy(*onExisting); err != nil {
		return usageError("-on-existing: " + err.Error())
	}
	if opts.list && opts.exportPath != "" {
		return usageError("-list and -export can't be used together")
	}
	if err := conn.Validate(); err != nil {
		return usageError(err.Error() + " (-src-key or -import, and -dst-key or -dst-subuser)")
	}

	switch {
	case opts.list:
		return cliList(ctx, conn, opts)
	case opts.exportPath != "":
		return cliExport(ctx, conn, opts)
	}
	return cliCopy(ctx, conn, opts)
}
