//go:build cgo

package gorattler

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

// PackageRecord is the shared conda package record fields.
type PackageRecord struct {
	Name          string          `json:"name"`
	Version       string          `json:"version"`
	Build         string          `json:"build"`
	BuildNumber   uint64          `json:"build_number"`
	Subdir        string          `json:"subdir,omitempty"`
	SHA256        string          `json:"sha256,omitempty"`
	MD5           string          `json:"md5,omitempty"`
	Depends       []string        `json:"depends,omitempty"`
	Constrains    []string        `json:"constrains,omitempty"`
	NoArch        json.RawMessage `json:"noarch,omitempty"`
	License       string          `json:"license,omitempty"`
	LicenseFamily string          `json:"license_family,omitempty"`
	Flags         []string        `json:"flags,omitempty"`
	TrackFeatures []string        `json:"track_features,omitempty"`
	Size          *uint64         `json:"size,omitempty"`
}

// Link describes the package cache source a file was linked from.
type Link struct {
	Source string `json:"source"`
	Type   *int   `json:"type,omitempty"`
}

// PathEntry describes one installed file of a prefix record.
type PathEntry struct {
	RelativePath      string  `json:"_path"`
	OriginalPath      string  `json:"original_path,omitempty"`
	PathType          string  `json:"path_type"`
	NoLink            bool    `json:"no_link,omitempty"`
	SHA256            string  `json:"sha256,omitempty"`
	SHA256InPrefix    string  `json:"sha256_in_prefix,omitempty"`
	SizeInBytes       *uint64 `json:"size_in_bytes,omitempty"`
	PrefixPlaceholder string  `json:"prefix_placeholder,omitempty"`
	FileMode          string  `json:"file_mode,omitempty"`
}

// PathsData is the installed paths section of a prefix record.
type PathsData struct {
	PathsVersion uint64      `json:"paths_version"`
	Paths        []PathEntry `json:"paths"`
}

// PrefixRecord is a package installed in an environment (conda-meta/*.json).
type PrefixRecord struct {
	PackageRecord
	Filename               string    `json:"fn"`
	URL                    string    `json:"url"`
	Channel                string    `json:"channel,omitempty"`
	ExtractedPackageDir    string    `json:"extracted_package_dir,omitempty"`
	PackageTarballFullPath string    `json:"package_tarball_full_path,omitempty"`
	RequestedSpec          string    `json:"requested_spec,omitempty"`
	RequestedSpecs         []string  `json:"requested_specs,omitempty"`
	Files                  []string  `json:"files,omitempty"`
	PathsData              PathsData `json:"paths_data"`
	Link                   *Link     `json:"link,omitempty"`

	// SourceIndex is the position of this record in the input batch. Single
	// parses and collected records carry their position in the input order.
	SourceIndex int `json:"source_index"`
	// SourcePath is the conda-meta path for collected records.
	SourcePath string `json:"source_path,omitempty"`
}

// RecordError reports one failed record in a batch or collection.
type RecordError struct {
	Index   int    `json:"index"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

// PrefixRecords is the result of a batch parse or prefix collection.
type PrefixRecords struct {
	Records []PrefixRecord `json:"records"`
	Errors  []RecordError  `json:"errors"`
}

// MinimalPrefixRecord is the projection of a prefix record used for
// change detection. It is a projection of a full parse, not a sparse parse.
type MinimalPrefixRecord struct {
	Name    string  `json:"name"`
	Version string  `json:"version"`
	Build   string  `json:"build"`
	SHA256  string  `json:"sha256,omitempty"`
	MD5     string  `json:"md5,omitempty"`
	Size    *uint64 `json:"size,omitempty"`
}

type documentRequest struct {
	Document string `json:"document"`
}

// ParsePrefixRecord parses one conda-meta document.
func ParsePrefixRecord(doc []byte) (PrefixRecord, error) {
	var record PrefixRecord
	if err := callJSON(parsePrefixOperation, documentRequest{Document: encodeDocument(doc)}, &record); err != nil {
		return PrefixRecord{}, fmt.Errorf("parse prefix record: %w", err)
	}
	return record, nil
}

// ParsePrefixRecords parses a batch of conda-meta documents. Per-document
// failures are reported in Errors with the failed input index. Successful
// records carry their input index in SourceIndex. Errors are sorted by Index.
func ParsePrefixRecords(docs [][]byte) (PrefixRecords, error) {
	encoded := make([]string, len(docs))
	for i, doc := range docs {
		encoded[i] = encodeDocument(doc)
	}
	var result PrefixRecords
	if err := callJSON(batchPrefixOperation, struct {
		Documents []string `json:"documents"`
	}{encoded}, &result); err != nil {
		return PrefixRecords{}, fmt.Errorf("parse prefix records: %w", err)
	}
	sort.SliceStable(result.Errors, func(i, j int) bool {
		return result.Errors[i].Index < result.Errors[j].Index
	})
	if result.Records == nil {
		result.Records = []PrefixRecord{}
	}
	if result.Errors == nil {
		result.Errors = []RecordError{}
	}
	return result, nil
}

// ParseMinimalPrefixRecord parses a conda-meta document into its minimal
// projection. It uses the full canonical parser, then projects fields.
func ParseMinimalPrefixRecord(doc []byte) (MinimalPrefixRecord, error) {
	var record MinimalPrefixRecord
	if err := callJSON(minimalPrefixOperation, documentRequest{Document: encodeDocument(doc)}, &record); err != nil {
		return MinimalPrefixRecord{}, fmt.Errorf("parse minimal prefix record: %w", err)
	}
	return record, nil
}

// CollectPrefixRecords reads every regular conda-meta/*.json file under a
// prefix directory and parses them. Symlinks and non-regular entries are
// skipped. Per-file failures are reported in Errors with the sorted file index
// and path. Only a missing or invalid conda-meta directory is returned as an
// error. Record entries and conda-meta itself cannot be symlinks; callers still
// control the trusted prefix path, whose ancestors may contain symlinks.
func CollectPrefixRecords(prefix string) (PrefixRecords, error) {
	if prefix == "" {
		return PrefixRecords{}, errors.New("collect prefix records: prefix is required")
	}
	var result PrefixRecords
	if err := callJSON(collectPrefixOperation, struct {
		Prefix string `json:"prefix"`
	}{prefix}, &result); err != nil {
		return PrefixRecords{}, fmt.Errorf("collect prefix records: %w", err)
	}
	return result, nil
}
