//go:build cgo

package gorattler

import "testing"

func TestParseArchiveFilename(t *testing.T) {
	cases := []struct {
		in   string
		name string
		ver  string
		bld  string
		typ  ArchiveType
	}{
		{"my-pkg-1.0-py_0.conda", "my-pkg", "1.0", "py_0", ArchiveTypeConda},
		{"foo-2.0-0.tar.bz2", "foo", "2.0", "0", ArchiveTypeTarBz2},
	}
	for _, tc := range cases {
		id, err := ParseArchiveFilename(tc.in)
		if err != nil {
			t.Fatalf("%s: %v", tc.in, err)
		}
		if id.Name != tc.name || id.Version != tc.ver || id.Build != tc.bld || id.Type != tc.typ {
			t.Fatalf("%s = %+v", tc.in, id)
		}
	}
}

func TestParseArchiveFilenameRejects(t *testing.T) {
	for _, bad := range []string{"", "dir/foo-1-0.conda", `dir\foo-1-0.conda`, "foo-1-0.zip", "foo-1.conda", "foo-1-0.tar"} {
		if _, err := ParseArchiveFilename(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
