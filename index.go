//go:build cgo

package gorattler

import (
	"encoding/json"
	"fmt"
)

// IndexJSON is the parsed info/index.json of a package archive.
type IndexJSON struct {
	Name        string          `json:"name"`
	Version     string          `json:"version"`
	Build       string          `json:"build"`
	BuildNumber uint64          `json:"build_number"`
	Subdir      string          `json:"subdir,omitempty"`
	Depends     []string        `json:"depends,omitempty"`
	Constrains  []string        `json:"constrains,omitempty"`
	NoArch      json.RawMessage `json:"noarch,omitempty"`
	License     string          `json:"license,omitempty"`
	Timestamp   *int64          `json:"timestamp,omitempty"`
}

// ParseIndexJSON parses an info/index.json document.
func ParseIndexJSON(doc []byte) (IndexJSON, error) {
	var index IndexJSON
	if err := callJSON(indexOperation, documentRequest{Document: encodeDocument(doc)}, &index); err != nil {
		return IndexJSON{}, fmt.Errorf("parse index.json: %w", err)
	}
	return index, nil
}
