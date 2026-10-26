package consumer

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	gorattler "github.com/conda-incubator/go-rattler"
)

const canonicalPrefixRecord = `{
  "name": "python",
  "version": "3.13.0",
  "build": "h0",
  "build_number": 0,
  "subdir": "osx-arm64",
  "url": "https://example.test/python-3.13-h0.conda",
  "fn": "python-3.13-h0.conda",
  "depends": ["libffi"],
  "files": ["bin/python"],
  "paths_data": {"paths_version": 1, "paths": [{"_path": "bin/python", "path_type": "hardlink"}]}
}`

func TestPublicBindingsFromAnotherModule(t *testing.T) {
	t.Run("CompareVersions prerelease", func(t *testing.T) {
		comparison, err := gorattler.CompareVersions("1.0rc1", "1.0")
		if err != nil || comparison != -1 {
			t.Fatalf("CompareVersions() = %d, %v", comparison, err)
		}
	})

	t.Run("LockEnvironment counts", func(t *testing.T) {
		counts, err := gorattler.LockEnvironment(filepath.Join("..", "pixi.lock"), "default", "osx-arm64")
		if err != nil || counts != (gorattler.PackageCounts{Conda: 1, PyPI: 1}) {
			t.Fatalf("LockEnvironment() = %+v, %v", counts, err)
		}
	})

	t.Run("ParsePrefixRecord canonical", func(t *testing.T) {
		record, err := gorattler.ParsePrefixRecord([]byte(canonicalPrefixRecord))
		if err != nil {
			t.Fatalf("ParsePrefixRecord() error = %v", err)
		}
		if record.Name != "python" || record.BuildNumber != 0 || record.Filename != "python-3.13-h0.conda" ||
			record.URL != "https://example.test/python-3.13-h0.conda" || len(record.Depends) != 1 {
			t.Fatalf("ParsePrefixRecord() = %+v", record)
		}
	})

	t.Run("ParsePrefixRecords partial", func(t *testing.T) {
		result, err := gorattler.ParsePrefixRecords([][]byte{[]byte("{not json"), []byte(canonicalPrefixRecord)})
		if err != nil {
			t.Fatalf("ParsePrefixRecords() error = %v", err)
		}
		if len(result.Records) != 1 || len(result.Errors) != 1 || result.Errors[0].Index != 0 {
			t.Fatalf("ParsePrefixRecords() = %+v", result)
		}
	})

	t.Run("ParseArchiveFilename", func(t *testing.T) {
		id, err := gorattler.ParseArchiveFilename("python-3.13.0-h0.conda")
		if err != nil || id.Name != "python" || id.Type != gorattler.ArchiveTypeConda {
			t.Fatalf("ParseArchiveFilename() = %+v, %v", id, err)
		}
	})

	t.Run("ParseIndexJSON required fields", func(t *testing.T) {
		index, err := gorattler.ParseIndexJSON([]byte(`{"name":"python","version":"3.13.0","build":"h0","build_number":0}`))
		if err != nil || index.Name != "python" || index.Version != "3.13.0" || index.Build != "h0" {
			t.Fatalf("ParseIndexJSON() = %+v, %v", index, err)
		}
	})

	t.Run("CanonicalChannel conda-forge", func(t *testing.T) {
		channel, err := gorattler.CanonicalChannel("conda-forge")
		if err != nil || channel == "" {
			t.Fatalf("CanonicalChannel() = %q, %v", channel, err)
		}
	})

	t.Run("ParseMatchSpec pinned range", func(t *testing.T) {
		info, err := gorattler.ParseMatchSpec("python>=3.12")
		if err != nil || info.Name != "python" || info.Pinned {
			t.Fatalf("ParseMatchSpec() = %+v, %v", info, err)
		}
	})

	t.Run("MatchSpecMatches full record", func(t *testing.T) {
		record := gorattler.PackageRecord{Name: "python", Version: "3.13.0", Build: "h0", BuildNumber: 0, Subdir: "osx-arm64"}
		matched, err := gorattler.MatchSpecMatches("python>=3.12", record)
		if err != nil || !matched {
			t.Fatalf("MatchSpecMatches() = %v, %v", matched, err)
		}
	})

	t.Run("VersionSpecContains", func(t *testing.T) {
		ok, err := gorattler.VersionSpecContains("<3.14", "3.13")
		if err != nil || !ok {
			t.Fatalf("VersionSpecContains() = %v, %v", ok, err)
		}
	})

	t.Run("ParseEnvironmentYAML sections", func(t *testing.T) {
		doc := []byte("channels:\n  - conda-forge\ndependencies:\n  - python\n  - pip:\n    - demo\n")
		env, err := gorattler.ParseEnvironmentYAML(doc)
		if err != nil || len(env.Channels) != 1 || len(env.Dependencies) == 0 || len(env.Subsections["pip"]) != 1 {
			t.Fatalf("ParseEnvironmentYAML() = %+v, %v", env, err)
		}
	})

	t.Run("ParseExplicitSpec platform and URL", func(t *testing.T) {
		doc := []byte("# platform: osx-arm64\n@EXPLICIT\nhttps://example.test/python-3.13-h0.conda\n")
		spec, err := gorattler.ParseExplicitSpec(doc)
		if err != nil || spec.Platform != "osx-arm64" || len(spec.Packages) != 1 {
			t.Fatalf("ParseExplicitSpec() = %+v, %v", spec, err)
		}
	})

	t.Run("SubdirFromOCIPlatform linux arm64 v8", func(t *testing.T) {
		subdir, err := gorattler.SubdirFromOCIPlatform("linux", "arm64", "v8")
		if err != nil || subdir != "linux-aarch64" {
			t.Fatalf("SubdirFromOCIPlatform() = %q, %v", subdir, err)
		}
	})

	t.Run("invalid input reports ErrInvalidInput", func(t *testing.T) {
		_, err := gorattler.CanonicalChannel("   ")
		if !errors.Is(err, gorattler.ErrInvalidInput) {
			t.Fatalf("CanonicalChannel(blank) error = %v, want ErrInvalidInput", err)
		}
	})
}

func TestLockComparisonFromAnotherModule(t *testing.T) {
	document, err := os.ReadFile(filepath.Join("..", "lock", "conda_only.lock"))
	if err != nil {
		t.Fatal(err)
	}
	locked, err := gorattler.LockPackages(document, "default", "linux-64")
	if err != nil || len(locked) != 2 {
		t.Fatalf("lock packages: %+v, %v", locked, err)
	}
	installed := make([]gorattler.PrefixRecord, 0, len(locked))
	for _, p := range locked {
		var number uint64
		if p.BuildNumber != nil {
			number = *p.BuildNumber
		}
		installed = append(installed, gorattler.PrefixRecord{PackageRecord: gorattler.PackageRecord{
			Name: p.Name, Version: p.Version, Build: p.Build, BuildNumber: number, Subdir: p.Subdir, SHA256: p.SHA256, MD5: p.MD5,
		}})
	}
	partial, err := gorattler.CompareLock(document, "default", "linux-64", gorattler.InstalledPackages{Conda: installed})
	if err != nil || partial.Complete || partial.Matches || len(partial.Unknown) == 0 {
		t.Fatalf("partial observations must never claim a match: %+v, %v", partial, err)
	}
	scoped, err := gorattler.LockSync(document, "default", "linux-64", nil)
	if err != nil || scoped.Scope != "conda" || len(scoped.Missing) != len(locked) || scoped.Matches {
		t.Fatalf("conda-scoped empty set: %+v, %v", scoped, err)
	}
}
