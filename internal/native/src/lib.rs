mod lock;
mod version;

use std::slice;

unsafe fn ffi_str<'a>(ptr: *const u8, len: usize) -> Result<&'a str, ()> {
    if ptr.is_null() && len != 0 {
        return Err(());
    }
    let bytes = if len == 0 {
        &[][..]
    } else {
        unsafe { slice::from_raw_parts(ptr, len) }
    };
    std::str::from_utf8(bytes).map_err(|_| ())
}
