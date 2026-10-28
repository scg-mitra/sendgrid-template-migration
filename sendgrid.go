package main

import (
	"bytes"
	"context"
	"encoding/json"
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

// do sends a JSON request and decodes the response into out (if non-nil).
// It retries on 429 rate-limit responses.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return fmt.Errorf("encoding request body: %w", err)
		}
	}

	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, bytes.NewReader(payload))
		if err != nil {
			return fmt.Errorf("building request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		req.Header.Set("Content-Type", "application/json")
		if c.OnBehalfOf != "" {
			req.Header.Set("On-Behalf-Of", c.OnBehalfOf)
		}

		resp, err := c.HTTP.Do(req)
		if err != nil {
			return fmt.Errorf("%s %s: %w", method, path, err)
		}
		data, err := io.ReadAll(resp.Body)
		// The body has been fully read; a close error can't affect the result.
		_ = resp.Body.Close()
		if err != nil {
			return fmt.Errorf("reading response: %w", err)
		}

		if resp.StatusCode == http.StatusTooManyRequests && attempt < 5 {
			wait := c.RetryWait
			if reset, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
				if d := time.Until(time.Unix(reset, 0)); d > 0 {
					wait = d + 500*time.Millisecond
				}
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return newAPIError(method, path, resp, data)
		}
		if out != nil {
			if err := json.Unmarshal(data, out); err != nil {
				return fmt.Errorf("decoding %s %s response: %w", method, path, err)
			}
		}
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

func newAPIError(method, path string, resp *http.Response, body []byte) *APIError {
	e := &APIError{Method: method, Path: path, StatusCode: resp.StatusCode, Status: resp.Status}
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

// CopyTemplate copies the template with the given ID from src to dst and
// returns the newly created template in dst.
// If creating a version fails after the template was created, the partial
// template is returned along with the error.
func CopyTemplate(ctx context.Context, src, dst *Client, templateID, nameOverride string, activeOnly bool) (*Template, error) {
	t, err := src.GetTemplate(ctx, templateID)
	if err != nil {
		return nil, fmt.Errorf("fetching source template %s: %w", templateID, err)
	}

	// Fetch full version content before creating anything in the destination,
	// so a read failure doesn't leave an empty template behind.
	versions := make([]Version, 0, len(t.Versions))
	for _, vs := range t.Versions {
		if activeOnly && vs.Active != 1 {
			continue
		}
		v, err := src.GetVersion(ctx, templateID, vs.ID)
		if err != nil {
			return nil, fmt.Errorf("fetching version %s: %w", vs.ID, err)
		}
		versions = append(versions, *v)
	}

	name := t.Name
	if nameOverride != "" {
		name = nameOverride
	}
	generation := t.Generation
	if generation == "" {
		generation = "dynamic"
	}

	created, err := dst.CreateTemplate(ctx, name, generation)
	if err != nil {
		return nil, fmt.Errorf("creating destination template: %w", err)
	}

	// Create inactive versions first and the active one last so it ends up
	// as the active version in the destination.
	sort.SliceStable(versions, func(i, j int) bool { return versions[i].Active < versions[j].Active })
	for _, v := range versions {
		nv, err := dst.CreateVersion(ctx, created.ID, v)
		if err != nil {
			return created, fmt.Errorf("creating version %q: %w", v.Name, err)
		}
		created.Versions = append(created.Versions, *nv)
	}
	return created, nil
}
