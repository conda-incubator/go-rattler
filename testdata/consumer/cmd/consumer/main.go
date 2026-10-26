package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

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

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "consumer: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	checks := []struct {
		name string
		fn   func() error
	}{
		{"compare versions", checkCompareVersions},
		{"lock environment", checkLockEnvironment},
		{"prefix record", checkPrefixRecord},
		{"prefix records partial", checkPrefixRecordsPartial},
		{"archive filename", checkArchiveFilename},
		{"index json", checkIndexJSON},
		{"canonical channel", checkCanonicalChannel},
		{"match spec", checkMatchSpec},
		{"version spec", checkVersionSpec},
		{"environment yaml", checkEnvironmentYAML},
		{"explicit spec", checkExplicitSpec},
		{"oci platform", checkOCIPlatform},
		{"invalid input", checkInvalidInput},
	}
	for _, c := range checks {
		if err := c.fn(); err != nil {
			return fmt.Errorf("%s: %w", c.name, err)
		}
	}
	return nil
}

func checkCompareVersions() error {
	comparison, err := gorattler.CompareVersions("1.0rc1", "1.0")
	if err != nil {
		return err
	}
	if comparison != -1 {
		return fmt.Errorf("compare versions = %d, want -1", comparison)
	}
	return nil
}

func checkLockEnvironment() error {
	counts, err := gorattler.LockEnvironment(filepath.Join("testdata", "pixi.lock"), "default", "osx-arm64")
	if err != nil {
		return err
	}
	if counts != (gorattler.PackageCounts{Conda: 1, PyPI: 1}) {
		return fmt.Errorf("lock environment counts = %+v", counts)
	}
	return nil
}

func checkPrefixRecord() error {
	record, err := gorattler.ParsePrefixRecord([]byte(canonicalPrefixRecord))
	if err != nil {
		return err
	}
	if record.Name != "python" || record.BuildNumber != 0 || record.Filename != "python-3.13-h0.conda" ||
		record.URL != "https://example.test/python-3.13-h0.conda" || len(record.Depends) != 1 {
		return fmt.Errorf("prefix record = %+v", record)
	}
	return nil
}

func checkPrefixRecordsPartial() error {
	result, err := gorattler.ParsePrefixRecords([][]byte{[]byte("{not json"), []byte(canonicalPrefixRecord)})
	if err != nil {
		return err
	}
	if len(result.Records) != 1 || len(result.Errors) != 1 || result.Errors[0].Index != 0 {
		return fmt.Errorf("prefix records partial = %+v", result)
	}
	return nil
}

func checkArchiveFilename() error {
	id, err := gorattler.ParseArchiveFilename("python-3.13.0-h0.conda")
	if err != nil {
		return err
	}
	if id.Name != "python" || id.Type != gorattler.ArchiveTypeConda {
		return fmt.Errorf("archive identifier = %+v", id)
	}
	return nil
}

func checkIndexJSON() error {
	index, err := gorattler.ParseIndexJSON([]byte(`{"name":"python","version":"3.13.0","build":"h0","build_number":0}`))
	if err != nil {
		return err
	}
	if index.Name != "python" || index.Version != "3.13.0" || index.Build != "h0" {
		return fmt.Errorf("index json = %+v", index)
	}
	return nil
}

func checkCanonicalChannel() error {
	channel, err := gorattler.CanonicalChannel("conda-forge")
	if err != nil {
		return err
	}
	if channel == "" {
		return errors.New("canonical channel is empty")
	}
	return nil
}

func checkMatchSpec() error {
	info, err := gorattler.ParseMatchSpec("python>=3.12")
	if err != nil {
		return err
	}
	if info.Name != "python" || info.Pinned {
		return fmt.Errorf("match spec = %+v", info)
	}
	record := gorattler.PackageRecord{Name: "python", Version: "3.13.0", Build: "h0", BuildNumber: 0, Subdir: "osx-arm64"}
	matched, err := gorattler.MatchSpecMatches("python>=3.12", record)
	if err != nil {
		return err
	}
	if !matched {
		return errors.New("python>=3.12 did not match python 3.13.0")
	}
	return nil
}

func checkVersionSpec() error {
	ok, err := gorattler.VersionSpecContains("<3.14", "3.13")
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("<3.14 did not contain 3.13")
	}
	return nil
}

func checkEnvironmentYAML() error {
	doc := []byte("channels:\n  - conda-forge\ndependencies:\n  - python\n  - pip:\n    - demo\n")
	env, err := gorattler.ParseEnvironmentYAML(doc)
	if err != nil {
		return err
	}
	if len(env.Channels) != 1 || len(env.Dependencies) == 0 || len(env.Subsections["pip"]) != 1 {
		return fmt.Errorf("environment yaml = %+v", env)
	}
	return nil
}

func checkExplicitSpec() error {
	doc := []byte("# platform: osx-arm64\n@EXPLICIT\nhttps://example.test/python-3.13-h0.conda\n")
	spec, err := gorattler.ParseExplicitSpec(doc)
	if err != nil {
		return err
	}
	if spec.Platform != "osx-arm64" || len(spec.Packages) != 1 {
		return fmt.Errorf("explicit spec = %+v", spec)
	}
	return nil
}

func checkOCIPlatform() error {
	subdir, err := gorattler.SubdirFromOCIPlatform("linux", "arm64", "v8")
	if err != nil {
		return err
	}
	if subdir != "linux-aarch64" {
		return fmt.Errorf("oci subdir = %q", subdir)
	}
	return nil
}

func checkInvalidInput() error {
	_, err := gorattler.CanonicalChannel("   ")
	if !errors.Is(err, gorattler.ErrInvalidInput) {
		return fmt.Errorf("blank channel error = %v, want ErrInvalidInput", err)
	}
	return nil
}
