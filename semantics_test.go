//go:build cgo

package gorattler

import (
	"errors"
	"strings"
	"testing"
)

func TestCanonicalChannel(t *testing.T) {
	name, err := CanonicalChannel("conda-forge")
	if err != nil {
		t.Fatalf("CanonicalChannel(conda-forge) error = %v", err)
	}
	if name != "https://conda.anaconda.org/conda-forge/" {
		t.Fatalf("CanonicalChannel(conda-forge) = %q", name)
	}
	url, err := CanonicalChannel("https://conda.anaconda.org/conda-forge")
	if err != nil || url != name {
		t.Fatalf("URL form = %q, %v; want %q", url, err, name)
	}
}

func TestCanonicalChannelRedactsSecrets(t *testing.T) {
	for _, value := range []string{
		"https://user:hunter2@repo.example.com/chan",
		"https://repo.example.com/t/SECRETTOKEN/chan",
	} {
		got, err := CanonicalChannel(value)
		if err != nil {
			t.Fatalf("CanonicalChannel(%q) error = %v", value, err)
		}
		for _, secret := range []string{"hunter2", "SECRETTOKEN"} {
			if strings.Contains(got, secret) {
				t.Errorf("CanonicalChannel(%q) = %q leaks %q", value, got, secret)
			}
		}
	}
}

func TestCanonicalChannelRejects(t *testing.T) {
	for _, value := range []string{"", "   ", "defaults", "./local", "/abs/path", "file:///tmp/chan"} {
		if _, err := CanonicalChannel(value); err == nil {
			t.Errorf("CanonicalChannel(%q) succeeded; want error", value)
		}
	}
}

func TestCanonicalChannelErrorDoesNotLeakSecret(t *testing.T) {
	_, err := CanonicalChannel("https://user:hunter2@[bad/chan")
	if err == nil {
		t.Fatal("malformed channel accepted")
	}
	if strings.Contains(err.Error(), "hunter2") {
		t.Fatalf("error leaks secret: %v", err)
	}
}

func TestParseMatchSpec(t *testing.T) {
	got, err := ParseMatchSpec("numpy==1.26.0 py311h0_0")
	if err != nil {
		t.Fatalf("ParseMatchSpec error = %v", err)
	}
	if got.Name != "numpy" || got.Version == "" || got.Build != "py311h0_0" || !got.Pinned {
		t.Fatalf("ParseMatchSpec = %+v; want pinned numpy with build", got)
	}
	loose, err := ParseMatchSpec("numpy >=1.20")
	if err != nil || loose.Pinned {
		t.Fatalf("range spec = %+v, %v; want Pinned=false", loose, err)
	}
	versionOnly, err := ParseMatchSpec("numpy==1.26.0")
	if err != nil || versionOnly.Pinned {
		t.Fatalf("version-only pin = %+v, %v; want Pinned=false without exact build", versionOnly, err)
	}
	exclusion, err := ParseMatchSpec("numpy!=1.26.0 py311h0_0")
	if err != nil || exclusion.Pinned {
		t.Fatalf("exclusion spec = %+v, %v; want Pinned=false", exclusion, err)
	}
}

func TestParseMatchSpecRejectsMalformed(t *testing.T) {
	for _, spec := range []string{"", "numpy [", "numpy 1.0 [build=a, build=b]", "num*py"} {
		if _, err := ParseMatchSpec(spec); err == nil {
			t.Errorf("ParseMatchSpec(%q) succeeded; want error", spec)
		}
	}
}

func testRecord() PackageRecord {
	return PackageRecord{
		Name:        "numpy",
		Version:     "1.26.0",
		Build:       "py311h0_0",
		BuildNumber: 3,
		Subdir:      "linux-64",
		Depends:     []string{"python >=3.11"},
	}
}

func TestMatchSpecMatches(t *testing.T) {
	rec := testRecord()
	for _, tt := range []struct {
		spec string
		want bool
	}{
		{"numpy >=1.20", true},
		{"numpy <1.0", false},
		{"numpy >=1.20 py311h0_0", true},
		{"numpy >=1.20 py310*", false},
		{"numpy >=1.20[build_number=\">=3\"]", true},
		{"numpy >=1.20[build_number=\">=4\"]", false},
		{"numpy >=1.20[md5=00000000000000000000000000000000]", false},
	} {
		got, err := MatchSpecMatches(tt.spec, rec)
		if err != nil || got != tt.want {
			t.Errorf("MatchSpecMatches(%q) = %v, %v; want %v", tt.spec, got, err, tt.want)
		}
	}
}

func TestMatchSpecMatchesRejectsUnverifiableConstraints(t *testing.T) {
	rec := testRecord()
	for _, spec := range []string{
		"conda-forge::numpy",
		"numpy[subdir=linux-64]",
		"numpy[channel=conda-forge]",
		"numpy[url=https://example.com/numpy.conda]",
	} {
		got, err := MatchSpecMatches(spec, rec)
		if err == nil {
			t.Errorf("MatchSpecMatches(%q) = %v, nil; want ErrInvalidInput", spec, got)
			continue
		}
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("MatchSpecMatches(%q) error = %v; want ErrInvalidInput", spec, err)
		}
	}
}

func TestMatchSpecMatchesRejectsEmptySpec(t *testing.T) {
	if _, err := MatchSpecMatches("", testRecord()); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("empty spec error = %v; want ErrInvalidInput", err)
	}
}

func TestVersionSpecContains(t *testing.T) {
	for _, tt := range []struct {
		spec, version string
		want          bool
	}{
		{">=1.2", "1.3", true},
		{"1.2.*", "1.3", false},
		{"1.2.*", "1.2.5", true},
		{"!=1.0", "1.0", false},
		{">=1.0,<2.0", "1.5", true},
		{">=1.0,<2.0", "2.0", false},
		{"==1!1.0", "1!1.0", true},
		{"==1!1.0", "1.99", false},
		{"<1.0", "1.0rc1", true},
		{"*", "0.0.1", true},
	} {
		got, err := VersionSpecContains(tt.spec, tt.version)
		if err != nil || got != tt.want {
			t.Errorf("VersionSpecContains(%q, %q) = %v, %v; want %v", tt.spec, tt.version, got, err, tt.want)
		}
	}
}

func TestVersionSpecContainsInvalid(t *testing.T) {
	for _, tt := range []struct{ spec, version string }{
		{"", "1.0"},
		{"!!bad", "1.0"},
		{">=1.0", "bad version!"},
	} {
		if _, err := VersionSpecContains(tt.spec, tt.version); err == nil {
			t.Errorf("VersionSpecContains(%q, %q) succeeded; want error", tt.spec, tt.version)
		}
	}
}

func TestParseEnvironmentYAML(t *testing.T) {
	doc := []byte("name: demo\n" +
		"channels: [conda-forge]\n" +
		"dependencies:\n" +
		"  - numpy\n" +
		"  - pip:\n" +
		"      - requests\n" +
		"  - pip:\n" +
		"      - click\n" +
		"variables:\n" +
		"  KEY: value\n")
	env, err := ParseEnvironmentYAML(doc)
	if err != nil {
		t.Fatalf("ParseEnvironmentYAML error = %v", err)
	}
	if env.Name != "demo" {
		t.Errorf("Name = %q", env.Name)
	}
	if len(env.Channels) != 1 || env.Channels[0] != "conda-forge" {
		t.Errorf("Channels = %v", env.Channels)
	}
	if len(env.Dependencies) != 1 || env.Dependencies[0] != "numpy" {
		t.Errorf("Dependencies = %v", env.Dependencies)
	}
	pip := env.Subsections["pip"]
	if len(pip) != 2 || pip[0] != "requests" || pip[1] != "click" {
		t.Errorf("pip subsections = %v; want both blocks in order", pip)
	}
	if env.Variables["KEY"] != "value" {
		t.Errorf("Variables = %v", env.Variables)
	}
}

func TestParseEnvironmentYAMLRejects(t *testing.T) {
	for name, doc := range map[string][]byte{
		"invalid utf8": {0xff, 0xfe, 0xfd},
		"invalid yaml": []byte("name: [unclosed\n"),
	} {
		if _, err := ParseEnvironmentYAML(doc); err == nil {
			t.Errorf("%s: ParseEnvironmentYAML succeeded; want error", name)
		}
	}
}

func TestParseExplicitSpec(t *testing.T) {
	doc := []byte("# platform: linux-64\n" +
		"@EXPLICIT\n" +
		"https://conda.anaconda.org/conda-forge/linux-64/a-1.tar.bz2#0123456789abcdef0123456789abcdef\n" +
		"https://conda.anaconda.org/conda-forge/linux-64/b-1.conda\n")
	spec, err := ParseExplicitSpec(doc)
	if err != nil {
		t.Fatalf("ParseExplicitSpec error = %v", err)
	}
	if spec.Platform != "linux-64" {
		t.Errorf("Platform = %q", spec.Platform)
	}
	if len(spec.Packages) != 2 {
		t.Fatalf("Packages = %+v", spec.Packages)
	}
	a, b := spec.Packages[0], spec.Packages[1]
	if a.URL != "https://conda.anaconda.org/conda-forge/linux-64/a-1.tar.bz2" || a.MD5 != "0123456789abcdef0123456789abcdef" {
		t.Errorf("first package = %+v; want fragment split into MD5", a)
	}
	if b.URL != "https://conda.anaconda.org/conda-forge/linux-64/b-1.conda" {
		t.Errorf("second package URL = %q; want install order preserved", b.URL)
	}
}

func TestParseExplicitSpecRejects(t *testing.T) {
	for name, doc := range map[string][]byte{
		"missing tag":    []byte("https://h/a.conda\n"),
		"garbage":        []byte("this is not explicit\n"),
		"bad md5 length": []byte("@EXPLICIT\nhttps://h/a.conda#abc\n"),
		"malformed URL":  []byte("@EXPLICIT\nnot a url\n"),
	} {
		if _, err := ParseExplicitSpec(doc); err == nil {
			t.Errorf("%s: ParseExplicitSpec succeeded; want error", name)
		}
	}
}

func TestSubdirFromOCIPlatform(t *testing.T) {
	for _, tt := range []struct {
		os, arch, variant, want string
	}{
		{"linux", "amd64", "", "linux-64"},
		{"linux", "arm64", "", "linux-aarch64"},
		{"linux", "arm64", "v8", "linux-aarch64"},
		{"linux", "386", "", "linux-32"},
		{"linux", "ppc64le", "", "linux-ppc64le"},
		{"linux", "s390x", "", "linux-s390x"},
		{"linux", "riscv64", "", "linux-riscv64"},
		{"linux", "arm", "v6", "linux-armv6l"},
		{"linux", "arm", "v7", "linux-armv7l"},
		{"darwin", "amd64", "", "osx-64"},
		{"darwin", "arm64", "", "osx-arm64"},
		{"windows", "amd64", "", "win-64"},
		{"windows", "386", "", "win-32"},
	} {
		got, err := SubdirFromOCIPlatform(tt.os, tt.arch, tt.variant)
		if err != nil || got != tt.want {
			t.Errorf("SubdirFromOCIPlatform(%q,%q,%q) = %q, %v; want %q", tt.os, tt.arch, tt.variant, got, err, tt.want)
		}
	}
}

func TestSubdirFromOCIPlatformRejects(t *testing.T) {
	for _, tt := range []struct{ os, arch, variant string }{
		{"", "amd64", ""},
		{"linux", "", ""},
		{"linux", "arm64", "v9"},
		{"linux", "amd64", "v8"},
		{"linux", "arm", ""},
		{"plan9", "amd64", ""},
	} {
		if _, err := SubdirFromOCIPlatform(tt.os, tt.arch, tt.variant); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("SubdirFromOCIPlatform(%q,%q,%q) error = %v; want ErrInvalidInput", tt.os, tt.arch, tt.variant, err)
		}
	}
}
