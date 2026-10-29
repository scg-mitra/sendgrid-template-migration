package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

const euBaseURL = "https://api.eu.sendgrid.com"

// describeKeyError explains an API key failure in plain words. access is the
// Template Engine access level the key needs ("read" or "full").
func describeKeyError(err error, access string) string {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case 401:
			return "SendGrid rejected this API key. Check it was copied correctly."
		case 403:
			return "this API key needs " + access + " access to Template Engine."
		}
	}
	return err.Error()
}

// Connection describes the two sides templates are copied between.
//
// The source is a backup file (SourceFile), or the account of SourceKey,
// optionally acting on behalf of one of its subusers (SourceSubuser).
//
// The destination is, in order of precedence: nothing, when ExportOnly is
// set; the account of DestKey; SourceKey acting on behalf of the subuser
// Subuser; or, when the source is a subuser, SourceKey's own (parent)
// account.
type Connection struct {
	BaseURL       string `json:"baseURL"`
	SourceKey     string `json:"sourceKey"`
	SourceSubuser string `json:"sourceSubuser"`
	SourceFile    string `json:"sourceFile"`
	DestKey       string `json:"destKey"`
	Subuser       string `json:"subuser"`
	ExportOnly    bool   `json:"exportOnly"`
}

// trimmed returns c with surrounding spaces removed from every field.
func (c Connection) trimmed() Connection {
	c.SourceKey = strings.TrimSpace(c.SourceKey)
	c.SourceSubuser = strings.TrimSpace(c.SourceSubuser)
	c.SourceFile = strings.TrimSpace(c.SourceFile)
	c.DestKey = strings.TrimSpace(c.DestKey)
	c.Subuser = strings.TrimSpace(c.Subuser)
	return c
}

// Validate reports whether enough is set to connect.
func (c Connection) Validate() error {
	c = c.trimmed()
	switch {
	case c.SourceFile == "" && c.SourceKey == "":
		return errors.New("the source API key is required")
	case c.ExportOnly:
		return nil
	case c.DestKey != "":
		return nil
	case c.Subuser != "" && c.SourceKey == "":
		return errors.New("a parent API key is required to copy to a subuser")
	case c.Subuser != "" && strings.EqualFold(c.Subuser, c.SourceSubuser):
		return errors.New("the source and destination are the same subuser")
	case c.Subuser == "" && (c.SourceSubuser == "" || c.SourceFile != ""):
		return errors.New("either a destination API key or a subuser username is required")
	}
	return nil
}

// Source returns where templates are read from: the backup file, or a
// client for the source account.
func (c Connection) Source() (templateSource, error) {
	c = c.trimmed()
	if c.SourceFile != "" {
		return ReadBackup(c.SourceFile)
	}
	src := NewClient(c.BaseURL, c.SourceKey)
	src.OnBehalfOf = c.SourceSubuser
	return src, nil
}

// Destination returns a client for the destination account, or nil when
// exporting only.
func (c Connection) Destination() *Client {
	c = c.trimmed()
	switch {
	case c.ExportOnly:
		return nil
	case c.DestKey != "":
		return NewClient(c.BaseURL, c.DestKey)
	}
	dst := NewClient(c.BaseURL, c.SourceKey)
	dst.OnBehalfOf = c.Subuser
	return dst
}

// SourceLabel describes the source for display.
func (c Connection) SourceLabel() string {
	c = c.trimmed()
	switch {
	case c.SourceFile != "":
		return fmt.Sprintf("the backup %q", filepath.Base(c.SourceFile))
	case c.SourceSubuser != "":
		return fmt.Sprintf("subuser %q", c.SourceSubuser)
	}
	return "the source account"
}

// DestinationLabel describes the destination for display.
func (c Connection) DestinationLabel() string {
	c = c.trimmed()
	switch {
	case c.ExportOnly:
		return "a backup file"
	case c.DestKey != "":
		return "the destination account"
	case c.Subuser != "":
		return fmt.Sprintf("subuser %q", c.Subuser)
	}
	return "the parent account"
}

// describeSourceError explains a failure to read the source account.
func (c Connection) describeSourceError(err error) string {
	var apiErr *APIError
	if s := strings.TrimSpace(c.SourceSubuser); s != "" && errors.As(err, &apiErr) &&
		(apiErr.StatusCode == 401 || apiErr.StatusCode == 403) {
		return fmt.Sprintf("couldn't act as subuser %q. Check the username, and that the parent key has "+
			"read access to Template Engine.", s)
	}
	return describeKeyError(err, "read")
}

// describeDestError explains a failure to reach the destination account.
func (c Connection) describeDestError(err error) string {
	var apiErr *APIError
	if s := strings.TrimSpace(c.Subuser); s != "" && strings.TrimSpace(c.DestKey) == "" && errors.As(err, &apiErr) &&
		(apiErr.StatusCode == 401 || apiErr.StatusCode == 403) {
		return fmt.Sprintf("couldn't act as subuser %q. Check the username, and that the parent key has "+
			"full access to Template Engine.", s)
	}
	return describeKeyError(err, "full")
}

// templateSource is where templates are copied from: a SendGrid account
// (*Client) or a backup file (*Backup).
type templateSource interface {
	templateReader
	ListTemplates(ctx context.Context) ([]Template, error)
}

// connected is the result of a successful connect.
type connected struct {
	src          templateSource
	dst          *Client // nil when exporting only
	srcTemplates []Template
	dstTemplates []Template
}

// connect checks both sides by listing their templates. Errors other than
// cancellation are worded for display to the user.
func connect(ctx context.Context, c Connection) (*connected, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	src, err := c.Source()
	if err != nil {
		return nil, err
	}
	srcTemplates, err := src.ListTemplates(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, err
		}
		return nil, fmt.Errorf("source: %s", c.describeSourceError(err))
	}
	out := &connected{src: src, srcTemplates: srcTemplates}
	if out.dst = c.Destination(); out.dst == nil {
		return out, nil
	}
	if out.dstTemplates, err = out.dst.ListTemplates(ctx); err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, err
		}
		return nil, fmt.Errorf("destination: %s", c.describeDestError(err))
	}
	return out, nil
}

// destNames maps the names of templates to their IDs. When several share a
// name, the first is used.
func destNames(templates []Template) map[string]string {
	m := make(map[string]string, len(templates))
	for _, t := range templates {
		if _, ok := m[t.Name]; !ok {
			m[t.Name] = t.ID
		}
	}
	return m
}
