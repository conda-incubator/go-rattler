//go:build cgo

package gorattler

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func readLockFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "lock", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

func TestLockPackagesConda(t *testing.T) {
	packages, err := LockPackages(readLockFixture(t, "conda_only.lock"), "default", "linux-64")
	if err != nil {
		t.Fatalf("LockPackages() error = %v", err)
	}
	want := []LockedPackage{
		{
			Kind: "conda", Name: "python", Version: "3.12.1", Build: "h123_0", Subdir: "linux-64",
			BuildNumber: new(uint64(0)),
			URL:         "https://conda.anaconda.org/conda-forge/linux-64/python-3.12.1-h123_0.conda",
			SHA256:      "1111111111111111111111111111111111111111111111111111111111111111",
			MD5:         "22222222222222222222222222222222",
			Depends:     []string{"zlib >=1.2"},
		},
		{
			Kind: "conda", Name: "zlib", Version: "1.3.1", Build: "hb9d3cd8_2", Subdir: "linux-64",
			BuildNumber: new(uint64(2)),
			URL:         "https://conda.anaconda.org/conda-forge/linux-64/zlib-1.3.1-hb9d3cd8_2.conda",
			SHA256:      "3333333333333333333333333333333333333333333333333333333333333333",
			MD5:         "44444444444444444444444444444444",
			Depends:     []string{"__glibc >=2.17,<3.0.a0"},
		},
	}
	if !reflect.DeepEqual(packages, want) {
		t.Fatalf("LockPackages() = %+v\nwant %+v", packages, want)
	}
}

func TestLockPackagesPyPINormalizesName(t *testing.T) {
	packages, err := LockPackages(readLockFixture(t, "pypi_mixed.lock"), "default", "linux-64")
	if err != nil {
		t.Fatalf("LockPackages() error = %v", err)
	}
	var pypi []LockedPackage
	for _, p := range packages {
		if p.Kind == "pypi" {
			pypi = append(pypi, p)
		}
	}
	if len(pypi) != 2 || pypi[0].Name != "demo-pkg" || pypi[0].Version != "1.0" || pypi[1].Name != "norm-me" {
		t.Fatalf("pypi packages = %+v", pypi)
	}
}

func TestLockPackagesUnresolvedSourceHasReason(t *testing.T) {
	packages, err := LockPackages(readLockFixture(t, "source_unresolved.lock"), "default", "linux-64")
	if err != nil {
		t.Fatalf("LockPackages() error = %v", err)
	}
	if len(packages) != 1 {
		t.Fatalf("got %d packages, want 1", len(packages))
	}
	p := packages[0]
	if !p.Source || p.Version != "" || p.Reason == "" {
		t.Fatalf("source record must have empty version and a Reason, got %+v", p)
	}
}

func TestLockPackagesErrors(t *testing.T) {
	conda := readLockFixture(t, "conda_only.lock")
	tests := []struct {
		name        string
		lock        []byte
		environment string
		subdir      string
		invalid     bool
		contains    string
	}{
		{name: "empty environment", lock: conda, environment: "", subdir: "linux-64", invalid: true},
		{name: "empty subdir", lock: conda, environment: "default", subdir: "", invalid: true},
		{name: "missing environment", lock: conda, environment: "absent", subdir: "linux-64", invalid: true, contains: "environment"},
		{name: "missing platform", lock: conda, environment: "default", subdir: "osx-arm64", invalid: true, contains: "platform"},
		{name: "malformed lock", lock: readLockFixture(t, "malformed.lock"), environment: "default", subdir: "linux-64", invalid: true, contains: "malformed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LockPackages(tt.lock, tt.environment, tt.subdir)
			if err == nil {
				t.Fatal("LockPackages() error = nil, want error")
			}
			if errors.Is(err, ErrInvalidInput) != tt.invalid {
				t.Fatalf("errors.Is(ErrInvalidInput) = %v, want %v (err: %v)", !tt.invalid, tt.invalid, err)
			}
			if tt.contains != "" && !strings.Contains(err.Error(), tt.contains) {
				t.Fatalf("error %q does not mention %q", err, tt.contains)
			}
		})
	}
}

func TestLockPackagesNeverNil(t *testing.T) {
	packages, err := LockPackages(readLockFixture(t, "noarch.lock"), "default", "linux-64")
	if err != nil || packages == nil {
		t.Fatalf("LockPackages() = %v, %v; want non-nil slice", packages, err)
	}
}
