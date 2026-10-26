//go:build cgo

package gorattler

import (
	"encoding/json"
	"strings"
	"testing"
)

const pythonSHA = "1111111111111111111111111111111111111111111111111111111111111111"

// prefixRecord builds a PrefixRecord with the identity fields set through the embedded
// PackageRecord, which is how the type is composed.
func prefixRecord(name, version, build string, buildNumber uint64, subdir, sha256 string) PrefixRecord {
	return PrefixRecord{PackageRecord: PackageRecord{
		Name: name, Version: version, Build: build, BuildNumber: buildNumber,
		Subdir: subdir, SHA256: sha256,
	}}
}

func condaPython(sha string) PrefixRecord {
	return prefixRecord("python", "3.12.1", "h123_0", 0, "linux-64", sha)
}

func TestLockSyncMatchesCompleteCondaSet(t *testing.T) {
	lock := readLockFixture(t, "conda_only.lock")
	report, err := LockSync(lock, "default", "linux-64", []PrefixRecord{
		condaPython(pythonSHA),
		prefixRecord("zlib", "1.3.1", "hb9d3cd8_2", 2, "linux-64",
			"3333333333333333333333333333333333333333333333333333333333333333"),
	})
	if err != nil {
		t.Fatalf("LockSync() error = %v", err)
	}
	if !report.Complete || !report.Matches || report.Scope != "conda" {
		t.Fatalf("report = %+v, want complete matching conda scope", report)
	}
	if len(report.Missing)+len(report.Extra)+len(report.Changed)+len(report.Unknown) != 0 {
		t.Fatalf("report has differences: %+v", report)
	}
}

func TestLockSyncMissingExtraAndChanged(t *testing.T) {
	lock := readLockFixture(t, "conda_only.lock")
	report, err := LockSync(lock, "default", "linux-64", []PrefixRecord{
		condaPython("9999999999999999999999999999999999999999999999999999999999999999"),
		prefixRecord("stray", "2.0", "0", 0, "linux-64", ""),
	})
	if err != nil {
		t.Fatalf("LockSync() error = %v", err)
	}
	if !report.Complete || report.Matches {
		t.Fatalf("Complete=%v Matches=%v, want complete and not matching", report.Complete, report.Matches)
	}
	if len(report.Missing) != 1 || report.Missing[0].Name != "zlib" {
		t.Fatalf("Missing = %+v, want zlib", report.Missing)
	}
	if len(report.Changed) != 1 || report.Changed[0].Reasons[0] != "sha256" {
		t.Fatalf("Changed = %+v, want sha256 change", report.Changed)
	}
	if len(report.Extra) != 1 || report.Extra[0].Name != "stray" {
		t.Fatalf("Extra = %+v, want stray", report.Extra)
	}
}

func TestLockSyncURLAloneIsNotAChange(t *testing.T) {
	lock := readLockFixture(t, "mirror.lock")
	mirror := prefixRecord("zlib", "1.3.1", "hb9d3cd8_2", 2, "linux-64",
		"3333333333333333333333333333333333333333333333333333333333333333")
	mirror.URL = "https://mirror.example.com/linux-64/zlib-1.3.1-hb9d3cd8_2.conda"
	report, err := LockSync(lock, "default", "linux-64", []PrefixRecord{mirror})
	if err != nil {
		t.Fatalf("LockSync() error = %v", err)
	}
	if len(report.Changed) != 0 || !report.Matches {
		t.Fatalf("mirror URL with matching hash must not change the package: %+v", report)
	}
}

func TestLockSyncMissingInstalledHashIsUnknown(t *testing.T) {
	lock := readLockFixture(t, "conda_only.lock")
	report, err := LockSync(lock, "default", "linux-64", []PrefixRecord{
		prefixRecord("python", "3.12.1", "h123_0", 0, "linux-64", ""),
		prefixRecord("zlib", "1.3.1", "hb9d3cd8_2", 2, "linux-64",
			"3333333333333333333333333333333333333333333333333333333333333333"),
	})
	if err != nil {
		t.Fatalf("LockSync() error = %v", err)
	}
	if len(report.Changed) != 0 {
		t.Fatalf("missing installed hash must not be Changed: %+v", report.Changed)
	}
	if report.Complete || report.Matches || len(report.Unknown) == 0 {
		t.Fatalf("missing installed hash must yield Unknown and incomplete: %+v", report)
	}
}

func TestLockSyncNoarchSubdirIsNotChanged(t *testing.T) {
	lock := readLockFixture(t, "noarch.lock")
	report, err := LockSync(lock, "default", "linux-64", []PrefixRecord{
		prefixRecord("six", "1.16.0", "pyh6c4a22f_0", 0, "linux-64",
			"7777777777777777777777777777777777777777777777777777777777777777"),
	})
	if err != nil {
		t.Fatalf("LockSync() error = %v", err)
	}
	if len(report.Changed) != 0 {
		t.Fatalf("noarch lock vs linux-64 installed must not change: %+v", report.Changed)
	}
}

func TestLockSyncDuplicateInstalledNameIsUnknown(t *testing.T) {
	lock := readLockFixture(t, "conda_only.lock")
	report, err := LockSync(lock, "default", "linux-64", []PrefixRecord{
		condaPython(pythonSHA),
		prefixRecord("python", "3.12.1", "other", 0, "linux-64", ""),
	})
	if err != nil {
		t.Fatalf("LockSync() error = %v", err)
	}
	if report.Complete || report.Matches || len(report.Unknown) == 0 {
		t.Fatalf("duplicate installed name must be Unknown: %+v", report)
	}
}

func TestLockSyncBuildNumberIsCarried(t *testing.T) {
	lock := readLockFixture(t, "conda_only.lock")
	// Same build string, different build number: must not produce a false Extra for the
	// locked record; the build number is part of the identity projection.
	report, err := LockSync(lock, "default", "linux-64", []PrefixRecord{
		condaPython(pythonSHA),
		prefixRecord("zlib", "1.3.1", "hb9d3cd8_2", 99, "linux-64",
			"3333333333333333333333333333333333333333333333333333333333333333"),
	})
	if err != nil {
		t.Fatalf("LockSync() error = %v", err)
	}
	if len(report.Extra) != 0 || len(report.Changed) != 1 || report.Changed[0].Reasons[0] != "build_number" {
		t.Fatalf("build number must be a change, not a false Extra: %+v", report)
	}
}

func TestLockSyncDoesNotTrustInstalledNoarchClaim(t *testing.T) {
	python := condaPython(pythonSHA)
	python.Subdir = "noarch"
	report, err := LockSync(readLockFixture(t, "conda_only.lock"), "default", "linux-64", []PrefixRecord{python})
	if err != nil || len(report.Changed) != 1 || !strings.Contains(strings.Join(report.Changed[0].Reasons, ","), "subdir") {
		t.Fatalf("a binary lock's subdir cannot be bypassed by installed metadata: %+v, %v", report, err)
	}
}

func TestLockSyncInvalidHashesAndMissingStrongHashAreUnknown(t *testing.T) {
	for _, test := range []struct{ sha, md5 string }{
		{"not-a-sha256", ""},
		{"", strings.Repeat("2", 32)},
	} {
		python := condaPython(test.sha)
		python.MD5 = test.md5
		report, err := LockSync(readLockFixture(t, "conda_only.lock"), "default", "linux-64", []PrefixRecord{python})
		if err != nil || report.Complete || report.Matches || len(report.Unknown) == 0 {
			t.Fatalf("invalid or missing expected strong hash cannot match: %+v, %v", report, err)
		}
	}
}

func TestCompareLockDuplicateUnlistedPyPIDoesNotDisappear(t *testing.T) {
	report, err := CompareLock(readLockFixture(t, "conda_only.lock"), "default", "linux-64", InstalledPackages{
		CondaComplete: true, PyPIComplete: true,
		Pypi: []InstalledPyPI{{Name: "stray.pkg", Version: "1"}, {Name: "stray_pkg", Version: "1"}},
	})
	if err != nil || report.Complete || len(report.Unknown) == 0 {
		t.Fatalf("unlisted duplicate observations must be ambiguous: %+v, %v", report, err)
	}
}

func TestLockSyncIgnoresOnlyExplicitlyOutOfScopePyPI(t *testing.T) {
	report, err := LockSync(readLockFixture(t, "pypi_mixed.lock"), "default", "linux-64", []PrefixRecord{condaPython(pythonSHA)})
	if err != nil || report.Scope != "conda" || !report.Complete || !report.Matches {
		t.Fatalf("complete conda-only match should disclose its scope: %+v, %v", report, err)
	}
}

func TestLockComparisonURLsDoNotExposeCredentials(t *testing.T) {
	lock := []byte(strings.ReplaceAll(string(readLockFixture(t, "conda_only.lock")), "https://conda.anaconda.org/conda-forge/", "https://sensitive-user:sensitive-secret@conda.anaconda.org/t/sensitive-token/conda-forge/"))
	packages, err := LockPackages(lock, "default", "linux-64")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(packages)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"sensitive-user", "sensitive-secret", "sensitive-token"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("locked URL leaked %q: %s", secret, encoded)
		}
	}
}

func TestCompareLockPyPIEqualityAndNormalization(t *testing.T) {
	lock := readLockFixture(t, "pypi_mixed.lock")
	report, err := CompareLock(lock, "default", "linux-64", InstalledPackages{
		Conda:         []PrefixRecord{condaPython(pythonSHA)},
		CondaComplete: true,
		Pypi: []InstalledPyPI{
			{Name: "Demo.Pkg", Version: "1.0",
				SHA256: "5555555555555555555555555555555555555555555555555555555555555555"},
			{Name: "norm_me", Version: "1.0.0",
				SHA256: "6666666666666666666666666666666666666666666666666666666666666666"},
		},
		PyPIComplete: true,
	})
	if err != nil {
		t.Fatalf("CompareLock() error = %v", err)
	}
	if report.Scope != "conda+pypi" || !report.Complete || !report.Matches {
		t.Fatalf("report = %+v, want complete matching conda+pypi", report)
	}
}

func TestCompareLockIgnoredPyPIIsNotClaimedClean(t *testing.T) {
	lock := readLockFixture(t, "pypi_mixed.lock")
	report, err := CompareLock(lock, "default", "linux-64", InstalledPackages{
		Conda:         []PrefixRecord{condaPython(pythonSHA)},
		CondaComplete: true,
	})
	if err != nil {
		t.Fatalf("CompareLock() error = %v", err)
	}
	if report.Complete || report.Matches {
		t.Fatalf("locked PyPI without a PyPI set must not be reported clean: %+v", report)
	}
	if len(report.Unknown) == 0 {
		t.Fatal("locked PyPI without a PyPI set must produce Unknown entries")
	}
}

func TestCompareLockEmptyPyPIAssertedCompleteWithNoLockedPyPI(t *testing.T) {
	// Conservative rule: even a lock without PyPI packages is not Complete unless the PyPI
	// scope was asserted, because unexpected installed extras cannot be ruled out.
	lock := readLockFixture(t, "conda_only.lock")
	report, err := CompareLock(lock, "default", "linux-64", InstalledPackages{
		Conda:         []PrefixRecord{condaPython(pythonSHA), prefixRecord("zlib", "1.3.1", "hb9d3cd8_2", 2, "linux-64", "3333333333333333333333333333333333333333333333333333333333333333")},
		CondaComplete: true,
	})
	if err != nil {
		t.Fatalf("CompareLock() error = %v", err)
	}
	if report.Complete {
		t.Fatalf("PyPI scope not asserted must never be Complete: %+v", report)
	}
}

func TestCompareLockSourceUnresolvedIsUnknownNotMissing(t *testing.T) {
	lock := readLockFixture(t, "source_unresolved.lock")
	report, err := CompareLock(lock, "default", "linux-64", InstalledPackages{CondaComplete: true})
	if err != nil {
		t.Fatalf("CompareLock() error = %v", err)
	}
	if len(report.Missing) != 0 || len(report.Unknown) == 0 || report.Complete {
		t.Fatalf("unresolved source must be Unknown, not Missing: %+v", report)
	}
}

func TestCompareLockDeterministic(t *testing.T) {
	lock := readLockFixture(t, "conda_only.lock")
	input := InstalledPackages{CondaComplete: true, Conda: []PrefixRecord{
		prefixRecord("b-extra", "1", "0", 0, "linux-64", ""),
		prefixRecord("a-extra", "1", "0", 0, "linux-64", ""),
	}}
	first, err := CompareLock(lock, "default", "linux-64", input)
	if err != nil {
		t.Fatalf("CompareLock() error = %v", err)
	}
	second, err := CompareLock(lock, "default", "linux-64", input)
	if err != nil {
		t.Fatalf("CompareLock() error = %v", err)
	}
	if len(first.Extra) != 2 || first.Extra[0].Name != "a-extra" {
		t.Fatalf("Extra not sorted: %+v", first.Extra)
	}
	if len(first.Extra) != len(second.Extra) || first.Extra[0] != second.Extra[0] {
		t.Fatal("repeated comparison is not deterministic")
	}
}

func TestCompareLockInvalidCallerInput(t *testing.T) {
	lock := readLockFixture(t, "conda_only.lock")
	if _, err := CompareLock(lock, "", "linux-64", InstalledPackages{}); err == nil {
		t.Fatal("empty environment must be rejected")
	}
	if _, err := CompareLock(readLockFixture(t, "malformed.lock"), "default", "linux-64", InstalledPackages{}); err == nil {
		t.Fatal("malformed lock must return an error")
	}
}
