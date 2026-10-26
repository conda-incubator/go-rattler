//go:build cgo

package api

// Operation IDs are stable ABI values shared with the Rust dispatcher in
// internal/native/src/lib.rs. Do not renumber; add new IDs at the end.
const (
	OperationParsePrefix        uint32 = 1
	OperationParsePrefixBatch   uint32 = 2
	OperationParseMinimalPrefix uint32 = 3
	OperationArchiveFilename    uint32 = 4
	OperationIndexJSON          uint32 = 5
	OperationChannel            uint32 = 6
	OperationMatchSpec          uint32 = 7
	OperationVersionSpec        uint32 = 8
	OperationEnvironmentYAML    uint32 = 9
	OperationExplicitSpec       uint32 = 10
	OperationOCISubdir          uint32 = 11
	OperationLockPackages       uint32 = 12
	OperationLockSync           uint32 = 13
	OperationCollectPrefix      uint32 = 14
	OperationCompareVersions    uint32 = 15
)
