//go:build cgo

package gorattler

import (
	"fmt"
	"unicode/utf8"
)

// LockedPackage is one package pinned by a lock file for one environment and platform.
type LockedPackage struct {
	// Kind is "conda" or "pypi".
	Kind        string   `json:"kind"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Build       string   `json:"build"`
	BuildNumber *uint64  `json:"build_number,omitempty"`
	Subdir      string   `json:"subdir"`
	URL         string   `json:"url"`
	SHA256      string   `json:"sha256"`
	MD5         string   `json:"md5"`
	Source      bool     `json:"source"`
	Depends     []string `json:"depends"`
	// Reason explains a source record whose version or identity is not fixed by the lock.
	Reason string `json:"reason,omitempty"`
}

// lockPackagesRequest is the JSON payload for operation 12.
type lockPackagesRequest struct {
	Lock        string `json:"lock"`
	Environment string `json:"environment"`
	Subdir      string `json:"subdir"`
}

// LockPackages returns the packages a lock file pins for one environment and subdir.
//
// The lock must be valid UTF-8 text in a rattler-supported lock format; anything else
// returns an error. Environment and subdir are required and matched exactly (canonical
// subdir string, e.g. "osx-arm64"); no host platform is assumed. Source references are
// not fetched. Results are sorted by kind, name, version, build, and url, and the slice
// is never nil.
func LockPackages(lock []byte, environment, subdir string) ([]LockedPackage, error) {
	if !utf8.Valid(lock) {
		return nil, fmt.Errorf("%w: lock is not valid UTF-8", ErrInvalidInput)
	}
	if environment == "" {
		return nil, fmt.Errorf("%w: environment is required", ErrInvalidInput)
	}
	if subdir == "" {
		return nil, fmt.Errorf("%w: subdir is required", ErrInvalidInput)
	}
	var packages []LockedPackage
	if err := callJSON(lockPackagesOperation, lockPackagesRequest{
		Lock:        string(lock),
		Environment: environment,
		Subdir:      subdir,
	}, &packages); err != nil {
		return nil, fmt.Errorf("lock packages: %w", err)
	}
	if packages == nil {
		packages = []LockedPackage{}
	}
	return packages, nil
}
