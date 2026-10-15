# go-rattler

Experimental Go bindings for selected [rattler](https://github.com/conda/rattler) functionality. `CompareVersions` calls `rattler_conda_version`; `LockEnvironment` counts packages in one `pixi.lock` environment and platform using `rattler_lock`. The latter does not check whether an installed environment matches the lockfile.

## How it works

The public Go package calls `internal/api`, which passes C-compatible values through cgo to one Rust `staticlib`. The Rust adapter in `internal/native/` calls the relevant rattler crates. Its code is split by domain (`version.rs`, `lock.rs`), but both functions share one C header and one static library. This keeps the Go API separate from raw pointers and status codes, and avoids linking multiple Rust static libraries into the same executable. New bindings add only the Rust dependencies they need.

We use plain `cargo build` because Go currently needs one static archive, not an installable C SDK. [cargo-c](https://github.com/lu-zero/cargo-c) can also generate headers and package static/shared libraries with `pkg-config` metadata, but it would add a tool requirement without making `go build` invoke Cargo or automatically bind more rattler APIs. The two declarations in `internal/native/include/go_rattler.h` are maintained by hand; as the ABI grows, header generation and a CI drift check are worth considering, with or without cargo-c.

## Build from source

Requires Go 1.26+, Rust/Cargo, cgo, and a C compiler. `go build` does **not** run Cargo. From a consuming Go module, build the exact Go dependency's Rust source first, then link its archive into your final executable:

```sh
module=github.com/conda-incubator/go-rattler # the path currently declared in go.mod
go get "$module@<version-or-commit>" # omit if already pinned in go.mod
go mod download "$module"
source_dir="$(go list -m -f '{{.Dir}}' "$module")"
native_dir="$PWD/.go-rattler-native" # writable; git-ignore this directory
CARGO_TARGET_DIR="$native_dir" cargo build --manifest-path "$source_dir/internal/native/Cargo.toml" --release --locked
CGO_ENABLED=1 CGO_LDFLAGS="-L$native_dir/release" go build -o my-program ./cmd/my-program
```

The Go module download supplies the Rust source, but `go build` does not compile it: run Cargo first, in a writable target directory outside the downloaded module. cgo then links `libgo_rattler_native.a` into the executable; no separate Rust library is needed at runtime, although OS libraries may still be dynamic. On Windows amd64, use MinGW-w64 for cgo, install the `x86_64-pc-windows-gnu` Rust target, set `CARGO_BUILD_TARGET=x86_64-pc-windows-gnu`, and point `CGO_LDFLAGS` at `$native_dir/x86_64-pc-windows-gnu/release`. The wrapper includes the required Windows system import libraries. Bare `go get` followed by `go build` is not enough; native archives are not distributed. Locally, `make check` builds the archive and tests both languages.

> [!NOTE]
> Graduation of this project means upstreaming into [rattler](https://github.com/conda/rattler).

Licensed under BSD-3-Clause, matching rattler.
