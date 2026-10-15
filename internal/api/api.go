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

// CompareVersions returns the Rust comparison result and its status code.
func CompareVersions(a, b string) (int, int) {
	first := C.CBytes([]byte(a))
	defer C.free(first)
	second := C.CBytes([]byte(b))
	defer C.free(second)
	var result C.int32_t
	status := C.go_rattler_compare_versions(
		(*C.uint8_t)(first), C.size_t(len(a)),
		(*C.uint8_t)(second), C.size_t(len(b)), &result,
	)
	return int(result), int(status)
}

// LockEnvironmentCounts returns counts and the Rust lookup status code.
func LockEnvironmentCounts(lockPath, name, platform string) (int, int, int) {
	path := C.CBytes([]byte(lockPath))
	defer C.free(path)
	env := C.CBytes([]byte(name))
	defer C.free(env)
	target := C.CBytes([]byte(platform))
	defer C.free(target)
	var conda, pypi C.uint32_t
	status := C.go_rattler_lock_environment_counts(
		(*C.uint8_t)(path), C.size_t(len(lockPath)),
		(*C.uint8_t)(env), C.size_t(len(name)),
		(*C.uint8_t)(target), C.size_t(len(platform)),
		&conda, &pypi,
	)
	return int(conda), int(pypi), int(status)
}
