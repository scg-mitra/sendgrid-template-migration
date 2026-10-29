package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

// newTestApp returns an App connected to a source fake holding templates and
// an empty destination fake. Both fakes share one base URL via a router.
func newTestApp(t *testing.T, mode string, templates ...Template) (*App, *fakeSendGrid, *fakeSendGrid, Connection) {
	t.Helper()
	srcFake, src := newFakeSendGrid(t, "parent", templates...)
	dstKey := "subuser-key"
	if mode == "subuser" {
		dstKey = "parent"
	}
	dstFake, dst := newFakeSendGrid(t, dstKey)

	// Route destination traffic (by API key / on-behalf-of) to the second fake.
	toSrc := httputil.NewSingleHostReverseProxy(mustParseURL(t, src.BaseURL))
	toDst := httputil.NewSingleHostReverseProxy(mustParseURL(t, dst.BaseURL))
	router := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("On-Behalf-Of") != "" || r.Header.Get("Authorization") == "Bearer subuser-key" {
			toDst.ServeHTTP(w, r)
			return
		}
		toSrc.ServeHTTP(w, r)
	}))
	t.Cleanup(router.Close)

	conn := Connection{BaseURL: router.URL, SourceKey: "parent"}
	if mode == "subuser" {
		conn.Subuser = "acme"
		dstFake.setSubuser("acme")
	} else {
		conn.DestKey = "subuser-key"
	}
	return &App{defaults: conn}, srcFake, dstFake, conn
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestAppConnect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		mode            string
		wantDestination string
	}{
		{name: "subuser API key", mode: "key", wantDestination: "the destination account"},
		{name: "parent key + username", mode: "subuser", wantDestination: `subuser "acme"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			legacy := Template{ID: "l-1", Name: "aaa legacy", Generation: "legacy"}
			app, _, _, conn := newTestApp(t, tt.mode, welcome, legacy)

			s, err := app.Connect(context.Background(), conn)
			if err != nil {
				t.Fatalf("Connect() error = %v", err)
			}
			if s.Destination != tt.wantDestination || s.DestCount != 0 {
				t.Errorf("session destination %q (%d templates), want %q (0)", s.Destination, s.DestCount, tt.wantDestination)
			}
			names := make([]string, 0, len(s.Templates))
			for _, ti := range s.Templates {
				names = append(names, ti.Name)
			}
			if !slices.Equal(names, []string{"aaa legacy", "Welcome"}) {
				t.Errorf("templates = %v, want sorted case-insensitively", names)
			}
		})
	}
}

func TestAppConnectErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		edit func(c *Connection)
		want string
	}{
		{name: "missing source key", edit: func(c *Connection) { c.SourceKey = "" }, want: "The source API key is required"},
		{name: "wrong source key", edit: func(c *Connection) { c.SourceKey = "nope" }, want: "Source: SendGrid rejected this API key. Check it was copied correctly."},
		{name: "wrong subuser", edit: func(c *Connection) { c.Subuser = "other" }, want: `Destination: couldn't act as subuser "other". Check the username, and that the parent key has full access to Template Engine.`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			app, _, _, conn := newTestApp(t, "subuser", welcome)
			tt.edit(&conn)
			_, err := app.Connect(context.Background(), conn)
			if err == nil || err.Error() != tt.want {
				t.Errorf("Connect() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestAppCopyTemplates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		ids         []string
		dstFail     string
		wantCopied  int
		wantPartial bool
	}{
		{name: "copies and reports progress", ids: []string{welcome.ID}, wantCopied: 1},
		{name: "unknown template fails, others continue", ids: []string{"d-missing", welcome.ID}, wantCopied: 1},
		{name: "failed copy is cleaned up", ids: []string{welcome.ID}, dstFail: "POST /v3/templates/{id}/versions"},
		{name: "partial copy reported when cleanup fails", ids: []string{welcome.ID}, dstFail: "POST /v3/templates/{id}/versions|DELETE /v3/templates/{id}", wantPartial: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			app, _, dstFake, conn := newTestApp(t, "subuser", welcome)
			var mu sync.Mutex
			var events []CopyProgress
			app.emit = func(name string, data any) {
				if name != eventCopyProgress {
					t.Errorf("event %q, want %q", name, eventCopyProgress)
				}
				mu.Lock()
				defer mu.Unlock()
				events = append(events, data.(CopyProgress))
			}
			if _, err := app.Connect(context.Background(), conn); err != nil {
				t.Fatalf("Connect() error = %v", err)
			}
			if tt.dstFail != "" {
				for _, pattern := range strings.Split(tt.dstFail, "|") {
					dstFake.setFail(pattern, http.StatusBadRequest)
				}
			}

			results, err := app.CopyTemplates(context.Background(), tt.ids, CopyOptions{})
			if err != nil {
				t.Fatalf("CopyTemplates() error = %v", err)
			}
			copied := 0
			for _, r := range results {
				if r.Error == "" {
					copied++
					if r.NewID == "" || r.Versions != 2 {
						t.Errorf("result %+v, want a new ID and 2 versions", r)
					}
				}
				if (r.PartialID != "") != tt.wantPartial {
					t.Errorf("partial ID %q, wantPartial %v", r.PartialID, tt.wantPartial)
				}
			}
			if copied != tt.wantCopied {
				t.Errorf("copied %d, want %d", copied, tt.wantCopied)
			}
			mu.Lock()
			defer mu.Unlock()
			if len(events) != len(tt.ids) {
				t.Fatalf("got %d progress events, want %d", len(events), len(tt.ids))
			}
			// Templates are copied concurrently, so events can arrive in any order.
			for i, ev := range events {
				if ev.Done != i+1 || ev.Total != len(tt.ids) || ev.Result != results[ev.Index] {
					t.Errorf("event %d = %+v, want done %d of %d with result %+v", i, ev, i+1, len(tt.ids), results[ev.Index])
				}
			}
		})
	}
}

func TestAppCopyTemplatesNotConnected(t *testing.T) {
	t.Parallel()
	if _, err := (&App{}).CopyTemplates(context.Background(), []string{"d-1"}, CopyOptions{}); err == nil {
		t.Error("CopyTemplates() before Connect: want error")
	}
}

func TestAppListSubusers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		key     string
		status  int
		body    string
		want    []string
		wantErr string
	}{
		{
			name: "sorted, disabled hidden", key: "k", status: 200,
			body: `[{"username":"zeta"},{"username":"Alpha"},{"username":"old","disabled":true}]`,
			want: []string{"Alpha", "zeta"},
		},
		{name: "no key", key: " ", wantErr: "Enter the parent API key first."},
		{name: "rejected key", key: "k", status: 401, body: `{"errors":[{"message":"x"}]}`, wantErr: "SendGrid rejected this API key. Check it was copied correctly."},
		{
			name: "no subuser permission", key: "k", status: 403, body: `{"errors":[{"message":"x"}]}`,
			wantErr: "This key can't list subusers (it needs read access to Subuser Management). You can still type the username.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Errorf("writing response: %v", err)
				}
			}))
			t.Cleanup(srv.Close)

			got, err := (&App{}).ListSubusers(context.Background(), srv.URL, tt.key)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Errorf("ListSubusers() error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ListSubusers() error = %v", err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("ListSubusers() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAppDefaultsRegions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		baseURL string
		want    int
	}{
		{name: "global", baseURL: defaultBaseURL, want: 2},
		{name: "eu", baseURL: euBaseURL, want: 2},
		{name: "custom adds an option", baseURL: "http://localhost:1234", want: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			d := (&App{defaults: Connection{BaseURL: tt.baseURL}}).Defaults()
			if len(d.Regions) != tt.want || d.BaseURL != tt.baseURL || d.Version != version {
				t.Errorf("Defaults() = %+v, want %d regions and base URL %q", d, tt.want, tt.baseURL)
			}
		})
	}
}

func TestCapitalize(t *testing.T) {
	t.Parallel()

	tests := []struct{ in, want string }{
		{"", ""},
		{"source key: bad", "Source key: bad"},
		{"Already", "Already"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			if got := capitalize(tt.in); got != tt.want {
				t.Errorf("capitalize(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestAppConnectSources(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		edit            func(t *testing.T, c *Connection)
		wantSource      string
		wantDestination string
		wantCanCopy     bool
	}{
		{
			name:       "export only",
			edit:       func(_ *testing.T, c *Connection) { c.Subuser, c.ExportOnly = "", true },
			wantSource: "the source account", wantDestination: "a backup file",
		},
		{
			name: "backup file to a key",
			edit: func(t *testing.T, c *Connection) {
				path := filepath.Join(t.TempDir(), "templates.json")
				if err := WriteBackup(path, &Backup{Format: backupFormat, Templates: []Template{welcome}}); err != nil {
					t.Fatal(err)
				}
				c.SourceKey, c.SourceFile = "", path
			},
			wantSource: `the backup "templates.json"`, wantDestination: "the destination account", wantCanCopy: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			app, _, dstFake, conn := newTestApp(t, "key", welcome)
			tt.edit(t, &conn)

			s, err := app.Connect(context.Background(), conn)
			if err != nil {
				t.Fatalf("Connect() error = %v", err)
			}
			if s.Source != tt.wantSource || s.Destination != tt.wantDestination || len(s.Templates) != 1 {
				t.Errorf("session = %+v, want source %q, destination %q and 1 template", s, tt.wantSource, tt.wantDestination)
			}
			_, err = app.CopyTemplates(context.Background(), []string{welcome.ID}, CopyOptions{})
			if (err == nil) != tt.wantCanCopy {
				t.Errorf("CopyTemplates() error = %v, want copying possible: %v", err, tt.wantCanCopy)
			}
			if tt.wantCanCopy && dstFake.templateCount() != 1 {
				t.Errorf("destination has %d templates, want 1", dstFake.templateCount())
			}
		})
	}
}

func TestAppConnectDestNames(t *testing.T) {
	t.Parallel()
	app, _, dstFake, conn := newTestApp(t, "key", welcome)
	for _, name := range []string{"b", "a", "b"} {
		dstFake.createDirect(name)
	}
	s, err := app.Connect(context.Background(), conn)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	if !slices.Equal(s.DestNames, []string{"a", "b"}) || s.DestCount != 3 {
		t.Errorf("dest names %v (count %d), want [a b] (3)", s.DestNames, s.DestCount)
	}
}

func TestAppPreviewTemplate(t *testing.T) {
	t.Parallel()

	noActive := Template{ID: "d-na", Name: "No active", Versions: []Version{{ID: "x1", Name: "draft", HTMLContent: "<b>draft</b>"}}}
	empty := Template{ID: "d-empty", Name: "Empty"}
	tests := []struct {
		name    string
		id      string
		want    *Preview
		wantErr bool
	}{
		{name: "active version", id: welcome.ID, want: &Preview{VersionName: "current", Active: true, Subject: "Hi {{name}}", HTML: "<p>one</p>"}},
		{name: "falls back to first version", id: noActive.ID, want: &Preview{VersionName: "draft", HTML: "<b>draft</b>"}},
		{name: "no versions", id: empty.ID},
		{name: "unknown template", id: "d-missing", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			app, _, _, conn := newTestApp(t, "key", welcome, noActive, empty)
			if _, err := app.Connect(context.Background(), conn); err != nil {
				t.Fatalf("Connect() error = %v", err)
			}
			got, err := app.PreviewTemplate(context.Background(), tt.id)
			if (err != nil) != tt.wantErr {
				t.Fatalf("PreviewTemplate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if (got == nil) != (tt.want == nil) || (got != nil && *got != *tt.want) {
				t.Errorf("PreviewTemplate() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestAppExportTemplates(t *testing.T) {
	t.Parallel()
	app, _, _, conn := newTestApp(t, "key", welcome)
	if _, err := app.Connect(context.Background(), conn); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "out.json")
	n, err := app.ExportTemplates(context.Background(), []string{welcome.ID}, path)
	if err != nil || n != 1 {
		t.Fatalf("ExportTemplates() = %d, %v; want 1, nil", n, err)
	}
	b, err := ReadBackup(path)
	if err != nil || len(b.Templates) != 1 || len(b.Templates[0].Versions) != 2 {
		t.Fatalf("backup = %+v, %v; want 1 template with 2 versions", b, err)
	}
	if _, err := app.ExportTemplates(context.Background(), []string{welcome.ID}, filepath.Join(t.TempDir(), "no", "dir.json")); err == nil {
		t.Error("ExportTemplates() to a missing directory: want error")
	}
}

func TestWriteResultsCSV(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		results []CopyResult
		want    [][]string
	}{
		{name: "header only", want: [][]string{{"name", "old_id", "new_name", "new_id", "result", "versions", "error"}}},
		{
			name: "mixed results, with quoting",
			results: []CopyResult{
				{ID: "d-1", Name: "Hi, you", NewName: "Hi, you", NewID: "d-9", Action: ActionCreated, Versions: 2},
				{ID: "d-2", Name: "Bad", Error: "Boom"},
			},
			want: [][]string{
				{"name", "old_id", "new_name", "new_id", "result", "versions", "error"},
				{"Hi, you", "d-1", "Hi, you", "d-9", "created", "2", ""},
				{"Bad", "d-2", "", "", "failed", "0", "Boom"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "r.csv")
			if err := (&App{}).SaveResultsCSV(tt.results, path); err != nil {
				t.Fatalf("SaveResultsCSV() error = %v", err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			got, err := csv.NewReader(bytes.NewReader(data)).ReadAll()
			if err != nil {
				t.Fatal(err)
			}
			if !slices.EqualFunc(got, tt.want, slices.Equal) {
				t.Errorf("CSV = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConnectionLabels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		conn     Connection
		wantSrc  string
		wantDest string
	}{
		{name: "key to key", conn: Connection{SourceKey: "s", DestKey: "d"}, wantSrc: "the source account", wantDest: "the destination account"},
		{name: "parent to subuser", conn: Connection{SourceKey: "s", Subuser: "acme"}, wantSrc: "the source account", wantDest: `subuser "acme"`},
		{name: "subuser to parent", conn: Connection{SourceKey: "s", SourceSubuser: "acme"}, wantSrc: `subuser "acme"`, wantDest: "the parent account"},
		{name: "file to key", conn: Connection{SourceFile: "/x/y/b.json", DestKey: "d"}, wantSrc: `the backup "b.json"`, wantDest: "the destination account"},
		{name: "export", conn: Connection{SourceKey: "s", ExportOnly: true}, wantSrc: "the source account", wantDest: "a backup file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.conn.SourceLabel(); got != tt.wantSrc {
				t.Errorf("SourceLabel() = %q, want %q", got, tt.wantSrc)
			}
			if got := tt.conn.DestinationLabel(); got != tt.wantDest {
				t.Errorf("DestinationLabel() = %q, want %q", got, tt.wantDest)
			}
		})
	}
}

func TestDescribeSourceError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		conn Connection
		err  error
		want string
	}{
		{name: "subuser forbidden", conn: Connection{SourceSubuser: "acme"}, err: &APIError{StatusCode: 403}, want: `couldn't act as subuser "acme". Check the username, and that the parent key has read access to Template Engine.`},
		{name: "key forbidden", conn: Connection{}, err: &APIError{StatusCode: 403}, want: "this API key needs read access to Template Engine."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.conn.describeSourceError(tt.err); got != tt.want {
				t.Errorf("describeSourceError() = %q, want %q", got, tt.want)
			}
		})
	}
}
