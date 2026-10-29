package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		a, b   string
		want   int
		wantOK bool
	}{
		{a: "1.2.0", b: "1.1.9", want: 1, wantOK: true},
		{a: "v1.10.0", b: "1.9.0", want: 1, wantOK: true},
		{a: "1.0", b: "1.0.0", want: 0, wantOK: true},
		{a: "1.0.0", b: "1.0.1", want: -1, wantOK: true},
		{a: "2.0.0-rc1", b: "1.9.9", want: 1, wantOK: true},
		{a: "dev", b: "1.0.0"},
		{a: "1.0.0", b: "0.0.0-abc123", want: 1, wantOK: true},
		{a: "", b: "1.0.0"},
	}
	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			t.Parallel()
			got, ok := compareVersions(tt.a, tt.b)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("compareVersions(%q, %q) = %d, %v; want %d, %v", tt.a, tt.b, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestFetchLatestRelease(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		status  int
		body    string
		want    UpdateInfo
		wantErr bool
	}{
		{name: "release", status: 200, body: `{"tag_name":"v9.1.0","html_url":"https://example.com/r"}`, want: UpdateInfo{Version: "9.1.0", URL: "https://example.com/r"}},
		{name: "no releases", status: 404, body: `{}`, wantErr: true},
		{name: "bad JSON", status: 200, body: `nope`, wantErr: true},
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

			got, err := fetchLatestRelease(context.Background(), srv.URL)
			if (err != nil) != tt.wantErr {
				t.Fatalf("fetchLatestRelease() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && *got != tt.want {
				t.Errorf("fetchLatestRelease() = %+v, want %+v", *got, tt.want)
			}
		})
	}
}

func TestCheckForUpdateDevBuild(t *testing.T) {
	t.Parallel()
	// Tests run with version "dev", which never reports an update or makes a request.
	got, err := (&App{releasesURL: "http://127.0.0.1:1/never"}).CheckForUpdate(context.Background())
	if got != nil || err != nil {
		t.Errorf("CheckForUpdate() = %+v, %v; want nil, nil for a dev build", got, err)
	}
}
