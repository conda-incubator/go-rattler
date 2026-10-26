//go:build cgo

package gorattler

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func fixtureBytes(t *testing.T, rel string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "records", "fixtures", rel))
	if err != nil {
		t.Fatalf("read fixture %s: %v", rel, err)
	}
	return data
}

func TestParsePrefixRecordPreservesFields(t *testing.T) {
	record, err := ParsePrefixRecord(fixtureBytes(t, "conda-meta/envscout-demo-1.2.3-py311_0.json"))
	if err != nil {
		t.Fatalf("ParsePrefixRecord: %v", err)
	}
	if record.Name != "envscout-demo" || record.Version != "1.2.3" || record.Build != "py311_0" {
		t.Fatalf("identity fields = %q %q %q", record.Name, record.Version, record.Build)
	}
	if record.Filename != "envscout-demo-1.2.3-py311_0.conda" {
		t.Fatalf("Filename = %q", record.Filename)
	}
	if !reflect.DeepEqual(record.RequestedSpecs, []string{"envscout-demo ==1.2.3"}) {
		t.Fatalf("RequestedSpecs = %v", record.RequestedSpecs)
	}
	if !reflect.DeepEqual(record.Files, []string{"bin/envscout-demo", "lib/python3.11/site-packages/demo.py"}) {
		t.Fatalf("Files = %v", record.Files)
	}
	if len(record.PathsData.Paths) != 2 || record.PathsData.Paths[0].RelativePath != "bin/envscout-demo" {
		t.Fatalf("PathsData = %+v", record.PathsData)
	}
	if record.PathsData.Paths[0].SizeInBytes == nil || *record.PathsData.Paths[0].SizeInBytes != 1024 {
		t.Fatalf("size_in_bytes not preserved: %+v", record.PathsData.Paths[0])
	}
	if record.Link == nil || record.Link.Type == nil || *record.Link.Type != 1 {
		t.Fatalf("Link = %+v", record.Link)
	}
	if record.SourceIndex != 0 {
		t.Fatalf("SourceIndex = %d, want 0 for single parse", record.SourceIndex)
	}
}

func TestParsePrefixRecordRejectsInvalid(t *testing.T) {
	if _, err := ParsePrefixRecord([]byte("{")); err == nil {
		t.Fatal("truncated JSON accepted")
	}
	if _, err := ParsePrefixRecord([]byte{0xff, 0xfe}); err == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
	if _, err := ParsePrefixRecord(append(fixtureBytes(t, "index/index.json"), []byte(" {}")...)); err == nil {
		t.Fatal("trailing JSON accepted")
	}
}

func TestParsePrefixRecordsMixedBatch(t *testing.T) {
	good := fixtureBytes(t, "conda-meta/envscout-demo-1.2.3-py311_0.json")
	bad := fixtureBytes(t, "conda-meta/bad-1.0-0.json")
	result, err := ParsePrefixRecords([][]byte{good, nil, bad, good})
	if err != nil {
		t.Fatalf("ParsePrefixRecords: %v", err)
	}
	if len(result.Records) != 2 {
		t.Fatalf("records = %d, want 2", len(result.Records))
	}
	if result.Records[0].SourceIndex != 0 || result.Records[1].SourceIndex != 3 {
		t.Fatalf("source indices = %d, %d", result.Records[0].SourceIndex, result.Records[1].SourceIndex)
	}
	if len(result.Errors) != 2 {
		t.Fatalf("errors = %+v, want 2", result.Errors)
	}
	if result.Errors[0].Index != 1 || result.Errors[1].Index != 2 {
		t.Fatalf("error indices = %d, %d", result.Errors[0].Index, result.Errors[1].Index)
	}
}

func TestParsePrefixRecordsAllInvalidIsNotAPIError(t *testing.T) {
	result, err := ParsePrefixRecords([][]byte{nil, []byte("{")})
	if err != nil {
		t.Fatalf("all-invalid batch returned API error: %v", err)
	}
	if len(result.Records) != 0 || len(result.Errors) != 2 {
		t.Fatalf("result = %+v", result)
	}
}

func TestParsePrefixRecordsEmptyBatchUsesNonNilSlices(t *testing.T) {
	result, err := ParsePrefixRecords(nil)
	if err != nil {
		t.Fatalf("ParsePrefixRecords(nil): %v", err)
	}
	if result.Records == nil || result.Errors == nil {
		t.Fatalf("empty batch slices must be non-nil: %+v", result)
	}
}

func TestParseMinimalPrefixRecordMatchesFull(t *testing.T) {
	doc := fixtureBytes(t, "conda-meta/envscout-demo-1.2.3-py311_0.json")
	full, err := ParsePrefixRecord(doc)
	if err != nil {
		t.Fatal(err)
	}
	minimal, err := ParseMinimalPrefixRecord(doc)
	if err != nil {
		t.Fatalf("ParseMinimalPrefixRecord: %v", err)
	}
	if minimal.Name != full.Name || minimal.Version != full.Version || minimal.Build != full.Build {
		t.Fatalf("minimal identity %+v != full %+v", minimal, full)
	}
	if minimal.SHA256 != full.SHA256 {
		t.Fatalf("sha256 mismatch: %q vs %q", minimal.SHA256, full.SHA256)
	}
	if minimal.Size == nil || full.Size == nil || *minimal.Size != *full.Size {
		t.Fatalf("size mismatch")
	}
}

func TestCollectPrefixRecordsSkipsSymlinksAndReportsErrors(t *testing.T) {
	dir := t.TempDir()
	meta := filepath.Join(dir, "conda-meta")
	if err := os.Mkdir(meta, 0o755); err != nil {
		t.Fatal(err)
	}
	good := fixtureBytes(t, "conda-meta/envscout-demo-1.2.3-py311_0.json")
	if err := os.WriteFile(filepath.Join(meta, "envscout-demo-1.2.3-py311_0.json"), good, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(meta, "bad-1.0-0.json"), fixtureBytes(t, "conda-meta/bad-1.0-0.json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(meta, "envscout-demo-1.2.3-py311_0.json"), filepath.Join(meta, "link-1.0-0.json")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	result, err := CollectPrefixRecords(dir)
	if err != nil {
		t.Fatalf("CollectPrefixRecords: %v", err)
	}
	if len(result.Records) != 1 || result.Records[0].Name != "envscout-demo" {
		t.Fatalf("records = %+v", result.Records)
	}
	if len(result.Errors) != 1 || result.Errors[0].Index != 0 || filepath.Base(result.Errors[0].Path) != "bad-1.0-0.json" {
		t.Fatalf("errors = %+v", result.Errors)
	}
}

func TestCollectPrefixRecordsRejectsEmptyPrefix(t *testing.T) {
	if _, err := CollectPrefixRecords(""); err == nil {
		t.Fatal("empty prefix accepted")
	}
}

func TestCollectPrefixRecordsRejectsSymlinkedMeta(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, filepath.Join(dir, "conda-meta")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := CollectPrefixRecords(dir); err == nil {
		t.Fatal("symlinked conda-meta accepted")
	}
}
