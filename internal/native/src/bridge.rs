//! Shared generic JSON C ABI foundation.
//!
//! One exported entry point, `go_rattler_json`, takes a numeric operation ID and
//! a JSON request, dispatches to a domain module, and returns a JSON response in
//! a Rust-allocated byte buffer that the caller releases with
//! `go_rattler_json_free`.
//!
//! Contract:
//! - `output` must be non-null and point to a `JsonBuffer` that the caller has
//!   zero-initialized (or freed previously). A non-empty buffer is rejected.
//! - Every call that writes to `output` (including error statuses) leaves an
//!   owned buffer behind; the caller MUST call `go_rattler_json_free` after
//!   every call that returned with a non-null output pointer, regardless of status.
//! - `input` may be null only when `input_len == 0`.
//! - `input_len` must not exceed `isize::MAX`.
//! - Freeing a `JsonBuffer` whose `data` is null is a no-op. Freeing twice is safe
//!   because `go_rattler_json_free` zeroes the struct. Foreign pointers that were
//!   not produced by this library are not detected and must not be freed here.

use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::panic::{AssertUnwindSafe, catch_unwind};
use std::slice;

/// Status: success; `output` holds the JSON result.
pub const STATUS_OK: i32 = 0;
/// Status: the domain rejected the request; `output` holds `{"error": "..."}`.
pub const STATUS_DOMAIN_ERROR: i32 = 2;
/// Status: an input/output pointer is invalid, input is not UTF-8, or the JSON is malformed.
pub const STATUS_INVALID_INPUT: i32 = 3;
/// Status: a panic or internal failure occurred; `output` holds `{"error": "..."}`.
pub const STATUS_INTERNAL: i32 = 4;

#[cfg(test)]
const TEST_PANIC_OP: u32 = u32::MAX;

/// Owned byte buffer returned to C.
///
/// The memory is allocated by Rust as a `Box<[u8]>`. `data` is null when empty.
#[repr(C)]
pub struct JsonBuffer {
    pub data: *mut u8,
    pub len: usize,
}

/// Error envelope serialized as `{"error": "..."}` for non-success statuses.
#[derive(Serialize, Deserialize)]
struct ErrorEnvelope {
    error: String,
}

/// Decodes a standard-alphabet, padded base64 document.
pub fn decode_document(encoded: &str) -> Result<Vec<u8>, String> {
    use base64::Engine;
    base64::engine::general_purpose::STANDARD
        .decode(encoded)
        .map_err(|err| format!("invalid base64 document: {err}"))
}

/// Maps a stable numeric operation ID to a domain dispatch.
pub fn dispatch(operation: u32, value: Value) -> Result<Value, String> {
    match operation {
        #[cfg(test)]
        TEST_PANIC_OP => panic!("test-only panic"),
        1..=5 | 14 => crate::records::dispatch(operation, value),
        6..=11 | 15 => crate::semantics::dispatch(operation, value),
        12 => crate::lock_json::lock_packages(value),
        13 => crate::lock_json::lock_sync(value),
        _ => Err(format!("unknown operation {operation}")),
    }
}

/// Views a raw input region as bytes, enforcing the pointer and length contract.
///
/// # Safety
/// When `len > 0`, `ptr` must be readable for `len` bytes for the duration of the call.
unsafe fn input_bytes<'a>(ptr: *const u8, len: usize) -> Result<&'a [u8], ()> {
    if len > isize::MAX as usize {
        return Err(());
    }
    if len == 0 {
        return Ok(&[]);
    }
    if ptr.is_null() {
        return Err(());
    }
    // SAFETY: caller contract guarantees `ptr` is readable for `len` bytes; the
    // length bound above satisfies `from_raw_parts`'s preconditions.
    Ok(unsafe { slice::from_raw_parts(ptr, len) })
}

/// Converts an owned byte vector into a `JsonBuffer` with an exact-length box.
fn into_buffer(bytes: Vec<u8>) -> JsonBuffer {
    if bytes.is_empty() {
        return JsonBuffer {
            data: std::ptr::null_mut(),
            len: 0,
        };
    }
    let boxed: Box<[u8]> = bytes.into_boxed_slice();
    let len = boxed.len();
    let data = Box::into_raw(boxed) as *mut u8;
    JsonBuffer { data, len }
}

/// Serializes an error envelope, falling back to a fixed message if serialization fails.
fn error_bytes(message: impl Into<String>) -> Vec<u8> {
    serde_json::to_vec(&ErrorEnvelope {
        error: message.into(),
    })
    .unwrap_or_else(|_| br#"{"error":"internal error"}"#.to_vec())
}

/// Runs one JSON operation and returns its status and response bytes.
fn run(operation: u32, input: &[u8]) -> (i32, Vec<u8>) {
    let value: Value = match serde_json::from_slice(input) {
        Ok(value) => value,
        Err(err) => {
            return (
                STATUS_INVALID_INPUT,
                error_bytes(format!("invalid JSON: {err}")),
            );
        }
    };
    match catch_unwind(AssertUnwindSafe(|| dispatch(operation, value))) {
        Ok(Ok(result)) => match serde_json::to_vec(&result) {
            Ok(bytes) => (STATUS_OK, bytes),
            Err(_) => (STATUS_INTERNAL, error_bytes("cannot serialize result")),
        },
        Ok(Err(message)) => (STATUS_DOMAIN_ERROR, error_bytes(message)),
        Err(_) => (STATUS_INTERNAL, error_bytes("internal panic")),
    }
}

/// Executes one JSON operation.
///
/// # Safety
/// - `output` must be non-null, aligned, writable, and zero-initialized or freed.
/// - When `input_len > 0`, `input` must be non-null and readable for `input_len` bytes.
/// - The caller must call `go_rattler_json_free` on `output` after every call that
///   returns a status, including errors.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn go_rattler_json(
    operation: u32,
    input: *const u8,
    input_len: usize,
    output: *mut JsonBuffer,
) -> i32 {
    if output.is_null() {
        return STATUS_INVALID_INPUT;
    }
    // SAFETY: `output` is non-null and, per the contract, points to a writable JsonBuffer.
    let out = unsafe { &mut *output };
    if !out.data.is_null() || out.len != 0 {
        return STATUS_INVALID_INPUT;
    }
    let outcome = catch_unwind(AssertUnwindSafe(|| {
        // SAFETY: forwarded from this function's caller contract.
        match unsafe { input_bytes(input, input_len) } {
            Ok(bytes) => run(operation, bytes),
            Err(()) => (
                STATUS_INVALID_INPUT,
                error_bytes("invalid input pointer or length"),
            ),
        }
    }));
    let (status, bytes) = match outcome {
        Ok(pair) => pair,
        Err(_) => (STATUS_INTERNAL, error_bytes("internal panic")),
    };
    *out = into_buffer(bytes);
    status
}

/// Releases a buffer returned by `go_rattler_json` and zeroes the struct.
///
/// Null `data` is a no-op. Calling this on an already-zeroed struct is safe.
///
/// # Safety
/// `buffer` must be null or point to a `JsonBuffer` written by `go_rattler_json`
/// that has not been freed since. Foreign pointers are not detected.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn go_rattler_json_free(buffer: *mut JsonBuffer) {
    if buffer.is_null() {
        return;
    }
    // SAFETY: non-null, and per contract points to a JsonBuffer from go_rattler_json.
    let buf = unsafe { &mut *buffer };
    let data = std::mem::replace(&mut buf.data, std::ptr::null_mut());
    let len = std::mem::replace(&mut buf.len, 0);
    if data.is_null() {
        return;
    }
    // SAFETY: `data`/`len` came from `Box::<[u8]>::into_raw` in `into_buffer`
    // and were taken out of the struct so this cannot be freed twice.
    let _ = catch_unwind(AssertUnwindSafe(|| unsafe {
        drop(Box::from_raw(std::ptr::slice_from_raw_parts_mut(data, len)));
    }));
}

#[cfg(test)]
mod tests {
    use super::*;

    fn call(operation: u32, input: &[u8]) -> (i32, String) {
        let mut out = JsonBuffer {
            data: std::ptr::null_mut(),
            len: 0,
        };
        let ptr = if input.is_empty() {
            std::ptr::null()
        } else {
            input.as_ptr()
        };
        let status = unsafe { go_rattler_json(operation, ptr, input.len(), &mut out) };
        let text = if out.data.is_null() {
            String::new()
        } else {
            let bytes = unsafe { slice::from_raw_parts(out.data, out.len) };
            String::from_utf8(bytes.to_vec()).unwrap()
        };
        unsafe { go_rattler_json_free(&mut out) };
        (status, text)
    }

    #[test]
    fn null_output_is_rejected_without_dereference() {
        let status = unsafe { go_rattler_json(1, std::ptr::null(), 0, std::ptr::null_mut()) };
        assert_eq!(status, STATUS_INVALID_INPUT);
    }

    #[test]
    fn nonempty_output_is_rejected() {
        let mut bytes = b"x".to_vec().into_boxed_slice();
        let mut out = JsonBuffer {
            data: bytes.as_mut_ptr(),
            len: bytes.len(),
        };
        let status = unsafe { go_rattler_json(1, std::ptr::null(), 0, &mut out) };
        assert_eq!(status, STATUS_INVALID_INPUT);
    }

    #[test]
    fn null_input_with_nonzero_length_is_rejected() {
        let (status, text) = {
            let mut out = JsonBuffer {
                data: std::ptr::null_mut(),
                len: 0,
            };
            let status = unsafe { go_rattler_json(1, std::ptr::null(), 4, &mut out) };
            let text =
                String::from_utf8(unsafe { slice::from_raw_parts(out.data, out.len) }.to_vec())
                    .unwrap();
            unsafe { go_rattler_json_free(&mut out) };
            (status, text)
        };
        assert_eq!(status, STATUS_INVALID_INPUT);
        assert!(text.contains("invalid input pointer"));
    }

    #[test]
    fn oversized_length_is_rejected() {
        let mut out = JsonBuffer {
            data: std::ptr::null_mut(),
            len: 0,
        };
        let status =
            unsafe { go_rattler_json(1, b"{}".as_ptr(), isize::MAX as usize + 1, &mut out) };
        assert_eq!(status, STATUS_INVALID_INPUT);
        unsafe { go_rattler_json_free(&mut out) };
    }

    #[test]
    fn empty_input_is_invalid_json() {
        let (status, text) = call(1, b"");
        assert_eq!(status, STATUS_INVALID_INPUT);
        assert!(text.starts_with(r#"{"error":"invalid JSON"#));
    }

    #[test]
    fn invalid_utf8_is_rejected_as_json() {
        let (status, _) = call(1, &[0xff, 0xfe, b'{']);
        assert_eq!(status, STATUS_INVALID_INPUT);
    }

    #[test]
    fn trailing_json_is_rejected() {
        let (status, text) = call(1, br#"{"a":1} {"b":2}"#);
        assert_eq!(status, STATUS_INVALID_INPUT);
        assert!(text.contains("invalid JSON"));
    }

    #[test]
    fn unknown_operation_is_domain_error() {
        let (status, text) = call(999, b"{}");
        assert_eq!(status, STATUS_DOMAIN_ERROR);
        assert_eq!(text, r#"{"error":"unknown operation 999"}"#);
    }

    #[test]
    fn panic_is_contained_before_crossing_the_abi() {
        let (status, body) = call(TEST_PANIC_OP, b"{}");
        assert_eq!(status, STATUS_INTERNAL);
        assert_eq!(body, r#"{"error":"internal panic"}"#);
    }

    #[test]
    fn repeated_free_is_idempotent() {
        let mut out = JsonBuffer {
            data: std::ptr::null_mut(),
            len: 0,
        };
        let _ = unsafe { go_rattler_json(999, b"{}".as_ptr(), 2, &mut out) };
        unsafe { go_rattler_json_free(&mut out) };
        assert!(out.data.is_null() && out.len == 0);
        unsafe { go_rattler_json_free(&mut out) };
        unsafe { go_rattler_json_free(std::ptr::null_mut()) };
    }

    #[test]
    fn empty_result_uses_null_buffer() {
        let buf = into_buffer(Vec::new());
        assert!(buf.data.is_null() && buf.len == 0);
    }

    #[test]
    fn document_decodes_standard_base64() {
        assert_eq!(decode_document("aGVsbG8=").unwrap(), b"hello");
        assert!(decode_document("!!!").is_err());
    }
}
