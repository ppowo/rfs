package rfs

import (
	"bytes"
	"errors"

	"golang.org/x/net/html"
)

// ParseHTML decodes a fetched Page as HTML for HTML-backed Flows.
func ParseHTML(page Page) (*html.Node, error) {
	if len(page) == 0 {
		return nil, errors.New("missing page body")
	}
	return html.Parse(bytes.NewReader(page))
}
