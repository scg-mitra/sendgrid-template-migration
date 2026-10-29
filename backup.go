package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

// backupFormat is the version of the backup file format written by
// WriteBackup. ReadBackup rejects files with a newer format.
const backupFormat = 1

// Backup is a file holding templates with the full content of their
// versions. It can be read back as the source of a copy.
type Backup struct {
	Format     int        `json:"format"`
	ExportedAt time.Time  `json:"exportedAt"`
	Templates  []Template `json:"templates"`
}

// ExportTemplates fetches the templates with the given IDs, with every
// version's full content, from src.
func ExportTemplates(ctx context.Context, src templateReader, ids []string) (*Backup, error) {
	b := &Backup{Format: backupFormat, ExportedAt: time.Now().UTC(), Templates: make([]Template, 0, len(ids))}
	for _, id := range ids {
		t, err := src.GetTemplate(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("fetching template %s: %w", id, err)
		}
		versions := make([]Version, 0, len(t.Versions))
		for _, vs := range t.Versions {
			v, err := src.GetVersion(ctx, id, vs.ID)
			if err != nil {
				return nil, fmt.Errorf("fetching version %s of template %s: %w", vs.ID, id, err)
			}
			versions = append(versions, *v)
		}
		t.Versions = versions
		b.Templates = append(b.Templates, *t)
	}
	return b, nil
}

// WriteBackup writes b to the file at path, replacing it if it exists.
func WriteBackup(path string, b *Backup) error {
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding backup: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("writing backup: %w", err)
	}
	return nil
}

// ReadBackup reads a backup file written by WriteBackup.
func ReadBackup(path string) (*Backup, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading backup: %w", err)
	}
	var b Backup
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("%s isn't a template backup file: %w", path, err)
	}
	switch {
	case b.Format == 0:
		return nil, fmt.Errorf("%s isn't a template backup file", path)
	case b.Format > backupFormat:
		return nil, fmt.Errorf("%s was written by a newer version of this app; update it to read the file", path)
	}
	return &b, nil
}

// errNotInBackup is returned when a template or version isn't in a Backup.
var errNotInBackup = errors.New("not found in the backup file")

// ListTemplates returns the templates in the backup. It never fails; the
// signature matches Client.ListTemplates.
func (b *Backup) ListTemplates(context.Context) ([]Template, error) {
	return b.Templates, nil
}

// GetTemplate returns the template with the given ID.
func (b *Backup) GetTemplate(_ context.Context, id string) (*Template, error) {
	for i := range b.Templates {
		if b.Templates[i].ID == id {
			t := b.Templates[i]
			return &t, nil
		}
	}
	return nil, fmt.Errorf("template %s: %w", id, errNotInBackup)
}

// GetVersion returns one version of a template, with its content.
func (b *Backup) GetVersion(ctx context.Context, templateID, versionID string) (*Version, error) {
	t, err := b.GetTemplate(ctx, templateID)
	if err != nil {
		return nil, err
	}
	for _, v := range t.Versions {
		if v.ID == versionID {
			return &v, nil
		}
	}
	return nil, fmt.Errorf("version %s of template %s: %w", versionID, templateID, errNotInBackup)
}
