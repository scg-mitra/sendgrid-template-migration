package main

import (
	"context"
	"errors"
	"sync"
)

// copyConcurrency is how many templates are copied at once. SendGrid's rate
// limits are handled by Client, so this mainly bounds the load we create.
const copyConcurrency = 4

// CopyResult is the outcome of copying one template.
type CopyResult struct {
	ID       string `json:"id"`
	Name     string `json:"name"`              // the source template's name
	NewName  string `json:"newName,omitempty"` // the destination template's name
	NewID    string `json:"newID,omitempty"`   // the destination template (created, added to or skipped)
	Action   string `json:"action,omitempty"`  // ActionCreated, ActionAdded or ActionSkipped
	Versions int    `json:"versions"`
	Error    string `json:"error,omitempty"`
	// PartialID is a destination template a failed copy left behind because
	// cleaning it up failed too.
	PartialID string `json:"partialID,omitempty"`
}

// migration copies templates from one source to one destination.
type migration struct {
	conn Connection
	src  templateReader
	dst  *Client

	mu       sync.Mutex
	existing map[string]string // destination template name -> ID
}

// newMigration returns a migration between the sides of c. existing lists
// the templates already in the destination.
func newMigration(conn Connection, src templateReader, dst *Client, existing []Template) *migration {
	return &migration{conn: conn, src: src, dst: dst, existing: destNames(existing)}
}

// copy copies the templates with the given IDs, several at a time, and
// returns a result for each in the same order. report, if not nil, is called
// after each template finishes, from one goroutine at a time; done counts
// the templates finished so far.
//
// If ctx is cancelled, templates not yet started are reported as cancelled
// without being copied.
func (m *migration) copy(ctx context.Context, ids []string, opts CopyOptions, names map[string]string,
	report func(index, done int, res CopyResult),
) []CopyResult {
	results := make([]CopyResult, len(ids))
	var (
		wg       sync.WaitGroup
		reportMu sync.Mutex
		done     int
	)
	finish := func(i int, res CopyResult) {
		results[i] = res
		reportMu.Lock()
		defer reportMu.Unlock()
		done++
		if report != nil {
			report(i, done, res)
		}
	}

	sem := make(chan struct{}, copyConcurrency)
	for i, id := range ids {
		select {
		case <-ctx.Done():
		case sem <- struct{}{}:
			wg.Go(func() {
				defer func() { <-sem }()
				finish(i, m.copyOne(ctx, id, opts, names[id]))
			})
			continue
		}
		finish(i, CopyResult{ID: id, Name: names[id], Error: "Cancelled"})
	}
	wg.Wait()
	return results
}

// copyOne copies a single template. name is its source name, if known, for
// reporting a failure to fetch it.
func (m *migration) copyOne(ctx context.Context, id string, opts CopyOptions, name string) CopyResult {
	if err := ctx.Err(); err != nil {
		return CopyResult{ID: id, Name: name, Error: "Cancelled"}
	}
	m.mu.Lock()
	existing := make(map[string]string, len(m.existing))
	for k, v := range m.existing {
		existing[k] = v
	}
	m.mu.Unlock()

	out, err := CopyTemplate(ctx, m.src, m.dst, id, opts, existing)
	res := CopyResult{ID: id, Name: name, Action: out.Action, Versions: out.Versions}
	if out.SourceName != "" {
		res.Name = out.SourceName
	}
	if err != nil {
		res.Action = ""
		res.Versions = 0
		res.Error = capitalize(describeCopyError(m.conn, err))
		if out.Template != nil {
			res.PartialID = out.Template.ID
		}
		return res
	}
	res.NewID, res.NewName = out.Template.ID, out.Template.Name
	if out.Action == ActionCreated && !opts.DryRun {
		// Later copies with the same name should see this one.
		m.mu.Lock()
		if _, ok := m.existing[res.NewName]; !ok {
			m.existing[res.NewName] = res.NewID
		}
		m.mu.Unlock()
	}
	return res
}

// describeCopyError explains why copying a template failed. Access errors
// are attributed to the destination, since the source was already checked.
func describeCopyError(conn Connection, err error) string {
	if errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) && (apiErr.StatusCode == 401 || apiErr.StatusCode == 403) {
		return conn.describeDestError(err)
	}
	return err.Error()
}
