//go:build cgo

package gorattler

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseIndexJSON(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join("testdata", "records", "fixtures", "index", "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	index, err := ParseIndexJSON(doc)
	if err != nil {
		t.Fatalf("ParseIndexJSON: %v", err)
	}
	if index.Name != "envscout-demo" || index.Version != "1.2.3" || index.Build != "py311_0" {
		t.Fatalf("index = %+v", index)
	}
}

func TestParseIndexJSONRejectsInvalid(t *testing.T) {
	cases := map[string][]byte{
		"empty name":    []byte(`{"name":"","version":"1.0","build":"0","build_number":0}`),
		"trailing json": []byte(`{"name":"a","version":"1.0","build":"0","build_number":0} {}`),
		"invalid utf8":  {0xff, 0xfe},
		"bad build":     []byte(`{"name":"a","version":"1.0","build":"","build_number":0}`),
	}
	for name, doc := range cases {
		if _, err := ParseIndexJSON(doc); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}
