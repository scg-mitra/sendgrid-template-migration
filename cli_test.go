package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestPrintResult(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		res     CopyResult
		dryRun  bool
		wantOut string
		wantErr string
	}{
		{
			name:    "created",
			res:     CopyResult{ID: "d-1", NewID: "d-2", NewName: "W", Action: ActionCreated, Versions: 2},
			wantOut: "OK   d-1 -> d-2 (\"W\", 2 versions)\n",
		},
		{
			name:    "created dry run",
			res:     CopyResult{ID: "d-1", NewName: "W", Action: ActionCreated, Versions: 1},
			dryRun:  true,
			wantOut: "OK   d-1 -> new template (\"W\", would copy 1 version)\n",
		},
		{
			name:    "added",
			res:     CopyResult{ID: "d-1", NewID: "d-9", NewName: "W", Action: ActionAdded, Versions: 1},
			wantOut: "ADD  d-1 -> d-9 (\"W\", added 1 version)\n",
		},
		{
			name:    "skipped",
			res:     CopyResult{ID: "d-1", NewID: "d-9", NewName: "W", Action: ActionSkipped},
			wantOut: "SKIP d-1: \"W\" already exists as d-9\n",
		},
		{
			name:    "failed with partial",
			res:     CopyResult{ID: "d-1", Error: "Boom", PartialID: "d-3"},
			wantErr: "FAIL d-1: Boom\n     partially created destination template d-3\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out, errOut bytes.Buffer
			printResult(&out, &errOut, tt.res, tt.dryRun)
			if out.String() != tt.wantOut || errOut.String() != tt.wantErr {
				t.Errorf("printResult() wrote %q / %q, want %q / %q", out.String(), errOut.String(), tt.wantOut, tt.wantErr)
			}
		})
	}
}

func TestPrintTemplates(t *testing.T) {
	t.Parallel()
	infos := []TemplateInfo{{ID: "d-1", Name: "Welcome", Generation: "dynamic", Versions: 2}}

	tests := []struct {
		name   string
		asJSON bool
		check  func(t *testing.T, out string)
	}{
		{name: "table", check: func(t *testing.T, out string) {
			lines := strings.Split(strings.TrimSpace(out), "\n")
			if len(lines) != 2 || !strings.HasPrefix(lines[0], "ID") || !strings.Contains(lines[1], "Welcome") {
				t.Errorf("table = %q", out)
			}
		}},
		{name: "json", asJSON: true, check: func(t *testing.T, out string) {
			var got []TemplateInfo
			if err := json.Unmarshal([]byte(out), &got); err != nil || len(got) != 1 || got[0] != infos[0] {
				t.Errorf("JSON = %q (%v)", out, err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			if err := printTemplates(&buf, infos, tt.asJSON); err != nil {
				t.Fatalf("printTemplates() error = %v", err)
			}
			tt.check(t, buf.String())
		})
	}
}

func TestPlural(t *testing.T) {
	t.Parallel()

	tests := []struct {
		n    int
		want string
	}{
		{n: 0, want: "0 versions"},
		{n: 1, want: "1 version"},
		{n: 5, want: "5 versions"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			if got := plural(tt.n, "version"); got != tt.want {
				t.Errorf("plural(%d) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}
