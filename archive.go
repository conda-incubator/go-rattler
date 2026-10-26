//go:build cgo

package gorattler

import "fmt"

// ArchiveType identifies a conda archive format.
type ArchiveType string

const (
	ArchiveTypeConda  ArchiveType = "conda"
	ArchiveTypeTarBz2 ArchiveType = "tar.bz2"
)

// ArchiveIdentifier is a parsed conda archive filename.
type ArchiveIdentifier struct {
	Name     string      `json:"name"`
	Version  string      `json:"version"`
	Build    string      `json:"build"`
	Type     ArchiveType `json:"type"`
	Filename string      `json:"filename"`
}

// ParseArchiveFilename parses a conda archive filename such as
// "name-1.0-build.conda". Paths are rejected; only bare filenames are accepted.
func ParseArchiveFilename(name string) (ArchiveIdentifier, error) {
	var id ArchiveIdentifier
	if err := callJSON(archiveOperation, struct {
		Filename string `json:"filename"`
	}{name}, &id); err != nil {
		return ArchiveIdentifier{}, fmt.Errorf("parse archive filename: %w", err)
	}
	return id, nil
}
