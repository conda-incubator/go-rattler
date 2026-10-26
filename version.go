//go:build cgo

// Package gorattler exposes selected conda operations backed by rattler.
package gorattler

import (
	"fmt"
)

// CompareVersions compares two conda versions, returning -1, 0, or 1.
// Build the native library with Cargo before building this package.
func CompareVersions(a, b string) (int, error) {
	var result int
	if err := callJSON(compareVersionsOperation, struct {
		A string `json:"a"`
		B string `json:"b"`
	}{a, b}, &result); err != nil {
		return 0, fmt.Errorf("compare versions: %w", err)
	}
	return result, nil
}
