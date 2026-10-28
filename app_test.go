package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"slices"
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
		{name: "wrong source key", edit: func(c *Connection) { c.SourceKey = "nope" }, want: "Source key: SendGrid rejected this API key. Check it was copied correctly."},
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
		{name: "partial copy reported", ids: []string{welcome.ID}, dstFail: "POST /v3/templates/{id}/versions", wantPartial: true},
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
				dstFake.mu.Lock()
				dstFake.fail[tt.dstFail] = http.StatusBadRequest
				dstFake.mu.Unlock()
			}

			results, err := app.CopyTemplates(context.Background(), tt.ids, false)
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
			for i, ev := range events {
				if ev.Done != i+1 || ev.Total != len(tt.ids) || ev.Result != results[i] {
					t.Errorf("event %d = %+v, want done %d of %d with result %+v", i, ev, i+1, len(tt.ids), results[i])
				}
			}
		})
	}
}

func TestAppCopyTemplatesNotConnected(t *testing.T) {
	t.Parallel()
	if _, err := (&App{}).CopyTemplates(context.Background(), []string{"d-1"}, false); err == nil {
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
		{name: "no key", key: " ", wantErr: "Enter the source (parent) API key first."},
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
