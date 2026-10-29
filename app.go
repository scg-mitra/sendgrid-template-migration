package main

import (
	"context"
	"embed"
	"encoding/csv"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// frontendFS is the built React app (npm run build in frontend/).
//
//go:embed all:frontend/dist
var frontendFS embed.FS

const (
	appID    = "com.scggrowthpartners.sendgridtemplatemigration"
	appTitle = "SendGrid Template Migration"

	// eventCopyProgress is emitted to the window after each template copy.
	eventCopyProgress = "copy:progress"
)

func init() {
	// Gives the frontend a typed payload for this event.
	application.RegisterEvent[CopyProgress](eventCopyProgress)
}

// hasDisplay reports whether a graphical session is available.
func hasDisplay() bool {
	switch runtime.GOOS {
	case "windows", "darwin":
		return true
	}
	return os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
}

// runApp opens the app window and blocks until it is closed or ctx is
// cancelled (e.g. Ctrl+C in the terminal that launched it).
func runApp(ctx context.Context, conn Connection) error {
	assets, err := fs.Sub(frontendFS, "frontend/dist")
	if err != nil {
		return fmt.Errorf("loading frontend: %w", err)
	}

	svc := &App{defaults: conn, releasesURL: latestReleaseURL}
	app := application.New(application.Options{
		Name:        appTitle,
		Description: "Copy SendGrid templates between accounts",
		Services:    []application.Service{application.NewService(svc)},
		Assets:      application.AssetOptions{Handler: application.BundledAssetFileServer(assets)},
		Mac:         application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
		Linux:       application.LinuxOptions{ApplicationID: appID},
	})
	svc.emit = func(name string, data any) { app.Event.Emit(name, data) }

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     appTitle,
		Width:     1040,
		Height:    720,
		MinWidth:  720,
		MinHeight: 560,
		URL:       "/",
	})

	stop := context.AfterFunc(ctx, app.Quit)
	defer stop()
	if err := app.Run(); err != nil {
		return fmt.Errorf("running app: %w", err)
	}
	return nil
}

// App is the backend for the window. Its exported methods are called from
// the frontend (see frontend/src/api.ts); a context.Context first parameter
// is supplied by Wails and is cancelled if the call is cancelled or the
// window closes.
type App struct {
	defaults Connection
	emit     func(name string, data any)
	// releasesURL is the GitHub API URL of the latest release, for CheckForUpdate.
	releasesURL string

	mu        sync.Mutex
	conn      Connection
	src       templateSource
	migration *migration // nil when exporting only
	byID      map[string]Template
}

// Region is a SendGrid API region the user can choose.
type Region struct {
	Label   string `json:"label"`
	BaseURL string `json:"baseURL"`
}

// Defaults are the initial values for the connect screen.
type Defaults struct {
	Connection
	Version string   `json:"version"`
	Regions []Region `json:"regions"`
}

// Defaults returns the values to pre-fill the connect screen with, taken
// from command-line flags and environment variables.
func (a *App) Defaults() Defaults {
	regions := []Region{
		{Label: "Global (api.sendgrid.com)", BaseURL: defaultBaseURL},
		{Label: "EU (api.eu.sendgrid.com)", BaseURL: euBaseURL},
	}
	if a.defaults.BaseURL != defaultBaseURL && a.defaults.BaseURL != euBaseURL {
		regions = append(regions, Region{Label: "Custom (" + a.defaults.BaseURL + ")", BaseURL: a.defaults.BaseURL})
	}
	return Defaults{Connection: a.defaults, Version: version, Regions: regions}
}

// ListSubusers returns the usernames of the active subusers of the account
// that sourceKey belongs to, sorted alphabetically.
func (a *App) ListSubusers(ctx context.Context, baseURL, sourceKey string) ([]string, error) {
	sourceKey = strings.TrimSpace(sourceKey)
	if sourceKey == "" {
		return nil, userMessage("Enter the parent API key first.")
	}
	subusers, err := NewClient(baseURL, sourceKey).ListSubusers(ctx)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 403 {
			return nil, userMessage("This key can't list subusers (it needs read access to Subuser Management). " +
				"You can still type the username.")
		}
		return nil, userMessage(capitalize(describeKeyError(err, "read")))
	}
	names := make([]string, 0, len(subusers))
	for _, su := range subusers {
		if !su.Disabled {
			names = append(names, su.Username)
		}
	}
	sort.Slice(names, func(i, j int) bool { return strings.ToLower(names[i]) < strings.ToLower(names[j]) })
	return names, nil
}

// TemplateInfo is a source template as shown in the template list.
type TemplateInfo struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Generation string `json:"generation"`
	Versions   int    `json:"versions"`
}

// Session describes a successful connection.
type Session struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	ExportOnly  bool   `json:"exportOnly"`
	DestCount   int    `json:"destCount"`
	// DestNames are the names of the templates in the destination, sorted.
	DestNames []string       `json:"destNames"`
	Templates []TemplateInfo `json:"templates"`
}

// Connect checks access to both sides and returns the source templates.
func (a *App) Connect(ctx context.Context, conn Connection) (*Session, error) {
	c, err := connect(ctx, conn)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, err
		}
		return nil, userMessage(capitalize(err.Error()))
	}

	byID := make(map[string]Template, len(c.srcTemplates))
	infos := make([]TemplateInfo, 0, len(c.srcTemplates))
	for _, t := range c.srcTemplates {
		byID[t.ID] = t
		infos = append(infos, TemplateInfo{ID: t.ID, Name: t.Name, Generation: t.Generation, Versions: len(t.Versions)})
	}
	sort.Slice(infos, func(i, j int) bool { return strings.ToLower(infos[i].Name) < strings.ToLower(infos[j].Name) })

	names := make([]string, 0, len(c.dstTemplates))
	for name := range destNames(c.dstTemplates) {
		names = append(names, name)
	}
	sort.Strings(names)

	var m *migration
	if c.dst != nil {
		m = newMigration(conn, c.src, c.dst, c.dstTemplates)
	}
	a.mu.Lock()
	a.conn, a.src, a.migration, a.byID = conn, c.src, m, byID
	a.mu.Unlock()

	return &Session{
		Source: conn.SourceLabel(), Destination: conn.DestinationLabel(), ExportOnly: conn.ExportOnly,
		DestCount: len(c.dstTemplates), DestNames: names, Templates: infos,
	}, nil
}

// CopyProgress is the payload of the eventCopyProgress event.
type CopyProgress struct {
	Done   int        `json:"done"`
	Total  int        `json:"total"`
	Index  int        `json:"index"` // position of Result's template in the ids passed to CopyTemplates
	Result CopyResult `json:"result"`
}

// CopyTemplates copies the given source templates to the destination,
// emitting eventCopyProgress after each one, and returns all results.
// opts.Name and opts.DryRun are ignored; opts.Overrides name or handle
// individual templates.
func (a *App) CopyTemplates(ctx context.Context, ids []string, opts CopyOptions) ([]CopyResult, error) {
	var err error
	a.mu.Lock()
	m, byID := a.migration, a.byID
	a.mu.Unlock()
	if m == nil {
		return nil, errors.New("not connected to a destination")
	}
	opts.Name, opts.DryRun = "", false
	if opts.OnExisting, err = ParseExistingPolicy(opts.OnExisting); err != nil {
		return nil, err
	}
	if opts.Overrides, err = cleanOverrides(opts.Overrides); err != nil {
		return nil, err
	}

	names := make(map[string]string, len(ids))
	for _, id := range ids {
		names[id] = byID[id].Name
	}
	results := m.copy(ctx, ids, opts, names, func(index, done int, res CopyResult) {
		if a.emit != nil {
			a.emit(eventCopyProgress, CopyProgress{Done: done, Total: len(ids), Index: index, Result: res})
		}
	})
	return results, ctx.Err()
}

// cleanOverrides trims override names and validates their policies, which
// may be empty to keep the policy from CopyOptions.
func cleanOverrides(in map[string]CopyOverride) (map[string]CopyOverride, error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := make(map[string]CopyOverride, len(in))
	for id, ov := range in {
		ov.Name = strings.TrimSpace(ov.Name)
		if ov.OnExisting != "" {
			var err error
			if ov.OnExisting, err = ParseExistingPolicy(ov.OnExisting); err != nil {
				return nil, fmt.Errorf("template %s: %w", id, err)
			}
		}
		out[id] = ov
	}
	return out, nil
}

// Preview is the content of a template version, for previewing.
type Preview struct {
	VersionName string `json:"versionName"`
	Active      bool   `json:"active"`
	Subject     string `json:"subject"`
	HTML        string `json:"html"`
	Plain       string `json:"plain"`
}

// PreviewTemplate returns the active version of a source template, or its
// first version if none is active. It returns nil if there are no versions.
func (a *App) PreviewTemplate(ctx context.Context, id string) (*Preview, error) {
	a.mu.Lock()
	src := a.src
	a.mu.Unlock()
	if src == nil {
		return nil, errors.New("not connected")
	}
	t, err := src.GetTemplate(ctx, id)
	if err != nil {
		return nil, userMessage(capitalize(describeKeyError(err, "read")))
	}
	if len(t.Versions) == 0 {
		return nil, nil
	}
	chosen := t.Versions[0]
	for _, v := range t.Versions {
		if v.Active == 1 {
			chosen = v
			break
		}
	}
	v, err := src.GetVersion(ctx, id, chosen.ID)
	if err != nil {
		return nil, userMessage(capitalize(describeKeyError(err, "read")))
	}
	return &Preview{VersionName: v.Name, Active: v.Active == 1, Subject: v.Subject, HTML: v.HTMLContent, Plain: v.PlainContent}, nil
}

// ExportTemplates saves the given source templates, with the full content of
// every version, to a backup file at path. It returns how many were saved.
func (a *App) ExportTemplates(ctx context.Context, ids []string, path string) (int, error) {
	a.mu.Lock()
	src := a.src
	a.mu.Unlock()
	if src == nil {
		return 0, errors.New("not connected")
	}
	b, err := ExportTemplates(ctx, src, ids)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return 0, err
		}
		return 0, userMessage(capitalize(describeKeyError(err, "read")))
	}
	if err := WriteBackup(path, b); err != nil {
		return 0, userMessage(capitalize(err.Error()))
	}
	return len(b.Templates), nil
}

// SaveResultsCSV writes copy results to a CSV file at path, for keeping a
// record of which new template ID replaced which old one.
func (a *App) SaveResultsCSV(results []CopyResult, path string) error {
	if err := writeResultsCSV(path, results); err != nil {
		return userMessage(capitalize(err.Error()))
	}
	return nil
}

// writeResultsCSV writes results as CSV to the file at path.
func writeResultsCSV(path string, results []CopyResult) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating %s: %w", path, err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("writing %s: %w", path, cerr)
		}
	}()
	w := csv.NewWriter(f)
	rows := make([][]string, 0, len(results)+1)
	rows = append(rows, []string{"name", "old_id", "new_name", "new_id", "result", "versions", "error"})
	for _, r := range results {
		action := string(r.Action)
		if r.Error != "" {
			action = "failed"
		}
		rows = append(rows, []string{r.Name, r.ID, r.NewName, r.NewID, action, strconv.Itoa(r.Versions), r.Error})
	}
	if err := w.WriteAll(rows); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// userMessage is an error whose text is a complete sentence shown to the
// user as-is.
type userMessage string

func (m userMessage) Error() string { return string(m) }

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
