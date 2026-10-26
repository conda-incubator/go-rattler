//go:build cgo

package gorattler

import (
	"fmt"
	"strings"
)

// SubdirFromOCIPlatform maps an OCI platform (os, arch, variant) to a conda
// subdir. The mapping is closed: unknown or ambiguous combinations are
// rejected with ErrInvalidInput. Arm variants other than v6/v7 are rejected
// for Linux arm; Linux arm64 accepts an absent or v8 variant.
func SubdirFromOCIPlatform(os, arch, variant string) (string, error) {
	if strings.TrimSpace(os) == "" || strings.TrimSpace(arch) == "" {
		return "", fmt.Errorf("%w: os and arch are required", ErrInvalidInput)
	}
	var out struct {
		Subdir string `json:"subdir"`
	}
	if err := callJSON(subdirOperation, struct {
		OS      string `json:"os"`
		Arch    string `json:"arch"`
		Variant string `json:"variant"`
	}{os, arch, variant}, &out); err != nil {
		return "", fmt.Errorf("oci platform: %w", err)
	}
	return out.Subdir, nil
}
