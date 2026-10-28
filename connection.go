package main

import (
	"context"
	"errors"
	"fmt"
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

// Connection describes the two accounts templates are copied between.
//
// The destination is reached either with its own API key (DestKey), or by
// the source (parent) key acting on behalf of a subuser (Subuser).
type Connection struct {
	BaseURL   string `json:"baseURL"`
	SourceKey string `json:"sourceKey"`
	DestKey   string `json:"destKey"`
	Subuser   string `json:"subuser"`
}

// Validate reports whether enough is set to connect.
func (c Connection) Validate() error {
	switch {
	case strings.TrimSpace(c.SourceKey) == "":
		return errors.New("the source API key is required")
	case strings.TrimSpace(c.DestKey) == "" && strings.TrimSpace(c.Subuser) == "":
		return errors.New("either a destination API key or a subuser username is required")
	}
	return nil
}

// Clients returns API clients for the source and destination accounts.
func (c Connection) Clients() (src, dst *Client) {
	src = NewClient(c.BaseURL, strings.TrimSpace(c.SourceKey))
	dstKey := strings.TrimSpace(c.DestKey)
	if dstKey == "" {
		dstKey = src.APIKey
	}
	dst = NewClient(c.BaseURL, dstKey)
	dst.OnBehalfOf = strings.TrimSpace(c.Subuser)
	return src, dst
}

// DestinationLabel describes the destination for display.
func (c Connection) DestinationLabel() string {
	if s := strings.TrimSpace(c.Subuser); s != "" {
		return fmt.Sprintf("subuser %q", s)
	}
	return "the destination account"
}

// describeDestError explains a failure to reach the destination account.
func (c Connection) describeDestError(err error) string {
	var apiErr *APIError
	if s := strings.TrimSpace(c.Subuser); s != "" && errors.As(err, &apiErr) &&
		(apiErr.StatusCode == 401 || apiErr.StatusCode == 403) {
		return fmt.Sprintf("couldn't act as subuser %q. Check the username, and that the parent key has "+
			"full access to Template Engine.", s)
	}
	return describeKeyError(err, "full")
}

// connected is the result of a successful connect.
type connected struct {
	src, dst                   *Client
	srcTemplates, dstTemplates []Template
}

// connect checks both sides by listing their templates. Errors other than
// cancellation are worded for display to the user.
func connect(ctx context.Context, c Connection) (*connected, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	src, dst := c.Clients()
	srcTemplates, err := src.ListTemplates(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, err
		}
		return nil, fmt.Errorf("source key: %s", describeKeyError(err, "read"))
	}
	dstTemplates, err := dst.ListTemplates(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, err
		}
		return nil, fmt.Errorf("destination: %s", c.describeDestError(err))
	}
	return &connected{src: src, dst: dst, srcTemplates: srcTemplates, dstTemplates: dstTemplates}, nil
}
