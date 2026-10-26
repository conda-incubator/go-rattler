//go:build cgo

package gorattler

import (
	"fmt"
	"unicode/utf8"
)

// InstalledPyPI is a caller-provided installed PyPI distribution.
type InstalledPyPI struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	URL      string `json:"url,omitempty"`
	SHA256   string `json:"sha256,omitempty"`
	MD5      string `json:"md5,omitempty"`
	Editable bool   `json:"editable,omitempty"`
}

// InstalledPackages is the installed-side input for CompareLock.
//
// Conda holds full prefix records; only identity fields (name, version, build, build
// number, subdir, URL, SHA256, MD5) take part in comparison. CondaComplete and
// PyPIComplete are caller assertions that the list is the entire installed set for that
// scope. An empty list with a complete flag means "nothing installed". A nil Pypi slice
// means PyPI was not provided.
type InstalledPackages struct {
	Conda         []PrefixRecord
	Pypi          []InstalledPyPI
	CondaComplete bool
	PyPIComplete  bool
}

// InstalledPackage is one installed package in a SyncReport.
type InstalledPackage struct {
	Kind        string  `json:"kind"`
	Name        string  `json:"name"`
	Version     string  `json:"version"`
	Build       string  `json:"build"`
	BuildNumber *uint64 `json:"build_number,omitempty"`
	Subdir      string  `json:"subdir"`
	URL         string  `json:"url"`
	SHA256      string  `json:"sha256"`
	MD5         string  `json:"md5"`
}

// PackageChange is a locked package whose installed counterpart differs.
type PackageChange struct {
	Locked    LockedPackage    `json:"locked"`
	Installed InstalledPackage `json:"installed"`
	Reasons   []string         `json:"reasons"`
}

// ComparisonIssue is a question the comparison could not answer.
type ComparisonIssue struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// SyncReport compares a lock with an installed set.
//
// Scope is "conda" for LockSync and "conda+pypi" for CompareLock; it is set by the entry
// point, never inferred from the lock contents.
// Complete is true only when every compared scope was asserted complete and there are no
// Unknown entries. Matches is true only when Complete is true and there are no Missing,
// Extra, or Changed entries. Missing, Extra, Changed, and Unknown are never nil.
type SyncReport struct {
	Scope    string             `json:"scope"`
	Missing  []LockedPackage    `json:"missing"`
	Extra    []InstalledPackage `json:"extra"`
	Changed  []PackageChange    `json:"changed"`
	Unknown  []ComparisonIssue  `json:"unknown"`
	Complete bool               `json:"complete"`
	Matches  bool               `json:"matches"`
}

// installedCondaIdentity is the wire projection of a PrefixRecord for operation 13.
// File lists and paths are not sent.
type installedCondaIdentity struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Build       string `json:"build"`
	BuildNumber uint64 `json:"build_number"`
	Subdir      string `json:"subdir"`
	URL         string `json:"url"`
	SHA256      string `json:"sha256"`
	MD5         string `json:"md5"`
}

// lockSyncRequest is the JSON payload for operation 13.
type lockSyncRequest struct {
	Scope         string                   `json:"scope"`
	Lock          string                   `json:"lock"`
	Environment   string                   `json:"environment"`
	Subdir        string                   `json:"subdir"`
	Installed     []installedCondaIdentity `json:"installed"`
	CondaComplete bool                     `json:"conda_complete"`
	PyPI          []InstalledPyPI          `json:"pypi"`
	PyPIProvided  bool                     `json:"pypi_provided"`
	PyPIComplete  bool                     `json:"pypi_complete"`
}

// LockSync compares the conda packages of a lock with a complete installed conda record
// list. Scope is "conda": PyPI is outside the comparison and cannot be claimed clean.
func LockSync(lock []byte, environment, subdir string, installed []PrefixRecord) (SyncReport, error) {
	return compareLock("conda", lock, environment, subdir, InstalledPackages{
		Conda:         installed,
		CondaComplete: true,
	})
}

// CompareLock compares a lock with installed conda and PyPI sets. Scope is "conda+pypi".
//
// Identity and hashes follow these rules:
//   - conda names are normalized, versions are compared as conda versions, build must
//     and build number match exactly, and subdir must match unless the lock is noarch;
//   - URLs are not compared, since mirrors and tokens change them; a URL alone never
//     marks a package changed;
//   - SHA256 is compared when both sides have it, MD5 only when both have MD5 and no
//     SHA256 pair exists; missing locked SHA256 remains Unknown even if MD5 matches.
//     A locked hash with no installed counterpart is Unknown, not Changed;
//   - duplicate normalized names on either side, and editable installs, are Unknown.
//
// Completeness is conservative. Complete requires both flags to be asserted and no
// Unknown entries; because unexpected extra packages cannot be ruled out from a lock, a
// caller must assert PyPIComplete for the PyPI scope to count. A hash match establishes
// metadata identity only; it does not verify files on disk. Source references are not
// fetched. Caller errors return an error; comparison gaps are reported as Unknown.
func CompareLock(lock []byte, environment, subdir string, installed InstalledPackages) (SyncReport, error) {
	return compareLock("conda+pypi", lock, environment, subdir, installed)
}

func compareLock(scope string, lock []byte, environment, subdir string, installed InstalledPackages) (SyncReport, error) {
	if !utf8.Valid(lock) {
		return SyncReport{}, fmt.Errorf("%w: lock is not valid UTF-8", ErrInvalidInput)
	}
	if environment == "" {
		return SyncReport{}, fmt.Errorf("%w: environment is required", ErrInvalidInput)
	}
	if subdir == "" {
		return SyncReport{}, fmt.Errorf("%w: subdir is required", ErrInvalidInput)
	}
	conda := make([]installedCondaIdentity, 0, len(installed.Conda))
	for _, record := range installed.Conda {
		conda = append(conda, installedCondaIdentity{
			Name:        record.Name,
			Version:     record.Version,
			Build:       record.Build,
			BuildNumber: record.BuildNumber,
			Subdir:      record.Subdir,
			URL:         record.URL,
			SHA256:      record.SHA256,
			MD5:         record.MD5,
		})
	}
	pypi := installed.Pypi
	if pypi == nil {
		// The wire format requires an array; PyPIProvided carries the nil/empty distinction.
		pypi = []InstalledPyPI{}
	}

	request := lockSyncRequest{
		Scope:         scope,
		Lock:          string(lock),
		Environment:   environment,
		Subdir:        subdir,
		Installed:     conda,
		CondaComplete: installed.CondaComplete,
		PyPI:          pypi,
		PyPIProvided:  installed.Pypi != nil,
		PyPIComplete:  installed.PyPIComplete,
	}
	var report SyncReport
	if err := callJSON(lockSyncOperation, request, &report); err != nil {
		return SyncReport{}, fmt.Errorf("compare lock: %w", err)
	}
	if report.Missing == nil {
		report.Missing = []LockedPackage{}
	}
	if report.Extra == nil {
		report.Extra = []InstalledPackage{}
	}
	if report.Changed == nil {
		report.Changed = []PackageChange{}
	}
	if report.Unknown == nil {
		report.Unknown = []ComparisonIssue{}
	}
	return report, nil
}
