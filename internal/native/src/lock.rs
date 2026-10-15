use rattler_lock::LockFile;
use std::panic::catch_unwind;

use crate::ffi_str;

/// Count locked conda and PyPI packages in a named environment and platform.
/// Status: 0=success, 1=invalid input, 2=unreadable/invalid lockfile,
/// 3=missing environment, 4=missing platform in that environment, 5=internal failure.
/// Outputs are written only on success.
///
/// # Safety
/// Non-null input pointers must reference readable buffers of the given lengths;
/// output pointers must reference separate writable u32 values. Rust cannot
/// validate foreign pointer provenance or caller-provided buffer lengths.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn go_rattler_lock_environment_counts(
    lock_path: *const u8,
    lock_path_len: usize,
    environment: *const u8,
    environment_len: usize,
    platform: *const u8,
    platform_len: usize,
    conda_count: *mut u32,
    pypi_count: *mut u32,
) -> i32 {
    if conda_count.is_null() || pypi_count.is_null() || conda_count == pypi_count {
        return 1;
    }
    catch_unwind(|| {
        let (Ok(path), Ok(name), Ok(platform_name)) = (
            unsafe { ffi_str(lock_path, lock_path_len) },
            unsafe { ffi_str(environment, environment_len) },
            unsafe { ffi_str(platform, platform_len) },
        ) else {
            return 1;
        };
        if path.is_empty() || name.is_empty() || platform_name.is_empty() {
            return 1;
        }
        let Ok(lock) = LockFile::from_path(std::path::Path::new(path)) else {
            return 2;
        };
        let Some(env) = lock.environment(name) else {
            return 3;
        };
        let Some(platform) = lock.platform(platform_name) else {
            return 4;
        };
        let Some(packages) = env.packages(platform) else {
            return 4;
        };
        let (mut conda, mut pypi) = (0u32, 0u32);
        for package in packages {
            if package.as_conda().is_some() {
                conda += 1;
            } else if package.as_pypi().is_some() {
                pypi += 1;
            }
        }
        unsafe {
            *conda_count = conda;
            *pypi_count = pypi;
        }
        0
    })
    .unwrap_or(5)
}
