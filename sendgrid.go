package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.sendgrid.com"

// Template is a SendGrid transactional template. Versions is only populated
// with summaries by the API; use Client.GetVersion for full content.
type Template struct {
	ID         string    `json:"id,omitempty"`
	Name       string    `json:"name"`
	Generation string    `json:"generation,omitempty"`
	Versions   []Version `json:"versions,omitempty"`
}

// Version is one version of a Template, including its content.
type Version struct {
	ID                   string `json:"id,omitempty"`
	TemplateID           string `json:"template_id,omitempty"`
	Active               int    `json:"active"`
	Name                 string `json:"name"`
	Subject              string `json:"subject"`
	HTMLContent          string `json:"html_content,omitempty"`
	PlainContent         string `json:"plain_content,omitempty"`
	GeneratePlainContent bool   `json:"generate_plain_content"`
	Editor               string `json:"editor,omitempty"`
	TestData             string `json:"test_data,omitempty"`
}

// Client is a minimal SendGrid v3 API client authenticated with one API key.
type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
	// RetryWait is how long to wait after a 429 response when SendGrid
	// doesn't say when the rate limit resets.
	RetryWait time.Duration
	// OnBehalfOf, if set, makes a parent account's key act as this subuser
	// (SendGrid's "on-behalf-of" header).
	OnBehalfOf string
}

// NewClient returns a Client for the API at baseURL using apiKey.
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL:   baseURL,
		APIKey:    apiKey,
		HTTP:      &http.Client{Timeout: 30 * time.Second},
		RetryWait: 2 * time.Second,
	}
}

// Retry limits for Client.do.
const (
	maxRateLimitRetries = 5 // 429 responses: the request wasn't processed, so any method is retried
	maxServerRetries    = 3 // 5xx responses and network errors: only idempotent methods are retried
)

// do sends a JSON request and decodes the response into out (if non-nil).
// It retries on 429 rate-limit responses, and for GET and DELETE requests
// also on 5xx responses and network errors.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return fmt.Errorf("encoding request body: %w", err)
		}
	}
	// A POST that failed with a server or network error may still have been
	// applied, so retrying it could create a duplicate.
	idempotent := method == http.MethodGet || method == http.MethodDelete

	rateLimited, serverErrors := 0, 0
	for {
		status, header, data, err := c.send(ctx, method, path, payload)
		retry := false
		switch {
		case err != nil:
			if ctx.Err() != nil || !idempotent || serverErrors >= maxServerRetries {
				return err
			}
			serverErrors++
			retry = true
		case status == http.StatusTooManyRequests && rateLimited < maxRateLimitRetries:
			rateLimited++
			retry = true
		case status >= 500 && idempotent && serverErrors < maxServerRetries:
			serverErrors++
			retry = true
		}
		if retry {
			if err := sleep(ctx, c.retryDelay(status, header, serverErrors)); err != nil {
				return err
			}
			continue
		}

		if status < 200 || status >= 300 {
			return newAPIError(method, path, status, data)
		}
		if out != nil && len(data) > 0 {
			if err := json.Unmarshal(data, out); err != nil {
				return fmt.Errorf("decoding %s %s response: %w", method, path, err)
			}
		}
		return nil
	}
}

// send makes one HTTP request and returns the response status, headers and body.
func (c *Client) send(ctx context.Context, method, path string, payload []byte) (int, http.Header, []byte, error) {
	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	if c.OnBehalfOf != "" {
		req.Header.Set("On-Behalf-Of", c.OnBehalfOf)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("%s %s: %w", method, path, err)
	}
	data, err := io.ReadAll(resp.Body)
	// The body has been fully read; a close error can't affect the result.
	_ = resp.Body.Close()
	if err != nil {
		return 0, nil, nil, fmt.Errorf("reading response: %w", err)
	}
	return resp.StatusCode, resp.Header, data, nil
}

// retryDelay is how long to wait before retrying. After a 429 it honours
// SendGrid's X-RateLimit-Reset header; otherwise it backs off exponentially.
func (c *Client) retryDelay(status int, header http.Header, serverErrors int) time.Duration {
	if status == http.StatusTooManyRequests {
		if reset, err := strconv.ParseInt(header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
			if d := time.Until(time.Unix(reset, 0)); d > 0 {
				return d + 500*time.Millisecond
			}
		}
		return c.RetryWait
	}
	return c.RetryWait << (serverErrors - 1)
}

// sleep waits for d, or returns ctx's error if it is cancelled first.
func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// APIError is a non-2xx response from the SendGrid API.
type APIError struct {
	Method, Path string
	StatusCode   int
	Status       string
	Message      string
}

// Error implements the error interface.
func (e *APIError) Error() string {
	return fmt.Sprintf("%s %s: %s: %s", e.Method, e.Path, e.Status, e.Message)
}

func newAPIError(method, path string, status int, body []byte) *APIError {
	e := &APIError{Method: method, Path: path, StatusCode: status, Status: fmt.Sprintf("%d %s", status, http.StatusText(status))}
	if i := strings.IndexByte(e.Path, '?'); i >= 0 {
		e.Path = e.Path[:i]
	}
	var parsed struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if json.Unmarshal(body, &parsed) == nil && len(parsed.Errors) > 0 {
		msgs := make([]string, len(parsed.Errors))
		for i, m := range parsed.Errors {
			msgs[i] = m.Message
		}
		e.Message = strings.Join(msgs, "; ")
	} else {
		e.Message = strings.TrimSpace(string(body))
	}
	return e
}

// Subuser is a subuser of a parent SendGrid account.
type Subuser struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Disabled bool   `json:"disabled"`
}

// subusersPageSize is the page size used by ListSubusers (SendGrid's max is 500).
const subusersPageSize = 500

// ListSubusers returns every subuser of the account. Only works with a
// parent account key that has Subuser Management access.
func (c *Client) ListSubusers(ctx context.Context) ([]Subuser, error) {
	var all []Subuser
	for offset := 0; ; offset += subusersPageSize {
		query := url.Values{"limit": {strconv.Itoa(subusersPageSize)}, "offset": {strconv.Itoa(offset)}}
		var page []Subuser
		if err := c.do(ctx, http.MethodGet, "/v3/subusers?"+query.Encode(), nil, &page); err != nil {
			return nil, err
		}
		all = append(all, page...)
		if len(page) < subusersPageSize {
			return all, nil
		}
	}
}

// GetTemplate fetches a template and its version summaries.
func (c *Client) GetTemplate(ctx context.Context, id string) (*Template, error) {
	var t Template
	if err := c.do(ctx, http.MethodGet, "/v3/templates/"+url.PathEscape(id), nil, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// ListTemplates returns all dynamic and legacy templates in the account.
func (c *Client) ListTemplates(ctx context.Context) ([]Template, error) {
	var all []Template
	query := url.Values{"generations": {"legacy,dynamic"}, "page_size": {"200"}}
	for {
		var page struct {
			Result   []Template `json:"result"`
			Metadata struct {
				Next string `json:"next"`
			} `json:"_metadata"`
		}
		if err := c.do(ctx, http.MethodGet, "/v3/templates?"+query.Encode(), nil, &page); err != nil {
			return nil, err
		}
		all = append(all, page.Result...)
		if page.Metadata.Next == "" || len(page.Result) == 0 {
			return all, nil
		}
		next, err := url.Parse(page.Metadata.Next)
		if err != nil {
			return nil, fmt.Errorf("parsing next page URL: %w", err)
		}
		token := next.Query().Get("page_token")
		if token == "" || token == query.Get("page_token") {
			return all, nil
		}
		query.Set("page_token", token)
	}
}

// GetVersion fetches a template version with its full content.
func (c *Client) GetVersion(ctx context.Context, templateID, versionID string) (*Version, error) {
	var v Version
	path := "/v3/templates/" + url.PathEscape(templateID) + "/versions/" + url.PathEscape(versionID)
	if err := c.do(ctx, http.MethodGet, path, nil, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// CreateTemplate creates an empty template ("dynamic" or "legacy").
func (c *Client) CreateTemplate(ctx context.Context, name, generation string) (*Template, error) {
	var t Template
	if err := c.do(ctx, http.MethodPost, "/v3/templates", Template{Name: name, Generation: generation}, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// CreateVersion adds a copy of v to the template. v's own IDs are ignored.
func (c *Client) CreateVersion(ctx context.Context, templateID string, v Version) (*Version, error) {
	v.ID, v.TemplateID = "", templateID
	var out Version
	if err := c.do(ctx, http.MethodPost, "/v3/templates/"+url.PathEscape(templateID)+"/versions", v, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteTemplate deletes a template and all of its versions.
func (c *Client) DeleteTemplate(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/v3/templates/"+url.PathEscape(id), nil, nil)
}

// DeleteVersion deletes one version of a template.
func (c *Client) DeleteVersion(ctx context.Context, templateID, versionID string) error {
	path := "/v3/templates/" + url.PathEscape(templateID) + "/versions/" + url.PathEscape(versionID)
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}

// templateReader reads templates with full version content: a SendGrid
// account (*Client) or a backup file (*Backup).
type templateReader interface {
	GetTemplate(ctx context.Context, id string) (*Template, error)
	GetVersion(ctx context.Context, templateID, versionID string) (*Version, error)
}

// The values of CopyOptions.OnExisting: what CopyTemplate does when the
// destination already has a template with the copy's name. They are plain
// strings rather than a named type so the generated TypeScript bindings
// don't use enums.
const (
	ExistingDuplicate   = "duplicate" // create another template with the same name
	ExistingSkip        = "skip"      // leave the destination unchanged
	ExistingAddVersions = "add"       // add the versions to the existing template
)

// ParseExistingPolicy parses a policy name, as given to -on-existing.
func ParseExistingPolicy(s string) (string, error) {
	switch p := strings.ToLower(strings.TrimSpace(s)); p {
	case ExistingDuplicate, ExistingSkip, ExistingAddVersions:
		return p, nil
	case "":
		return ExistingDuplicate, nil
	}
	return "", fmt.Errorf("unknown policy %q (want duplicate, skip or add)", s)
}

// CopyOptions controls how CopyTemplate copies a template.
type CopyOptions struct {
	// Name is the name for the copy. If empty, the copy is named
	// Prefix + the source template's name + Suffix.
	Name       string `json:"name,omitempty"`
	Prefix     string `json:"prefix"`
	Suffix     string `json:"suffix"`
	ActiveOnly bool   `json:"activeOnly"`
	OnExisting string `json:"onExisting"` // ExistingDuplicate (the default), ExistingSkip or ExistingAddVersions
	DryRun     bool   `json:"-"`          // report what would happen without changing the destination
	// Overrides holds choices for individual templates, keyed by source
	// template ID, that replace Name and OnExisting for that template.
	Overrides map[string]CopyOverride `json:"overrides,omitempty"`
}

// CopyOverride replaces CopyOptions.Name and OnExisting for one template.
// Empty fields keep the value from CopyOptions.
type CopyOverride struct {
	Name       string `json:"name,omitempty"`
	OnExisting string `json:"onExisting,omitempty"`
}

// forTemplate returns the options for the template with the given ID, with
// its override, if any, applied.
func (o CopyOptions) forTemplate(id string) CopyOptions {
	ov, ok := o.Overrides[id]
	if !ok {
		return o
	}
	if ov.Name != "" {
		o.Name = ov.Name
	}
	if ov.OnExisting != "" {
		o.OnExisting = ov.OnExisting
	}
	return o
}

// copyName returns the destination name for a source template named name.
func (o CopyOptions) copyName(name string) string {
	if o.Name != "" {
		return o.Name
	}
	return o.Prefix + name + o.Suffix
}

// The values of CopyOutcome.Action and CopyResult.Action: what CopyTemplate
// did with a template.
const (
	ActionCreated = "created" // a new destination template was created
	ActionAdded   = "added"   // versions were added to an existing template
	ActionSkipped = "skipped" // the name already existed, so nothing was done
)

// CopyOutcome describes the result of CopyTemplate.
type CopyOutcome struct {
	SourceName string
	Action     string
	// Template is the destination template: the one created or added to, or
	// the existing one when skipped. Its Versions are those created by the
	// copy. After a failed copy it is set only if a partial copy was left
	// behind because removing it failed too.
	Template *Template
	// Versions is the number of versions copied (or that would be, in a dry run).
	Versions int
}

// cleanupTimeout bounds how long removing a failed partial copy may take.
const cleanupTimeout = 30 * time.Second

// CopyTemplate copies the template with the given ID from src to dst.
// existing maps the names of templates already in dst to their IDs; it is
// consulted according to opts.OnExisting. opts.Overrides for templateID, if
// any, apply.
//
// If creating a version fails, whatever the copy created in dst is removed
// again (even if ctx was cancelled), so a failed copy leaves no trace.
func CopyTemplate(ctx context.Context, src templateReader, dst *Client, templateID string, opts CopyOptions, existing map[string]string) (CopyOutcome, error) {
	var out CopyOutcome
	opts = opts.forTemplate(templateID)
	t, err := src.GetTemplate(ctx, templateID)
	if err != nil {
		return out, fmt.Errorf("fetching source template %s: %w", templateID, err)
	}
	out.SourceName = t.Name
	name := opts.copyName(t.Name)

	out.Action = ActionCreated
	existingID, exists := existing[name]
	if exists {
		switch opts.OnExisting {
		case ExistingSkip:
			out.Action = ActionSkipped
			out.Template = &Template{ID: existingID, Name: name}
			return out, nil
		case ExistingAddVersions:
			out.Action = ActionAdded
		}
	}

	summaries := make([]Version, 0, len(t.Versions))
	for _, vs := range t.Versions {
		if !opts.ActiveOnly || vs.Active == 1 {
			summaries = append(summaries, vs)
		}
	}
	out.Versions = len(summaries)
	if opts.DryRun {
		out.Template = &Template{ID: existingID, Name: name}
		return out, nil
	}

	// Fetch full version content before changing the destination, so a read
	// failure doesn't leave anything behind.
	versions := make([]Version, 0, len(summaries))
	for _, vs := range summaries {
		v, err := src.GetVersion(ctx, templateID, vs.ID)
		if err != nil {
			return out, fmt.Errorf("fetching version %s: %w", vs.ID, err)
		}
		versions = append(versions, *v)
	}

	var target *Template
	if out.Action == ActionAdded {
		target = &Template{ID: existingID, Name: name}
	} else {
		generation := t.Generation
		if generation == "" {
			generation = "dynamic"
		}
		if target, err = dst.CreateTemplate(ctx, name, generation); err != nil {
			return out, fmt.Errorf("creating destination template: %w", err)
		}
		target.Versions = nil
	}

	// Create inactive versions first and the active one last so it ends up
	// as the active version in the destination.
	sort.SliceStable(versions, func(i, j int) bool { return versions[i].Active < versions[j].Active })
	for _, v := range versions {
		nv, err := dst.CreateVersion(ctx, target.ID, v)
		if err != nil {
			err = fmt.Errorf("creating version %q: %w", v.Name, err)
			if cerr := undoCopy(ctx, dst, target, out.Action); cerr != nil {
				out.Template = target
				return out, errors.Join(err, fmt.Errorf("removing the partial copy: %w", cerr))
			}
			return out, err
		}
		target.Versions = append(target.Versions, *nv)
	}
	out.Template = target
	return out, nil
}

// undoCopy removes what a failed copy created in dst: the whole template if
// it was new, or just the added versions.
func undoCopy(ctx context.Context, dst *Client, target *Template, action string) error {
	// Clean up even if the copy failed because ctx was cancelled.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
	defer cancel()
	if action == ActionCreated {
		return dst.DeleteTemplate(ctx, target.ID)
	}
	for _, v := range target.Versions {
		if err := dst.DeleteVersion(ctx, target.ID, v.ID); err != nil {
			return err
		}
	}
	return nil
}
