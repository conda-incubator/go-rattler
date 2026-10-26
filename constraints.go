//go:build cgo

package gorattler

import (
	"fmt"
	"strings"
)

// MatchSpecInfo is the normalized form of a conda MatchSpec. Pinned reports
// whether the spec carries an exact version and build; it is informational
// and does not constitute a lock.
type MatchSpecInfo struct {
	Canonical string
	Name      string
	Version   string
	Build     string
	Pinned    bool
}

type matchSpecParseResult struct {
	Canonical string  `json:"canonical"`
	Name      string  `json:"name"`
	Version   *string `json:"version"`
	Build     *string `json:"build"`
	Pinned    bool    `json:"pinned"`
}

// ParseMatchSpec parses a conda MatchSpec with strict options.
func ParseMatchSpec(spec string) (MatchSpecInfo, error) {
	if strings.TrimSpace(spec) == "" {
		return MatchSpecInfo{}, fmt.Errorf("%w: matchspec must not be empty", ErrInvalidInput)
	}
	var out matchSpecParseResult
	if err := callJSON(matchSpecOperation, struct {
		Mode string `json:"mode"`
		Spec string `json:"spec"`
	}{"parse", spec}, &out); err != nil {
		return MatchSpecInfo{}, fmt.Errorf("parse matchspec: %w", err)
	}
	info := MatchSpecInfo{Canonical: out.Canonical, Name: out.Name, Pinned: out.Pinned}
	if out.Version != nil {
		info.Version = *out.Version
	}
	if out.Build != nil {
		info.Build = *out.Build
	}
	return info, nil
}

// MatchSpecMatches reports whether record satisfies spec. The record must
// contain the full PackageRecord fields. Constraints that PackageRecord
// matching cannot verify (channel, subdir, url, namespace, filename, when,
// extras) are rejected with ErrInvalidInput rather than silently matched.
func MatchSpecMatches(spec string, record PackageRecord) (bool, error) {
	if strings.TrimSpace(spec) == "" {
		return false, fmt.Errorf("%w: matchspec must not be empty", ErrInvalidInput)
	}
	var out struct {
		Matches bool `json:"matches"`
	}
	if err := callJSON(matchSpecOperation, struct {
		Mode   string        `json:"mode"`
		Spec   string        `json:"spec"`
		Record PackageRecord `json:"record"`
	}{"matches", spec, record}, &out); err != nil {
		return false, fmt.Errorf("match matchspec: %w", err)
	}
	return out.Matches, nil
}

// VersionSpecContains reports whether version satisfies the conda version
// spec, parsed with strict options. An empty spec is rejected.
func VersionSpecContains(spec, version string) (bool, error) {
	if strings.TrimSpace(spec) == "" {
		return false, fmt.Errorf("%w: version spec must not be empty", ErrInvalidInput)
	}
	var out struct {
		Contains bool `json:"contains"`
	}
	if err := callJSON(versionSpecOperation, struct {
		Spec    string `json:"spec"`
		Version string `json:"version"`
	}{spec, version}, &out); err != nil {
		return false, fmt.Errorf("version spec: %w", err)
	}
	return out.Contains, nil
}
