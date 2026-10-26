//go:build cgo

package gorattler

import (
	"errors"
	"sync"
	"testing"
)

func TestConcurrentNativeResultsAndErrors(t *testing.T) {
	document := []byte(`{"name":"demo","version":"1.2.3","build":"py_0","build_number":0,"subdir":"linux-64","fn":"demo-1.2.3-py_0.conda","url":"https://example.test/linux-64/demo-1.2.3-py_0.conda","files":["bin/demo"]}`)
	var workers sync.WaitGroup
	for range 8 {
		workers.Go(func() {
			for range 50 {
				record, err := ParsePrefixRecord(document)
				if err != nil || record.Name != "demo" || record.Version != "1.2.3" {
					t.Errorf("concurrent record result: %+v, %v", record, err)
					return
				}
				comparison, err := CompareVersions("1.2.3", "1.2.4")
				if err != nil || comparison != -1 {
					t.Errorf("concurrent version result: %d, %v", comparison, err)
					return
				}
				if _, err := ParsePrefixRecord([]byte("not JSON")); !errors.Is(err, ErrInvalidInput) {
					t.Errorf("native error identity lost: %v", err)
					return
				}
			}
		})
	}
	workers.Wait()
}

func TestMalformedGoRequestAndResponseAreClassified(t *testing.T) {
	var output any
	if err := callJSON(channelOperation, make(chan int), &output); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unencodable request must be invalid input: %v", err)
	}
	var wrongType string
	request := struct {
		A string `json:"a"`
		B string `json:"b"`
	}{"1", "2"}
	if err := callJSON(compareVersionsOperation, request, &wrongType); !errors.Is(err, ErrNative) {
		t.Fatalf("wrong response projection must be an internal error: %v", err)
	}
}

func TestNativeErrorExposesOperationAndStatus(t *testing.T) {
	_, err := ParsePrefixRecord([]byte("not JSON"))
	var native *Error
	if !errors.Is(err, ErrInvalidInput) || !errors.As(err, &native) || native.Operation != parsePrefixOperation || native.Status != 2 {
		t.Fatalf("native domain error lost its operation/status: %v", err)
	}
	if errors.Is(err, ErrNative) {
		t.Fatalf("invalid document must not be classified as an internal failure: %v", err)
	}
}
