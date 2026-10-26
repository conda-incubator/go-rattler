//go:build cgo

package gorattler

import (
	"fmt"
	"strings"
)

// CanonicalChannel returns the canonical base URL for a conda channel name or
// absolute https URL. Credentials and query strings are removed. Relative and
// local filesystem paths and the "defaults" multichannel are rejected.
func CanonicalChannel(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%w: channel must not be empty", ErrInvalidInput)
	}
	var out struct {
		Canonical string `json:"canonical"`
	}
	if err := callJSON(channelOperation, struct {
		Value string `json:"value"`
	}{value}, &out); err != nil {
		return "", fmt.Errorf("canonical channel: %w", err)
	}
	return out.Canonical, nil
}
