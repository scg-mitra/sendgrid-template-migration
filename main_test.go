package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"
)

// fakeSendGrid is an in-memory stand-in for the SendGrid templates API.
type fakeSendGrid struct {
	t      *testing.T
	apiKey string

	mu        sync.Mutex
	templates map[string]Template // versions hold full content
	order     []string
	fail      map[string]int // mux pattern -> status code to return
	nextID    int
	calls     []string
	subuser   string // if set, requests must carry On-Behalf-Of: subuser
}

func newFakeSendGrid(t *testing.T, apiKey string, templates ...Template) (*fakeSendGrid, *Client) {
	t.Helper()
	f := &fakeSendGrid{t: t, apiKey: apiKey, templates: map[string]Template{}, fail: map[string]int{}}
	for _, tpl := range templates {
		f.templates[tpl.ID] = tpl
		f.order = append(f.order, tpl.ID)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v3/templates", f.list)
	mux.HandleFunc("GET /v3/templates/{id}", f.get)
	mux.HandleFunc("GET /v3/templates/{id}/versions/{vid}", f.getVersion)
	mux.HandleFunc("POST /v3/templates", f.create)
	mux.HandleFunc("POST /v3/templates/{id}/versions", f.createVersion)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.calls = append(f.calls, r.Method+" "+r.URL.Path)
		f.mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer "+f.apiKey {
			f.writeError(w, http.StatusUnauthorized, "authorization required")
			return
		}
		_, pattern := mux.Handler(r)
		f.mu.Lock()
		status := f.fail[pattern]
		subuser := f.subuser
		f.mu.Unlock()
		if got := r.Header.Get("On-Behalf-Of"); got != subuser {
			f.writeError(w, http.StatusForbidden, "access forbidden")
			return
		}
		if status != 0 {
			f.writeError(w, status, "injected failure")
			return
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return f, NewClient(srv.URL, apiKey)
}

func (f *fakeSendGrid) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		f.t.Errorf("encoding fake response: %v", err)
	}
}

func (f *fakeSendGrid) writeError(w http.ResponseWriter, status int, msg string) {
	f.writeJSON(w, status, map[string]any{"errors": []map[string]string{{"message": msg}}})
}

func (f *fakeSendGrid) list(w http.ResponseWriter, _ *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	result := make([]Template, 0, len(f.order))
	for _, id := range f.order {
		result = append(result, f.templates[id])
	}
	f.writeJSON(w, http.StatusOK, map[string]any{"result": result, "_metadata": map[string]any{}})
}

func (f *fakeSendGrid) get(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	tpl, ok := f.templates[r.PathValue("id")]
	if !ok {
		f.writeError(w, http.StatusNotFound, "template not found")
		return
	}
	// Like SendGrid, the template endpoint returns version summaries only.
	summary := tpl
	summary.Versions = make([]Version, len(tpl.Versions))
	for i, v := range tpl.Versions {
		summary.Versions[i] = Version{ID: v.ID, Active: v.Active, Name: v.Name}
	}
	f.writeJSON(w, http.StatusOK, summary)
}

func (f *fakeSendGrid) getVersion(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, v := range f.templates[r.PathValue("id")].Versions {
		if v.ID == r.PathValue("vid") {
			f.writeJSON(w, http.StatusOK, v)
			return
		}
	}
	f.writeError(w, http.StatusNotFound, "version not found")
}

func (f *fakeSendGrid) create(w http.ResponseWriter, r *http.Request) {
	var in Template
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		f.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	in.ID = fmt.Sprintf("d-new%d", f.nextID)
	f.templates[in.ID] = in
	f.order = append(f.order, in.ID)
	f.writeJSON(w, http.StatusCreated, in)
}

func (f *fakeSendGrid) createVersion(w http.ResponseWriter, r *http.Request) {
	var in Version
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		f.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	id := r.PathValue("id")
	tpl, ok := f.templates[id]
	if !ok {
		f.writeError(w, http.StatusNotFound, "template not found")
		return
	}
	if in.ID != "" || in.TemplateID != id {
		f.t.Errorf("create version: got id %q template_id %q, want empty id and %q", in.ID, in.TemplateID, id)
	}
	if in.Active == 1 { // only one version can be active
		for i := range tpl.Versions {
			tpl.Versions[i].Active = 0
		}
	}
	f.nextID++
	in.ID = "v-new" + strconv.Itoa(f.nextID)
	tpl.Versions = append(tpl.Versions, in)
	f.templates[id] = tpl
	f.writeJSON(w, http.StatusCreated, in)
}

func (f *fakeSendGrid) setSubuser(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.subuser = name
}

func (f *fakeSendGrid) template(id string) Template {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.templates[id]
}

func (f *fakeSendGrid) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

var welcome = Template{ID: "d-abc", Name: "Welcome", Generation: "dynamic", Versions: []Version{
	{ID: "v1", Active: 1, Name: "current", Subject: "Hi {{name}}", HTMLContent: "<p>one</p>", Editor: "code", TestData: `{"name":"x"}`},
	{ID: "v2", Active: 0, Name: "old", Subject: "Old", HTMLContent: "<p>two</p>", PlainContent: "two", Editor: "design"},
}}

func TestCopyTemplate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		source       Template
		nameOverride string
		activeOnly   bool
		wantName     string
		wantVersions []string // names, in creation order
		wantActive   string
	}{
		{
			name:         "all versions, active created last",
			source:       welcome,
			wantName:     "Welcome",
			wantVersions: []string{"old", "current"},
			wantActive:   "current",
		},
		{
			name:         "active version only",
			source:       welcome,
			activeOnly:   true,
			wantName:     "Welcome",
			wantVersions: []string{"current"},
			wantActive:   "current",
		},
		{
			name:         "name override",
			source:       welcome,
			nameOverride: "Welcome (copy)",
			wantName:     "Welcome (copy)",
			wantVersions: []string{"old", "current"},
			wantActive:   "current",
		},
		{
			name: "legacy template keeps generation",
			source: Template{ID: "legacy-1", Name: "Receipt", Generation: "legacy", Versions: []Version{
				{ID: "v1", Active: 1, Name: "only", Subject: "Receipt", HTMLContent: "<%body%>"},
			}},
			wantName:     "Receipt",
			wantVersions: []string{"only"},
			wantActive:   "only",
		},
		{
			name:         "template without versions",
			source:       Template{ID: "d-empty", Name: "Empty", Generation: "dynamic"},
			wantName:     "Empty",
			wantVersions: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, src := newFakeSendGrid(t, "src", tt.source)
			dstFake, dst := newFakeSendGrid(t, "dst")

			got, err := CopyTemplate(context.Background(), src, dst, tt.source.ID, tt.nameOverride, tt.activeOnly)
			if err != nil {
				t.Fatalf("CopyTemplate() error = %v", err)
			}

			stored := dstFake.template(got.ID)
			if stored.Name != tt.wantName || stored.Generation != tt.source.Generation {
				t.Errorf("destination template = %q (%s), want %q (%s)", stored.Name, stored.Generation, tt.wantName, tt.source.Generation)
			}
			names := make([]string, 0, len(stored.Versions))
			active := ""
			for _, v := range stored.Versions {
				names = append(names, v.Name)
				if v.Active == 1 {
					active = v.Name
				}
			}
			if !slices.Equal(names, tt.wantVersions) {
				t.Errorf("versions = %v, want %v", names, tt.wantVersions)
			}
			if active != tt.wantActive {
				t.Errorf("active version = %q, want %q", active, tt.wantActive)
			}
			if len(got.Versions) != len(tt.wantVersions) {
				t.Errorf("returned %d versions, want %d", len(got.Versions), len(tt.wantVersions))
			}

			// Content must survive the copy.
			for _, want := range tt.source.Versions {
				for _, v := range stored.Versions {
					if v.Name == want.Name && (v.Subject != want.Subject || v.HTMLContent != want.HTMLContent ||
						v.PlainContent != want.PlainContent || v.Editor != want.Editor || v.TestData != want.TestData) {
						t.Errorf("version %q content = %+v, want %+v", v.Name, v, want)
					}
				}
			}
		})
	}
}

func TestCopyTemplateErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		templateID  string
		srcFail     string // mux pattern to fail on the source
		dstFail     string // mux pattern to fail on the destination
		wantStatus  int
		wantPartial bool // a destination template was created
		wantDstIdle bool // nothing was sent to the destination
	}{
		{name: "unknown template", templateID: "d-missing", wantStatus: http.StatusNotFound, wantDstIdle: true},
		{name: "source version fetch fails", templateID: welcome.ID, srcFail: "GET /v3/templates/{id}/versions/{vid}", wantStatus: http.StatusInternalServerError, wantDstIdle: true},
		{name: "destination forbids create", templateID: welcome.ID, dstFail: "POST /v3/templates", wantStatus: http.StatusForbidden},
		{name: "version create fails", templateID: welcome.ID, dstFail: "POST /v3/templates/{id}/versions", wantStatus: http.StatusBadRequest, wantPartial: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srcFake, src := newFakeSendGrid(t, "src", welcome)
			dstFake, dst := newFakeSendGrid(t, "dst")
			if tt.srcFail != "" {
				srcFake.fail[tt.srcFail] = tt.wantStatus
			}
			if tt.dstFail != "" {
				dstFake.fail[tt.dstFail] = tt.wantStatus
			}

			got, err := CopyTemplate(context.Background(), src, dst, tt.templateID, "", false)
			var apiErr *APIError
			if !errors.As(err, &apiErr) || apiErr.StatusCode != tt.wantStatus {
				t.Fatalf("CopyTemplate() error = %v, want APIError with status %d", err, tt.wantStatus)
			}
			if (got != nil) != tt.wantPartial {
				t.Errorf("partial template returned = %v, want %v", got != nil, tt.wantPartial)
			}
			if n := dstFake.callCount(); tt.wantDstIdle && n != 0 {
				t.Errorf("destination received %d calls, want 0", n)
			}
		})
	}
}

func TestListTemplates(t *testing.T) {
	t.Parallel()

	page := func(next string, ids ...string) string {
		result := make([]map[string]string, 0, len(ids))
		for _, id := range ids {
			result = append(result, map[string]string{"id": id, "name": "n-" + id})
		}
		b, err := json.Marshal(map[string]any{"result": result, "_metadata": map[string]string{"next": next}})
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}

	tests := []struct {
		name    string
		pages   map[string]string // page_token -> response body
		wantIDs []string
	}{
		{
			name:    "single page",
			pages:   map[string]string{"": page("", "d-1", "d-2")},
			wantIDs: []string{"d-1", "d-2"},
		},
		{
			name: "follows next page token",
			pages: map[string]string{
				"":     page("https://api.sendgrid.com/v3/templates?page_size=200&page_token=tok2", "d-1"),
				"tok2": page("", "d-2"),
			},
			wantIDs: []string{"d-1", "d-2"},
		},
		{
			name: "stops when next repeats the same token",
			pages: map[string]string{
				"":  page("https://x/v3/templates?page_token=a", "d-1"),
				"a": page("https://x/v3/templates?page_token=a", "d-2"),
			},
			wantIDs: []string{"d-1", "d-2"},
		},
		{
			name:    "empty account",
			pages:   map[string]string{"": page("")},
			wantIDs: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if g := r.URL.Query().Get("generations"); g != "legacy,dynamic" {
					t.Errorf("generations = %q, want legacy,dynamic", g)
				}
				body, ok := tt.pages[r.URL.Query().Get("page_token")]
				if !ok {
					t.Errorf("unexpected page_token %q", r.URL.Query().Get("page_token"))
					http.NotFound(w, r)
					return
				}
				if _, err := w.Write([]byte(body)); err != nil {
					t.Errorf("writing response: %v", err)
				}
			}))
			t.Cleanup(srv.Close)

			got, err := NewClient(srv.URL, "k").ListTemplates(context.Background())
			if err != nil {
				t.Fatalf("ListTemplates() error = %v", err)
			}
			ids := make([]string, 0, len(got))
			for _, tpl := range got {
				ids = append(ids, tpl.ID)
			}
			if !slices.Equal(ids, tt.wantIDs) {
				t.Errorf("ListTemplates() IDs = %v, want %v", ids, tt.wantIDs)
			}
		})
	}
}

func TestAPIError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		status      int
		body        string
		wantMessage string
	}{
		{name: "sendgrid error JSON", status: 403, body: `{"errors":[{"field":null,"message":"access forbidden"}]}`, wantMessage: "access forbidden"},
		{name: "multiple errors", status: 400, body: `{"errors":[{"message":"a"},{"message":"b"}]}`, wantMessage: "a; b"},
		{name: "plain text body", status: 502, body: "bad gateway\n", wantMessage: "bad gateway"},
		{name: "empty errors list", status: 500, body: `{"errors":[]}`, wantMessage: `{"errors":[]}`},
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

			_, err := NewClient(srv.URL, "k").ListTemplates(context.Background())
			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("error = %v, want *APIError", err)
			}
			if apiErr.StatusCode != tt.status || apiErr.Message != tt.wantMessage || apiErr.Path != "/v3/templates" {
				t.Errorf("APIError = %+v, want status %d, message %q, path without query", apiErr, tt.status, tt.wantMessage)
			}
		})
	}
}

func TestClientRetriesRateLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		limited   int // number of 429 responses before success
		wantErr   bool
		wantCalls int
	}{
		{name: "succeeds after one 429", limited: 1, wantCalls: 2},
		{name: "succeeds after five 429s", limited: 5, wantCalls: 6},
		{name: "gives up after five retries", limited: 100, wantErr: true, wantCalls: 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var mu sync.Mutex
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				mu.Lock()
				calls++
				n := calls
				mu.Unlock()
				if n <= tt.limited {
					w.Header().Set("X-RateLimit-Reset", "0") // in the past: use RetryWait
					w.WriteHeader(http.StatusTooManyRequests)
					return
				}
				if _, err := w.Write([]byte(`{"result":[]}`)); err != nil {
					t.Errorf("writing response: %v", err)
				}
			}))
			t.Cleanup(srv.Close)

			c := NewClient(srv.URL, "k")
			c.RetryWait = time.Millisecond
			_, err := c.ListTemplates(context.Background())
			if (err != nil) != tt.wantErr {
				t.Fatalf("ListTemplates() error = %v, wantErr %v", err, tt.wantErr)
			}
			mu.Lock()
			defer mu.Unlock()
			if calls != tt.wantCalls {
				t.Errorf("calls = %d, want %d", calls, tt.wantCalls)
			}
		})
	}
}

func TestClientHonoursContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status int
	}{
		{name: "cancelled before request", status: http.StatusOK},
		{name: "cancelled while waiting to retry", status: http.StatusTooManyRequests},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			}))
			t.Cleanup(srv.Close)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c := NewClient(srv.URL, "k")
			c.RetryWait = time.Hour
			if tt.status == http.StatusOK {
				cancel()
			} else {
				time.AfterFunc(20*time.Millisecond, cancel)
			}
			_, err := c.ListTemplates(ctx)
			if !errors.Is(err, context.Canceled) {
				t.Errorf("error = %v, want context.Canceled", err)
			}
		})
	}
}

func TestDescribeKeyError(t *testing.T) {
	t.Parallel()

	network := errors.New("dial tcp: connection refused")
	tests := []struct {
		name   string
		err    error
		access string
		want   string
	}{
		{name: "401", err: &APIError{StatusCode: 401}, access: "read", want: "SendGrid rejected this API key. Check it was copied correctly."},
		{name: "403 read", err: &APIError{StatusCode: 403}, access: "read", want: "this API key needs read access to Template Engine."},
		{name: "403 wrapped", err: fmt.Errorf("listing: %w", &APIError{StatusCode: 403}), access: "full", want: "this API key needs full access to Template Engine."},
		{name: "other API error", err: &APIError{Method: "GET", Path: "/v3/templates", StatusCode: 500, Status: "500 Internal Server Error", Message: "boom"}, access: "read", want: "GET /v3/templates: 500 Internal Server Error: boom"},
		{name: "network error", err: network, access: "read", want: network.Error()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := describeKeyError(tt.err, tt.access); got != tt.want {
				t.Errorf("describeKeyError() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestListFlagSet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		values []string
		want   []string
	}{
		{name: "single", values: []string{"d-1"}, want: []string{"d-1"}},
		{name: "comma separated with spaces", values: []string{"d-1, d-2 ,d-3"}, want: []string{"d-1", "d-2", "d-3"}},
		{name: "repeated flag", values: []string{"d-1", "d-2,d-3"}, want: []string{"d-1", "d-2", "d-3"}},
		{name: "empty entries dropped", values: []string{",d-1,,", " "}, want: []string{"d-1"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var l listFlag
			for _, v := range tt.values {
				if err := l.Set(v); err != nil {
					t.Fatalf("Set(%q) error = %v", v, err)
				}
			}
			if !slices.Equal(l, tt.want) {
				t.Errorf("listFlag = %v, want %v", l, tt.want)
			}
		})
	}
}

func TestStripPSNArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "no args", args: []string{"app"}, want: []string{"app"}},
		{name: "psn removed", args: []string{"app", "-psn_0_12345"}, want: []string{"app"}},
		{name: "other flags kept", args: []string{"app", "-tui", "-psn_0_1", "-template", "d-1"}, want: []string{"app", "-tui", "-template", "d-1"}},
		{name: "program name never removed", args: []string{"-psn_weird"}, want: []string{"-psn_weird"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := stripPSNArgs(tt.args); !slices.Equal(got, tt.want) {
				t.Errorf("stripPSNArgs(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestConnectionValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		conn    Connection
		wantErr bool
	}{
		{name: "subuser key", conn: Connection{SourceKey: "src", DestKey: "dst"}},
		{name: "parent key + username", conn: Connection{SourceKey: "src", Subuser: "acme"}},
		{name: "missing source key", conn: Connection{DestKey: "dst"}, wantErr: true},
		{name: "blank source key", conn: Connection{SourceKey: "  ", Subuser: "acme"}, wantErr: true},
		{name: "no destination", conn: Connection{SourceKey: "src"}, wantErr: true},
		{name: "blank username", conn: Connection{SourceKey: "src", Subuser: " "}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := tt.conn.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConnectionClients(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		conn           Connection
		wantDstKey     string
		wantOnBehalfOf string
	}{
		{name: "subuser key", conn: Connection{SourceKey: "src", DestKey: " dst "}, wantDstKey: "dst"},
		{name: "parent key acts as subuser", conn: Connection{SourceKey: "src", Subuser: " acme "}, wantDstKey: "src", wantOnBehalfOf: "acme"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			src, dst := tt.conn.Clients()
			if src.APIKey != "src" || src.OnBehalfOf != "" {
				t.Errorf("source client key %q on-behalf-of %q, want src and none", src.APIKey, src.OnBehalfOf)
			}
			if dst.APIKey != tt.wantDstKey || dst.OnBehalfOf != tt.wantOnBehalfOf {
				t.Errorf("destination client key %q on-behalf-of %q, want %q and %q",
					dst.APIKey, dst.OnBehalfOf, tt.wantDstKey, tt.wantOnBehalfOf)
			}
		})
	}
}

func TestCopyTemplateOnBehalfOfSubuser(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		subuser    string // username the app sends
		wantStatus int    // 0 = success
	}{
		{name: "correct subuser", subuser: "acme"},
		{name: "wrong subuser", subuser: "other", wantStatus: http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, src := newFakeSendGrid(t, "parent", welcome)
			dstFake, dstClient := newFakeSendGrid(t, "parent")
			dstFake.setSubuser("acme")

			conn := Connection{BaseURL: src.BaseURL, SourceKey: "parent", Subuser: tt.subuser}
			_, dst := conn.Clients()
			dst.BaseURL = dstClient.BaseURL // the subuser lives on the second fake server

			got, err := CopyTemplate(context.Background(), src, dst, welcome.ID, "", false)
			if tt.wantStatus == 0 {
				if err != nil {
					t.Fatalf("CopyTemplate() error = %v", err)
				}
				if n := len(dstFake.template(got.ID).Versions); n != 2 {
					t.Errorf("subuser template has %d versions, want 2", n)
				}
				return
			}
			var apiErr *APIError
			if !errors.As(err, &apiErr) || apiErr.StatusCode != tt.wantStatus {
				t.Fatalf("CopyTemplate() error = %v, want status %d", err, tt.wantStatus)
			}
		})
	}
}

func TestDescribeDestError(t *testing.T) {
	t.Parallel()

	subuserMsg := `couldn't act as subuser "acme". Check the username, and that the parent key has full access to Template Engine.`
	tests := []struct {
		name string
		conn Connection
		err  error
		want string
	}{
		{name: "subuser 403", conn: Connection{Subuser: "acme"}, err: &APIError{StatusCode: 403}, want: subuserMsg},
		{name: "subuser 401", conn: Connection{Subuser: "acme"}, err: &APIError{StatusCode: 401}, want: subuserMsg},
		{name: "subuser key 403", conn: Connection{DestKey: "k"}, err: &APIError{StatusCode: 403}, want: "this API key needs full access to Template Engine."},
		{name: "subuser other error", conn: Connection{Subuser: "acme"}, err: errors.New("timeout"), want: "timeout"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.conn.describeDestError(tt.err); got != tt.want {
				t.Errorf("describeDestError() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestListSubusers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		total     int
		wantCalls int
	}{
		{name: "none", total: 0, wantCalls: 1},
		{name: "one page", total: 3, wantCalls: 1},
		{name: "exactly one full page", total: subusersPageSize, wantCalls: 2},
		{name: "two pages", total: subusersPageSize + 7, wantCalls: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var mu sync.Mutex
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				calls++
				mu.Unlock()
				if r.URL.Path != "/v3/subusers" {
					t.Errorf("path = %q, want /v3/subusers", r.URL.Path)
				}
				offset, err := strconv.Atoi(r.URL.Query().Get("offset"))
				if err != nil {
					t.Errorf("bad offset: %v", err)
				}
				limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
				if err != nil {
					t.Errorf("bad limit: %v", err)
				}
				page := make([]Subuser, 0, limit)
				for i := offset; i < min(offset+limit, tt.total); i++ {
					page = append(page, Subuser{ID: i, Username: "user" + strconv.Itoa(i)})
				}
				if err := json.NewEncoder(w).Encode(page); err != nil {
					t.Errorf("encoding: %v", err)
				}
			}))
			t.Cleanup(srv.Close)

			got, err := NewClient(srv.URL, "k").ListSubusers(context.Background())
			if err != nil {
				t.Fatalf("ListSubusers() error = %v", err)
			}
			if len(got) != tt.total {
				t.Errorf("got %d subusers, want %d", len(got), tt.total)
			}
			mu.Lock()
			defer mu.Unlock()
			if calls != tt.wantCalls {
				t.Errorf("calls = %d, want %d", calls, tt.wantCalls)
			}
		})
	}
}
