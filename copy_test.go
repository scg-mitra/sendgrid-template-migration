package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestCopyOptionsCopyName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opts CopyOptions
		want string
	}{
		{name: "unchanged", opts: CopyOptions{}, want: "Welcome"},
		{name: "prefix and suffix", opts: CopyOptions{Prefix: "[EU] ", Suffix: " (copy)"}, want: "[EU] Welcome (copy)"},
		{name: "name wins over prefix", opts: CopyOptions{Name: "Hello", Prefix: "x"}, want: "Hello"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.opts.copyName("Welcome"); got != tt.want {
				t.Errorf("copyName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseExistingPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "", want: ExistingDuplicate},
		{in: "skip", want: ExistingSkip},
		{in: " ADD ", want: ExistingAddVersions},
		{in: "duplicate", want: ExistingDuplicate},
		{in: "overwrite", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			got, err := ParseExistingPolicy(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseExistingPolicy(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ParseExistingPolicy(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCopyTemplateExisting(t *testing.T) {
	t.Parallel()

	existingTpl := Template{ID: "d-old", Name: "Welcome", Generation: "dynamic", Versions: []Version{
		{ID: "v-old", Active: 1, Name: "original", Subject: "Old"},
	}}

	tests := []struct {
		name          string
		opts          CopyOptions
		dstFail       string
		wantAction    string
		wantID        string // destination template ID, "" = a new one
		wantDstCount  int    // templates in the destination afterwards
		wantVersions  []string
		wantActive    string
		wantErr       bool
		wantDstWrites bool
	}{
		{
			name: "duplicate creates another", opts: CopyOptions{OnExisting: ExistingDuplicate},
			wantAction: ActionCreated, wantDstCount: 2, wantVersions: []string{"old", "current"}, wantActive: "current", wantDstWrites: true,
		},
		{
			name: "skip leaves destination alone", opts: CopyOptions{OnExisting: ExistingSkip},
			wantAction: ActionSkipped, wantID: "d-old", wantDstCount: 1, wantVersions: []string{"original"}, wantActive: "original",
		},
		{
			name: "add puts versions on the existing template", opts: CopyOptions{OnExisting: ExistingAddVersions},
			wantAction: ActionAdded, wantID: "d-old", wantDstCount: 1, wantVersions: []string{"original", "old", "current"}, wantActive: "current", wantDstWrites: true,
		},
		{
			name: "renamed copy doesn't clash", opts: CopyOptions{OnExisting: ExistingSkip, Suffix: " v2"},
			wantAction: ActionCreated, wantDstCount: 2, wantVersions: []string{"old", "current"}, wantActive: "current", wantDstWrites: true,
		},
		{
			name: "dry run changes nothing", opts: CopyOptions{OnExisting: ExistingAddVersions, DryRun: true},
			wantAction: ActionAdded, wantID: "d-old", wantDstCount: 1, wantVersions: []string{"original"}, wantActive: "original",
		},
		{
			name: "failed add removes only the added versions", opts: CopyOptions{OnExisting: ExistingAddVersions},
			dstFail: "fail-second-version", wantErr: true, wantDstCount: 1, wantVersions: []string{"original"}, wantActive: "original", wantDstWrites: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, src := newFakeSendGrid(t, "src", welcome)
			dstFake, dst := newFakeSendGrid(t, "dst", existingTpl)
			if tt.dstFail != "" {
				// Let the first version through, then fail.
				var mu sync.Mutex
				calls := 0
				inner := dst.HTTP.Transport
				if inner == nil {
					inner = http.DefaultTransport
				}
				dst.HTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					if r.Method == http.MethodPost && r.URL.Path == "/v3/templates/d-old/versions" {
						mu.Lock()
						calls++
						n := calls
						mu.Unlock()
						if n == 2 {
							rec := httptest.NewRecorder()
							rec.WriteHeader(http.StatusBadRequest)
							return rec.Result(), nil
						}
					}
					return inner.RoundTrip(r)
				})}
			}
			before := dstFake.callCount()

			out, err := CopyTemplate(context.Background(), src, dst, welcome.ID, tt.opts, destNames([]Template{existingTpl}))
			if (err != nil) != tt.wantErr {
				t.Fatalf("CopyTemplate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if out.Action != tt.wantAction {
					t.Errorf("action = %q, want %q", out.Action, tt.wantAction)
				}
				if tt.wantID != "" && out.Template.ID != tt.wantID {
					t.Errorf("destination template = %q, want %q", out.Template.ID, tt.wantID)
				}
			}
			if n := dstFake.templateCount(); n != tt.wantDstCount {
				t.Errorf("destination has %d templates, want %d", n, tt.wantDstCount)
			}
			if wrote := dstFake.callCount() > before; wrote != tt.wantDstWrites {
				t.Errorf("destination written = %v, want %v", wrote, tt.wantDstWrites)
			}

			check := dstFake.template("d-old")
			if tt.wantID == "" && !tt.wantErr {
				check = dstFake.template(out.Template.ID)
			}
			names := make([]string, 0, len(check.Versions))
			active := ""
			for _, v := range check.Versions {
				names = append(names, v.Name)
				if v.Active == 1 {
					active = v.Name
				}
			}
			if !slices.Equal(names, tt.wantVersions) || active != tt.wantActive {
				t.Errorf("versions = %v (active %q), want %v (active %q)", names, active, tt.wantVersions, tt.wantActive)
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCopyTemplateCleansUpAfterCancel(t *testing.T) {
	t.Parallel()
	_, src := newFakeSendGrid(t, "src", welcome)
	dstFake, dst := newFakeSendGrid(t, "dst")

	// Cancel the copy as soon as the destination template has been created.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inner := http.DefaultTransport
	dst.HTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		resp, err := inner.RoundTrip(r)
		if r.Method == http.MethodPost && r.URL.Path == "/v3/templates" {
			cancel()
		}
		return resp, err
	})}

	_, err := CopyTemplate(ctx, src, dst, welcome.ID, CopyOptions{}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CopyTemplate() error = %v, want context.Canceled", err)
	}
	if n := dstFake.templateCount(); n != 0 {
		t.Errorf("destination has %d templates after a cancelled copy, want 0", n)
	}
}

func TestClientRetriesServerErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		method    string
		failures  int // 503 responses before success
		wantErr   bool
		wantCalls int
	}{
		{name: "GET retried", method: http.MethodGet, failures: 2, wantCalls: 3},
		{name: "GET gives up", method: http.MethodGet, failures: 100, wantErr: true, wantCalls: maxServerRetries + 1},
		{name: "DELETE retried", method: http.MethodDelete, failures: 1, wantCalls: 2},
		{name: "POST not retried", method: http.MethodPost, failures: 1, wantErr: true, wantCalls: 1},
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
				if n <= tt.failures {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				if _, err := w.Write([]byte(`{}`)); err != nil {
					t.Errorf("writing response: %v", err)
				}
			}))
			t.Cleanup(srv.Close)

			c := NewClient(srv.URL, "k")
			c.RetryWait = time.Millisecond
			err := c.do(context.Background(), tt.method, "/v3/x", nil, nil)
			if (err != nil) != tt.wantErr {
				t.Fatalf("do() error = %v, wantErr %v", err, tt.wantErr)
			}
			mu.Lock()
			defer mu.Unlock()
			if calls != tt.wantCalls {
				t.Errorf("calls = %d, want %d", calls, tt.wantCalls)
			}
		})
	}
}

func TestClientRetriesNetworkErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		method    string
		wantCalls int
	}{
		{name: "GET retried", method: http.MethodGet, wantCalls: maxServerRetries + 1},
		{name: "POST not retried", method: http.MethodPost, wantCalls: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var mu sync.Mutex
			calls := 0
			c := NewClient("http://example.invalid", "k")
			c.RetryWait = time.Millisecond
			c.HTTP = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				mu.Lock()
				defer mu.Unlock()
				calls++
				return nil, errors.New("connection reset")
			})}
			if err := c.do(context.Background(), tt.method, "/v3/x", nil, nil); err == nil {
				t.Fatal("do() error = nil, want network error")
			}
			mu.Lock()
			defer mu.Unlock()
			if calls != tt.wantCalls {
				t.Errorf("calls = %d, want %d", calls, tt.wantCalls)
			}
		})
	}
}

func TestMigrationCopy(t *testing.T) {
	t.Parallel()

	many := make([]Template, 0, 10)
	ids := make([]string, 0, 10)
	for i := range 10 {
		id := "d-" + strconv.Itoa(i)
		many = append(many, Template{ID: id, Name: "T" + strconv.Itoa(i), Versions: []Version{{ID: "v", Active: 1, Name: "v"}}})
		ids = append(ids, id)
	}

	tests := []struct {
		name          string
		ids           []string
		cancelled     bool
		wantCopied    int
		wantCancelled int
	}{
		{name: "all copied in order", ids: ids, wantCopied: 10},
		{name: "unknown id fails alone", ids: append([]string{"d-nope"}, ids[:3]...), wantCopied: 3},
		{name: "cancelled before start", ids: ids, cancelled: true, wantCancelled: 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, src := newFakeSendGrid(t, "src", many...)
			dstFake, dst := newFakeSendGrid(t, "dst")
			m := newMigration(Connection{}, src, dst, nil)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tt.cancelled {
				cancel()
			}
			var dones []int
			seen := map[int]bool{}
			results := m.copy(ctx, tt.ids, CopyOptions{OnExisting: ExistingDuplicate}, nil, func(index, done int, res CopyResult) {
				dones = append(dones, done) // report calls are serialised
				if seen[index] || res.ID != tt.ids[index] {
					t.Errorf("report(%d) for %q: duplicate or wrong index", index, res.ID)
				}
				seen[index] = true
			})

			copied, cancelledN := 0, 0
			for i, r := range results {
				if r.ID != tt.ids[i] {
					t.Errorf("result %d is for %q, want %q", i, r.ID, tt.ids[i])
				}
				switch {
				case r.Error == "Cancelled":
					cancelledN++
				case r.Error == "" && r.Action == ActionCreated && r.NewID != "":
					copied++
				}
			}
			if copied != tt.wantCopied || cancelledN != tt.wantCancelled {
				t.Errorf("copied %d, cancelled %d; want %d, %d", copied, cancelledN, tt.wantCopied, tt.wantCancelled)
			}
			if n := dstFake.templateCount(); n != tt.wantCopied {
				t.Errorf("destination has %d templates, want %d", n, tt.wantCopied)
			}
			want := make([]int, len(tt.ids))
			for i := range want {
				want[i] = i + 1
			}
			if !slices.Equal(dones, want) {
				t.Errorf("done counts = %v, want %v", dones, want)
			}
		})
	}
}

func TestMigrationCopySeesEarlierCopies(t *testing.T) {
	t.Parallel()
	_, src := newFakeSendGrid(t, "src", welcome)
	dstFake, dst := newFakeSendGrid(t, "dst")
	m := newMigration(Connection{}, src, dst, nil)

	first := m.copy(context.Background(), []string{welcome.ID}, CopyOptions{OnExisting: ExistingSkip}, nil, nil)
	second := m.copy(context.Background(), []string{welcome.ID}, CopyOptions{OnExisting: ExistingSkip}, nil, nil)
	if first[0].Action != ActionCreated || second[0].Action != ActionSkipped || second[0].NewID != first[0].NewID {
		t.Errorf("first %+v, second %+v; want created then skipped as the same template", first[0], second[0])
	}
	if n := dstFake.templateCount(); n != 1 {
		t.Errorf("destination has %d templates, want 1", n)
	}
}

func TestDescribeCopyError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "cancelled", err: context.Canceled, want: "cancelled"},
		{name: "forbidden is about the destination", err: &APIError{StatusCode: 403}, want: "this API key needs full access to Template Engine."},
		{name: "other", err: errors.New("boom"), want: "boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := describeCopyError(Connection{DestKey: "k"}, tt.err); got != tt.want {
				t.Errorf("describeCopyError() = %q, want %q", got, tt.want)
			}
		})
	}
}
