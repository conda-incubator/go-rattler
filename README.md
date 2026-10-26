# go-rattler

Experimental Go bindings for selected [rattler](https://github.com/conda/rattler) functionality, intended for offline environment inspection. Parsing and conda semantics run in Rust; callers supply document bytes and installed-package observations, rather than invoking conda or Pixi subprocesses.

## Inspection APIs

| Area | Functions |
|---|---|
| Installed conda metadata | `ParsePrefixRecord`, `ParsePrefixRecords`, `ParseMinimalPrefixRecord`, `CollectPrefixRecords` |
| Package metadata | `ParseArchiveFilename`, `ParseIndexJSON` |
| Channels and constraints | `CanonicalChannel`, `ParseMatchSpec`, `MatchSpecMatches`, `VersionSpecContains`, `CompareVersions` |
| Environment specifications | `ParseEnvironmentYAML`, `ParseExplicitSpec` |
| Platform mapping | `SubdirFromOCIPlatform` |
| Lock inspection | `LockPackages`, `LockSync`, `CompareLock`, `LockEnvironment` |

Batch record parsing returns valid records and indexed errors together. Full prefix parsing uses rattler's schema; the minimal API projects a full parse and does not promise a faster sparse parser. `CollectPrefixRecords` is a host-filesystem convenience; byte-taking parsers also work with files read from container images. Go types expose the fields needed for inspection, rather than reproducing every future Rust field.

`CollectPrefixRecords` reads regular `conda-meta/*.json` files in sorted order, skips symlink/non-regular entries, and reports read/parse failures alongside successful records. It is not a filesystem sandbox. Its `SourceIndex` and `SourcePath` identify processed inputs; callers needing complete discovery coverage should account for excluded entries themselves.

Channel normalization uses the default conda channel alias, returns a credential-redacted canonical URL, and does not expand the `defaults` multichannel or resolve local paths. Package-record MatchSpec matching rejects constraints it cannot check, rather than silently ignoring them. Parsing an environment manifest is not lock verification. OCI-to-conda subdir mapping is an explicit adapter and rejects unsupported platforms; it never substitutes the host platform.

Lock comparison currently reads Pixi lockfiles through `rattler_lock`; conda-lock YAML and explicit-spec documents are not accepted as interchangeable lock formats. `LockSync` explicitly compares the conda scope. `CompareLock` compares both conda and PyPI, requiring supplied complete observations of both scopes, even when the lock contains no PyPI packages.

Comparison checks **installed metadata**, not installed file contents or downloaded archive hashes. Results distinguish missing, extra, changed, and unknown packages. Callers must accurately declare whether their observations are complete, and PyPI observations must exclude distributions already owned by conda. Missing expected hashes, editable/source packages, incomplete observations, and ambiguous identities cannot produce a verified match. URLs in lock reports are redacted; normalized package identity and available hashes are compared, not raw mirror URLs. It does not implement Pixi's installed-lock hash or full manifest/lock synchronization. `LockEnvironment` remains a count-only convenience over lock enumeration.

## How it works

The public Go package calls `internal/api`, which passes length-delimited JSON through cgo to one Rust `staticlib`. Domain modules handle records, constraints/specifications, and lock comparison. One C entry point returns an owned byte buffer, and one cleanup entry point releases it. The Go adapter copies the response and frees Rust-owned memory on both success and error paths. Rust panics are contained before crossing the ABI. Callers never manage native pointers.

The ABI uses synchronous calls and has no context-cancellation hook. Callers control reading the input documents and should batch small records where practical. Errors support `errors.Is(err, gorattler.ErrInvalidInput)` and `errors.Is(err, gorattler.ErrNative)`; `errors.As` exposes the native `*gorattler.Error`. Successful batch parsing may still contain per-record errors, which must be checked before declaring complete evidence.

We use plain `cargo build` because Go currently needs one static archive, not an installable C SDK. The small C header is maintained alongside the Rust buffer ABI and compiled by cgo in the tests. New operations share this ABI rather than adding another native library or an exported C function per operation.

## Build from source

Requires Go 1.26+, Rust/Cargo, cgo, and a C compiler. `go build` does **not** run Cargo. From a consuming Go module, build the exact Go dependency's Rust source first, then link its archive into your final executable:

```sh
module=github.com/conda-incubator/go-rattler
go get "$module@<version-or-commit>" # omit if already pinned in go.mod
go mod download "$module"
source_dir="$(go list -m -f '{{.Dir}}' "$module")"
native_dir="$PWD/.go-rattler-native" # writable; git-ignore this directory
CARGO_TARGET_DIR="$native_dir" cargo build --manifest-path "$source_dir/internal/native/Cargo.toml" --release --locked
CGO_ENABLED=1 CGO_LDFLAGS="-L \"$native_dir/release\"" go build -o my-program ./cmd/my-program
```

The Go module download supplies the Rust source, but `go build` does not compile it: run Cargo first, in a writable target directory outside the downloaded module. cgo then links `libgo_rattler_native.a` into the executable; no separate Rust library is needed at runtime, although OS libraries may still be dynamic. On Windows amd64, use MinGW-w64 for cgo, install the `x86_64-pc-windows-gnu` Rust target, set `CARGO_BUILD_TARGET=x86_64-pc-windows-gnu`, and point `CGO_LDFLAGS` at `$native_dir/x86_64-pc-windows-gnu/release`. The wrapper includes the required Windows system import libraries. Bare `go get` followed by `go build` is not enough; native archives are not distributed.

Locally, `make check` builds the archive and runs Rust tests, Go race tests, a separate consuming Go module, vet, formatting checks, Clippy, and golangci-lint. CI runs the native tests/consumer on Linux, macOS, and Windows GNU; Go lint has its own job. For stricter cgo pointer checks, run `GOEXPERIMENT=cgocheck2 make test`.

> [!NOTE]
> Graduation of this project means upstreaming into [rattler](https://github.com/conda/rattler).

Licensed under BSD-3-Clause, matching rattler.
