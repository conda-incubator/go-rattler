use rattler_conda_version::Version;
use std::{cmp::Ordering, panic::catch_unwind, str::FromStr};

use crate::ffi_str;

/// Compare two conda versions through a C-compatible function.
/// Status: 0=success, 1=invalid first version, 2=invalid second version,
/// 3=invalid input pointer/UTF-8, 4=internal failure. Output is written only on success.
///
/// # Safety
/// Non-null inputs must point to readable buffers of the corresponding lengths;
/// `comparison` must point to a writable i32. Rust cannot validate foreign
/// pointer provenance or ensure that a caller-supplied length is accurate.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn go_rattler_compare_versions(
    a: *const u8,
    a_len: usize,
    b: *const u8,
    b_len: usize,
    comparison: *mut i32,
) -> i32 {
    if comparison.is_null() || (a.is_null() && a_len != 0) || (b.is_null() && b_len != 0) {
        return 3;
    }
    catch_unwind(|| {
        let (Ok(first), Ok(second)) = (unsafe { ffi_str(a, a_len) }, unsafe { ffi_str(b, b_len) })
        else {
            return 3;
        };
        let Ok(first) = Version::from_str(first) else {
            return 1;
        };
        let Ok(second) = Version::from_str(second) else {
            return 2;
        };
        let value = match first.cmp(&second) {
            Ordering::Less => -1,
            Ordering::Equal => 0,
            Ordering::Greater => 1,
        };
        unsafe { *comparison = value };
        0
    })
    .unwrap_or(4)
}

#[cfg(test)]
mod tests {
    use super::go_rattler_compare_versions;

    #[test]
    fn compares_conda_versions() {
        let mut result = 99;
        let status = unsafe {
            go_rattler_compare_versions(b"1.0rc1".as_ptr(), 6, b"1.0".as_ptr(), 3, &mut result)
        };
        assert_eq!((status, result), (0, -1));
    }

    #[test]
    fn rejects_bad_pointer_without_writing_result() {
        let mut result = 99;
        let status = unsafe {
            go_rattler_compare_versions(std::ptr::null(), 1, b"1".as_ptr(), 1, &mut result)
        };
        assert_eq!((status, result), (3, 99));
    }
}
