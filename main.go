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

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	os.Args = stripPSNArgs(os.Args)

	var templateIDs listFlag
	srcKey := flag.String("src-key", os.Getenv("SENDGRID_SRC_API_KEY"), "API key of the source (parent) account [env SENDGRID_SRC_API_KEY]")
	dstKey := flag.String("dst-key", os.Getenv("SENDGRID_DST_API_KEY"), "API key of the destination (subuser) account [env SENDGRID_DST_API_KEY]")
	subuser := flag.String("dst-subuser", os.Getenv("SENDGRID_DST_SUBUSER"), "copy to this subuser using the source (parent) key instead of -dst-key [env SENDGRID_DST_SUBUSER]")
	flag.Var(&templateIDs, "template", "template ID to copy (repeatable or comma-separated)")
	name := flag.String("name", "", "name for the copied template (default: same as source; only valid with a single -template)")
	activeOnly := flag.Bool("active-only", false, "copy only the active version instead of all versions")
	baseURL := flag.String("base-url", defaultBaseURL, "SendGrid API base URL (use https://api.eu.sendgrid.com for EU)")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return 0
	}
	conn := Connection{BaseURL: *baseURL, SourceKey: *srcKey, DestKey: *dstKey, Subuser: *subuser}

	// No templates on the command line: open the app window.
	if len(templateIDs) == 0 && hasDisplay() {
		if err := runApp(ctx, conn); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		return 0
	}

	if err := conn.Validate(); err != nil {
		return usageError(err.Error() + " (-src-key, and -dst-key or -dst-subuser)")
	}
	switch {
	case len(templateIDs) == 0:
		return usageError("at least one -template is required")
	case *name != "" && len(templateIDs) > 1:
		return usageError("-name can only be used with a single -template")
	}

	src, dst := conn.Clients()

	failed := 0
	for _, id := range templateIDs {
		t, err := CopyTemplate(ctx, src, dst, id, *name, *activeOnly)
		if err != nil {
			failed++
			fmt.Fprintf(os.Stderr, "FAIL %s: %v\n", id, err)
			if t != nil {
				fmt.Fprintf(os.Stderr, "     partially created destination template %s\n", t.ID)
			}
			continue
		}
		fmt.Printf("OK   %s -> %s (%q, %d version(s))\n", id, t.ID, t.Name, len(t.Versions))
	}
	if failed > 0 {
		return 1
	}
	return 0
}

func usageError(msg string) int {
	fmt.Fprintln(os.Stderr, "error:", msg)
	flag.Usage()
	return 2
}
