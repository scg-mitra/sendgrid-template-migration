package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"runtime"
	"sort"
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

	svc := &App{defaults: conn}
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
		Width:     880,
		Height:    700,
		MinWidth:  640,
		MinHeight: 520,
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
// frontend/app.js; a context.Context first parameter is supplied by Wails and
// is cancelled if the call is cancelled or the window closes.
type App struct {
	defaults Connection
	emit     func(name string, data any)

	mu       sync.Mutex
	conn     Connection
	src, dst *Client
	byID     map[string]Template
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
		return nil, userMessage("Enter the source (parent) API key first.")
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
	ID           string `json:"id"`
	Name         string `json:"name"`
	Generation   string `json:"generation"`
	Versions     int    `json:"versions"`
	ExistsInDest bool   `json:"existsInDest"`
}

// Session describes a successful connection.
type Session struct {
	Destination string         `json:"destination"`
	DestCount   int            `json:"destCount"`
	Templates   []TemplateInfo `json:"templates"`
}

// Connect checks access to both accounts and returns the source templates.
func (a *App) Connect(ctx context.Context, conn Connection) (*Session, error) {
	c, err := connect(ctx, conn)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, err
		}
		return nil, userMessage(capitalize(err.Error()))
	}

	existing := make(map[string]bool, len(c.dstTemplates))
	for _, t := range c.dstTemplates {
		existing[t.Name] = true
	}
	byID := make(map[string]Template, len(c.srcTemplates))
	infos := make([]TemplateInfo, 0, len(c.srcTemplates))
	for _, t := range c.srcTemplates {
		byID[t.ID] = t
		infos = append(infos, TemplateInfo{
			ID: t.ID, Name: t.Name, Generation: t.Generation,
			Versions: len(t.Versions), ExistsInDest: existing[t.Name],
		})
	}
	sort.Slice(infos, func(i, j int) bool { return strings.ToLower(infos[i].Name) < strings.ToLower(infos[j].Name) })

	a.mu.Lock()
	a.conn, a.src, a.dst, a.byID = conn, c.src, c.dst, byID
	a.mu.Unlock()

	return &Session{Destination: conn.DestinationLabel(), DestCount: len(c.dstTemplates), Templates: infos}, nil
}

// CopyResult is the outcome of copying one template.
type CopyResult struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	NewID     string `json:"newID,omitempty"`
	Versions  int    `json:"versions"`
	Error     string `json:"error,omitempty"`
	PartialID string `json:"partialID,omitempty"` // destination template left behind by a failed copy
}

// CopyProgress is the payload of the eventCopyProgress event.
type CopyProgress struct {
	Done   int        `json:"done"`
	Total  int        `json:"total"`
	Result CopyResult `json:"result"`
}

// CopyTemplates copies the given source templates to the destination,
// emitting eventCopyProgress after each one, and returns all results.
func (a *App) CopyTemplates(ctx context.Context, ids []string, activeOnly bool) ([]CopyResult, error) {
	a.mu.Lock()
	conn, src, dst, byID := a.conn, a.src, a.dst, a.byID
	a.mu.Unlock()
	if src == nil || dst == nil {
		return nil, errors.New("not connected")
	}

	results := make([]CopyResult, 0, len(ids))
	for i, id := range ids {
		if err := ctx.Err(); err != nil {
			return results, err
		}
		res := CopyResult{ID: id, Name: byID[id].Name}
		created, err := CopyTemplate(ctx, src, dst, id, "", activeOnly)
		switch {
		case err != nil:
			res.Error = capitalize(describeCopyError(conn, err))
			if created != nil {
				res.PartialID = created.ID
			}
		default:
			res.NewID = created.ID
			res.Versions = len(created.Versions)
		}
		results = append(results, res)
		if a.emit != nil {
			a.emit(eventCopyProgress, CopyProgress{Done: i + 1, Total: len(ids), Result: res})
		}
	}
	return results, nil
}

// describeCopyError explains why copying a template failed. Access errors
// are attributed to the destination, since the source was already checked.
func describeCopyError(conn Connection, err error) string {
	var apiErr *APIError
	if errors.As(err, &apiErr) && (apiErr.StatusCode == 401 || apiErr.StatusCode == 403) {
		return conn.describeDestError(err)
	}
	return err.Error()
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
