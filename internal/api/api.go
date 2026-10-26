//go:build cgo

// Package api contains the private C ABI bridge to the Rust rattler adapter.
package api

/*
#cgo CFLAGS: -I${SRCDIR}/../native/include
#cgo LDFLAGS: -lgo_rattler_native
#cgo darwin LDFLAGS: -framework CoreFoundation -framework Security -framework SystemConfiguration
#cgo windows LDFLAGS: -ladvapi32 -lbcrypt -lntdll -luserenv -lws2_32
#include "go_rattler.h"
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"math"
	"unsafe"
)

// JSON runs one generic JSON operation through the Rust bridge. The returned
// bytes are a Go-owned copy of the response; Rust memory is always released.
// On a non-zero status the bytes hold the Rust {"error": "..."} envelope and
// err is a *Status describing it.
func JSON(operation uint32, input []byte) ([]byte, error) {
	var inputPtr *C.uint8_t
	if len(input) > 0 {
		cin := C.CBytes(input)
		defer C.free(cin)
		inputPtr = (*C.uint8_t)(cin)
	}
	var output C.JsonBuffer
	defer C.go_rattler_json_free(&output)
	status := C.go_rattler_json(C.uint32_t(operation), inputPtr, C.size_t(len(input)), &output)
	if uint64(output.len) > uint64(math.MaxInt) || (output.len != 0 && output.data == nil) {
		return nil, &Status{Operation: operation, Code: 4, Body: []byte(`{"error":"invalid native response buffer"}`)}
	}
	body := copyBuffer(&output)
	if status != C.GO_RATTLER_STATUS_OK {
		return body, &Status{Operation: operation, Code: int(status), Body: body}
	}
	return body, nil
}

// copyBuffer copies the Rust-owned bytes into Go memory. A NULL buffer yields nil.
func copyBuffer(buf *C.JsonBuffer) []byte {
	if buf.data == nil || buf.len == 0 {
		return nil
	}
	src := unsafe.Slice((*byte)(unsafe.Pointer(buf.data)), int(buf.len))
	out := make([]byte, len(src))
	copy(out, src)
	return out
}

// Status reports a non-success JSON bridge status. Body holds the raw response.
type Status struct {
	Operation uint32
	Code      int
	Body      []byte
}

func (s *Status) Error() string {
	return fmt.Sprintf("native operation %d failed with status %d", s.Operation, s.Code)
}
