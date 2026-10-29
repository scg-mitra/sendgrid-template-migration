package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// latestReleaseURL is the GitHub API endpoint for this app's latest release.
const latestReleaseURL = "https://api.github.com/repos/scg-mitra/sendgrid-template-migration/releases/latest"

// UpdateInfo describes a newer release than the running version.
type UpdateInfo struct {
	Version string `json:"version"`
	URL     string `json:"url"`
}

// CheckForUpdate returns the latest release if it is newer than the running
// version, or nil if it isn't or this is a development build.
func (a *App) CheckForUpdate(ctx context.Context) (*UpdateInfo, error) {
	if version == "dev" || a.releasesURL == "" {
		return nil, nil
	}
	latest, err := fetchLatestRelease(ctx, a.releasesURL)
	if err != nil {
		return nil, err
	}
	if newer, ok := compareVersions(latest.Version, version); !ok || newer <= 0 {
		return nil, nil
	}
	return latest, nil
}

// fetchLatestRelease reads the latest release from the GitHub API at url.
func fetchLatestRelease(ctx context.Context, url string) (*UpdateInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("checking for updates: %w", err)
	}
	defer func() {
		// Only the status and JSON body matter; a close error changes nothing.
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("checking for updates: %s", resp.Status)
	}
	var release struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&release); err != nil {
		return nil, fmt.Errorf("decoding release: %w", err)
	}
	return &UpdateInfo{Version: strings.TrimPrefix(release.TagName, "v"), URL: release.HTMLURL}, nil
}

// compareVersions compares two dotted numeric versions such as "1.2.0" or
// "v1.10", ignoring any "-suffix". It returns -1, 0 or 1, and false if
// either isn't a version.
func compareVersions(a, b string) (int, bool) {
	pa, okA := parseVersion(a)
	pb, okB := parseVersion(b)
	if !okA || !okB {
		return 0, false
	}
	for i := range max(len(pa), len(pb)) {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		switch {
		case x < y:
			return -1, true
		case x > y:
			return 1, true
		}
	}
	return 0, true
}

// parseVersion splits a version like "v1.2.3-rc1" into [1 2 3].
func parseVersion(s string) ([]int, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return nil, false
	}
	fields := strings.Split(s, ".")
	parts := make([]int, 0, len(fields))
	for _, f := range fields {
		n, err := strconv.Atoi(f)
		if err != nil || n < 0 {
			return nil, false
		}
		parts = append(parts, n)
	}
	return parts, true
}
