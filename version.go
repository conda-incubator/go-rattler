//go:build cgo

// Package gorattler exposes selected conda operations backed by rattler.
package gorattler

import (
	"fmt"

	"github.com/dbast/go-rattler/internal/api"
)

// CompareVersions compares two conda versions, returning -1, 0, or 1.
// Build the native library with Cargo before building this package.
func CompareVersions(a, b string) (int, error) {
	result, status := api.CompareVersions(a, b)
	switch status {
	case 0:
		return result, nil
	case 1:
		return 0, fmt.Errorf("invalid first conda version %q", a)
	case 2:
		return 0, fmt.Errorf("invalid second conda version %q", b)
	default:
		return 0, fmt.Errorf("rattler comparison failed (status %d)", status)
	}
}
