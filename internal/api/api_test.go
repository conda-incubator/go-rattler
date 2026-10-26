//go:build cgo

package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

func TestNativeJSONTransport(t *testing.T) {
	response, err := JSON(OperationCompareVersions, []byte(`{"a":"1","b":"2"}`))
	if err != nil || string(response) != "-1" {
		t.Fatalf("native JSON result: %s, %v", response, err)
	}
	for _, request := range [][]byte{nil, []byte("{"), []byte{0xff}, []byte(`{} {}`)} {
		_, err := JSON(OperationCompareVersions, request)
		var native *Status
		if !errors.As(err, &native) || native.Code != 3 {
			t.Fatalf("invalid request returned %v", err)
		}
	}
	_, err = JSON(999, []byte("{}"))
	var native *Status
	if !errors.As(err, &native) || native.Code != 2 {
		t.Fatalf("unknown operation returned %v", err)
	}
}

func TestLargeNativeResultRemainsOwnedByGo(t *testing.T) {
	files := make([]string, 10000)
	for i := range files {
		files[i] = fmt.Sprintf("lib/python3.13/site-packages/package/module_%05d.py", i)
	}
	document, err := json.Marshal(map[string]any{
		"name": "demo", "version": "1", "build": "0", "build_number": 0,
		"fn": "demo-1-0.conda", "url": "https://example.test/demo-1-0.conda", "files": files,
	})
	if err != nil {
		t.Fatal(err)
	}
	request, err := json.Marshal(map[string]string{"document": base64.StdEncoding.EncodeToString(document)})
	if err != nil {
		t.Fatal(err)
	}
	response, err := JSON(OperationParsePrefix, request)
	if err != nil || len(response) < 500000 {
		t.Fatalf("large native response: %d bytes, %v", len(response), err)
	}
	saved := bytes.Clone(response)
	for range 20 {
		if _, err := JSON(OperationParsePrefix, request); err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Equal(response, saved) {
		t.Fatal("reusing native result buffers changed an earlier Go-owned response")
	}
}
