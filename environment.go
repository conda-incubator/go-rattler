//go:build cgo

package gorattler

import (
	"fmt"
)

// EnvironmentYAML is a projection of an environment.yaml file. It is parsed
// syntax only; it does not verify that the dependencies resolve or form a lock.
// Variables may be sensitive; callers decide what to log.
type EnvironmentYAML struct {
	Name         string
	Prefix       string
	Channels     []string
	Dependencies []string
	// Subsections maps a subsection name (for example "pip") to its entries,
	// preserving document order. Duplicate subsections are concatenated.
	Subsections map[string][]string
	Variables   map[string]string
}

// ParseEnvironmentYAML parses an environment.yaml document. The caller owns
// doc; it is not retained.
func ParseEnvironmentYAML(doc []byte) (EnvironmentYAML, error) {
	var out struct {
		Name         *string             `json:"name"`
		Prefix       *string             `json:"prefix"`
		Channels     []string            `json:"channels"`
		Dependencies []string            `json:"dependencies"`
		Subsections  map[string][]string `json:"subsections"`
		Variables    map[string]string   `json:"variables"`
	}
	if err := callJSON(environmentOperation, struct {
		Document string `json:"document"`
	}{encodeDocument(doc)}, &out); err != nil {
		return EnvironmentYAML{}, fmt.Errorf("parse environment YAML: %w", err)
	}
	result := EnvironmentYAML{
		Channels:     out.Channels,
		Dependencies: out.Dependencies,
		Subsections:  out.Subsections,
		Variables:    out.Variables,
	}
	if out.Name != nil {
		result.Name = *out.Name
	}
	if out.Prefix != nil {
		result.Prefix = *out.Prefix
	}
	return result, nil
}
