package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestBackupRoundTrip(t *testing.T) {
	t.Parallel()
	_, src := newFakeSendGrid(t, "src", welcome)

	b, err := ExportTemplates(context.Background(), src, []string{welcome.ID})
	if err != nil {
		t.Fatalf("ExportTemplates() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "backup.json")
	if err := WriteBackup(path, b); err != nil {
		t.Fatalf("WriteBackup() error = %v", err)
	}
	got, err := ReadBackup(path)
	if err != nil {
		t.Fatalf("ReadBackup() error = %v", err)
	}
	if got.Format != backupFormat || len(got.Templates) != 1 {
		t.Fatalf("backup = %+v, want format %d with 1 template", got, backupFormat)
	}
	for i, v := range got.Templates[0].Versions {
		if v != welcome.Versions[i] {
			t.Errorf("version %d = %+v, want full content %+v", i, v, welcome.Versions[i])
		}
	}

	// The backup can then be copied from, like an account.
	dstFake, dst := newFakeSendGrid(t, "dst")
	out, err := CopyTemplate(context.Background(), got, dst, welcome.ID, CopyOptions{}, nil)
	if err != nil {
		t.Fatalf("CopyTemplate() from backup error = %v", err)
	}
	if n := len(dstFake.template(out.Template.ID).Versions); n != 2 {
		t.Errorf("copied %d versions from the backup, want 2", n)
	}
}

func TestExportTemplatesError(t *testing.T) {
	t.Parallel()
	_, src := newFakeSendGrid(t, "src", welcome)
	if _, err := ExportTemplates(context.Background(), src, []string{"d-missing"}); err == nil {
		t.Error("ExportTemplates() of an unknown template: want error")
	}
}

func TestReadBackupErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string // "" = file doesn't exist
	}{
		{name: "missing file"},
		{name: "not JSON", content: "hello"},
		{name: "other JSON", content: `{"result":[]}`},
		{name: "newer format", content: `{"format":99,"templates":[]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "b.json")
			if tt.content != "" {
				if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := ReadBackup(path); err == nil {
				t.Error("ReadBackup() error = nil, want error")
			}
		})
	}
}

func TestBackupLookup(t *testing.T) {
	t.Parallel()
	b := &Backup{Format: backupFormat, Templates: []Template{welcome}}

	tests := []struct {
		name       string
		templateID string
		versionID  string
		wantErr    bool
	}{
		{name: "found", templateID: welcome.ID, versionID: "v2"},
		{name: "unknown template", templateID: "d-x", versionID: "v1", wantErr: true},
		{name: "unknown version", templateID: welcome.ID, versionID: "v9", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			v, err := b.GetVersion(context.Background(), tt.templateID, tt.versionID)
			if (err != nil) != tt.wantErr {
				t.Fatalf("GetVersion() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && !errors.Is(err, errNotInBackup) {
				t.Errorf("error = %v, want errNotInBackup", err)
			}
			if !tt.wantErr && v.HTMLContent != "<p>two</p>" {
				t.Errorf("GetVersion() = %+v, want version v2's content", v)
			}
		})
	}
}
