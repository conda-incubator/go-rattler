//! Typed conda semantics operations: channel, matchspec, versionspec,
//! environment YAML, explicit specs, OCI subdir mapping, and version comparison.
//!
//! Every operation takes a JSON request and returns a JSON result. Document
//! fields are standard base64 and are decoded with `crate::bridge::decode_document`.

use std::cmp::Ordering;
use std::str::FromStr;

use rattler_conda_types::{
    Channel, ChannelConfig, EnvironmentYaml, EqualityOperator, ExplicitEnvironmentSpec, MatchSpec,
    MatchSpecOrSubSection, PackageArchiveHash, PackageRecord, ParseMatchSpecOptions,
    ParseStrictness, Subdir, Version, VersionSpec, match_spec::Matches,
};
use serde::Deserialize;
use serde_json::{Value, json};
use std::collections::BTreeMap;
use std::path::PathBuf;

/// Operation codes, matching the numeric IDs used by the Go side.
const CHANNEL: u32 = 6;
const MATCHSPEC: u32 = 7;
const VERSIONSPEC: u32 = 8;
const ENVIRONMENT: u32 = 9;
const EXPLICIT: u32 = 10;
const SUBDIR: u32 = 11;
const COMPARE_VERSIONS: u32 = 15;

/// Dispatches a semantics operation. The bridge routes codes 6..=11 here and
/// the Go side routes version comparison through operation 15.
pub(crate) fn dispatch(operation: u32, input: Value) -> Result<Value, String> {
    match operation {
        CHANNEL => channel(input),
        MATCHSPEC => matchspec(input),
        VERSIONSPEC => versionspec(input),
        ENVIRONMENT => environment(input),
        EXPLICIT => explicit(input),
        SUBDIR => subdir(input),
        COMPARE_VERSIONS => compare_versions(input),
        other => Err(format!("unsupported semantics operation {other}")),
    }
}

fn decode<T: for<'de> Deserialize<'de>>(input: Value) -> Result<T, String> {
    serde_json::from_value(input).map_err(|err| format!("invalid request: {err}"))
}

// ---------------------------------------------------------------------------
// Channel (6)
// ---------------------------------------------------------------------------

#[derive(Deserialize)]
struct ChannelRequest {
    value: String,
}

/// Canonicalises a channel name or absolute URL.
///
/// Delegates to `Channel::canonical_name`, which redacts credentials and
/// token-like path segments. Relative paths, local paths, and the `defaults`
/// multichannel are rejected. No filesystem access is performed.
pub(crate) fn canonical_channel(value: &str) -> Result<String, String> {
    if value.trim().is_empty() {
        return Err("channel must not be empty".into());
    }
    if value == "defaults" {
        return Err("the multichannel name \"defaults\" is not supported".into());
    }
    if value.contains("://") && !value.starts_with("https://") && !value.starts_with("http://") {
        return Err("only https and http channel URLs are supported".into());
    }
    if !value.contains("://") && !is_plain_channel_name(value) {
        return Err("only named or absolute https channels are supported".into());
    }
    // The root directory is never used: relative and file inputs are rejected above.
    let config = ChannelConfig::default_with_root_dir(PathBuf::from("/"));
    let channel = Channel::from_str(value, &config).map_err(|_| "invalid channel".to_string())?;
    // Remove userinfo entirely (username and password) before serialising, so
    // neither can reach output. Token-like path segments are redacted by
    // `canonical_name`, which runs on the credential-free URL.
    let mut base = channel.base_url.url().as_ref().clone();
    base.set_username("")
        .map_err(|_| "invalid channel".to_string())?;
    base.set_password(None)
        .map_err(|_| "invalid channel".to_string())?;
    let mut stripped = channel.clone();
    stripped.base_url = rattler_conda_types::ChannelUrl::from(base);
    Ok(stripped.canonical_name())
}

/// Accepts a conda channel name: ASCII letters, digits, `-`, `_`, `.`.
fn is_plain_channel_name(value: &str) -> bool {
    !value.is_empty()
        && value
            .chars()
            .all(|c| c.is_ascii_alphanumeric() || matches!(c, '-' | '_' | '.'))
}

fn channel(input: Value) -> Result<Value, String> {
    let req: ChannelRequest = decode(input)?;
    Ok(json!({ "canonical": canonical_channel(&req.value)? }))
}

// ---------------------------------------------------------------------------
// MatchSpec (7)
// ---------------------------------------------------------------------------

#[derive(Deserialize)]
#[serde(tag = "mode", rename_all = "lowercase")]
enum MatchSpecRequest {
    Parse {
        spec: String,
    },
    Matches {
        spec: String,
        record: Box<PackageRecord>,
    },
}

/// Parses a MatchSpec with strict options. Extras, conditionals, and flags are
/// rejected by these options; non-exact name matchers are rejected too.
fn parse_spec(spec: &str) -> Result<MatchSpec, String> {
    MatchSpec::from_str(spec, ParseMatchSpecOptions::new(ParseStrictness::Strict))
        .map_err(|err| format!("invalid matchspec: {err}"))
}

/// Constraints that `Matches<PackageRecord>` does not verify. Matching them
/// against a bare record would silently return a wrong answer, so they are rejected.
fn unsupported_constraint(spec: &MatchSpec) -> Option<&'static str> {
    if spec.channel.is_some() {
        Some("channel")
    } else if spec.subdir.is_some() {
        Some("subdir")
    } else if spec.url.is_some() {
        Some("url")
    } else if spec.namespace.is_some() {
        Some("namespace")
    } else if spec.file_name.is_some() {
        Some("filename")
    } else if spec.condition.is_some() {
        Some("when")
    } else if spec.extras.is_some() {
        Some("extras")
    } else {
        None
    }
}

/// Whether a MatchSpec pins one exact package: an exact version (`==` or a bare
/// version, not a range or prefix) together with an exact build string.
fn is_pinned(spec: &MatchSpec) -> bool {
    let exact_version = matches!(
        spec.version,
        Some(VersionSpec::Exact(EqualityOperator::Equals, _))
    );
    let exact_build = matches!(
        spec.build,
        Some(rattler_conda_types::StringMatcher::Exact(_))
    );
    exact_version && exact_build
}

fn matchspec(input: Value) -> Result<Value, String> {
    match decode::<MatchSpecRequest>(input)? {
        MatchSpecRequest::Parse { spec } => {
            let parsed = parse_spec(&spec)?;
            Ok(json!({
                "canonical": parsed.to_string(),
                "name": parsed.name.to_string(),
                "version": parsed.version.as_ref().map(ToString::to_string),
                "build": parsed.build.as_ref().map(ToString::to_string),
                "pinned": is_pinned(&parsed),
            }))
        }
        MatchSpecRequest::Matches { spec, record } => {
            let parsed = parse_spec(&spec)?;
            if let Some(field) = unsupported_constraint(&parsed) {
                return Err(format!(
                    "unsupported matchspec constraint {field:?}: PackageRecord matching cannot verify it"
                ));
            }
            Ok(json!({ "matches": parsed.matches(record.as_ref()) }))
        }
    }
}

// ---------------------------------------------------------------------------
// VersionSpec (8)
// ---------------------------------------------------------------------------

#[derive(Deserialize)]
struct VersionSpecRequest {
    spec: String,
    version: String,
}

/// Checks a version against a strict conda version spec. An empty spec is
/// rejected rather than interpreted as "any".
fn versionspec(input: Value) -> Result<Value, String> {
    let req: VersionSpecRequest = decode(input)?;
    if req.spec.trim().is_empty() {
        return Err("version spec must not be empty".into());
    }
    let spec = VersionSpec::from_str(&req.spec, ParseStrictness::Strict)
        .map_err(|err| format!("invalid version spec: {err}"))?;
    let version =
        Version::from_str(&req.version).map_err(|err| format!("invalid version: {err}"))?;
    Ok(json!({ "contains": spec.matches(&version) }))
}

// ---------------------------------------------------------------------------
// Environment YAML (9)
// ---------------------------------------------------------------------------

#[derive(Deserialize)]
struct EnvironmentRequest {
    document: String,
}

fn environment(input: Value) -> Result<Value, String> {
    let req: EnvironmentRequest = decode(input)?;
    let bytes = crate::bridge::decode_document(&req.document)?;
    let text = std::str::from_utf8(&bytes)
        .map_err(|_| "environment YAML is not valid UTF-8".to_string())?;
    let parsed = EnvironmentYaml::from_yaml_str(text)
        .map_err(|err| format!("invalid environment YAML: {err}"))?;

    let mut dependencies = Vec::new();
    let mut subsections: BTreeMap<String, Vec<String>> = BTreeMap::new();
    for dep in &parsed.dependencies {
        match dep {
            MatchSpecOrSubSection::MatchSpec(spec) => dependencies.push(spec.to_string()),
            // Duplicate subsections (for example several `pip:` blocks) are
            // concatenated in document order rather than dropped.
            MatchSpecOrSubSection::SubSection(name, items) => {
                subsections
                    .entry(name.clone())
                    .or_default()
                    .extend(items.iter().cloned());
            }
        }
    }

    Ok(json!({
        "name": parsed.name,
        "prefix": parsed.prefix.as_ref().map(|p| p.display().to_string()),
        "channels": parsed.channels.iter().map(|c| c.as_str().to_string()).collect::<Vec<_>>(),
        "dependencies": dependencies,
        "subsections": subsections,
        "variables": parsed.variables,
    }))
}

// ---------------------------------------------------------------------------
// Explicit spec (10)
// ---------------------------------------------------------------------------

#[derive(Deserialize)]
struct ExplicitRequest {
    document: String,
}

fn explicit(input: Value) -> Result<Value, String> {
    let req: ExplicitRequest = decode(input)?;
    let bytes = crate::bridge::decode_document(&req.document)?;
    let spec = ExplicitEnvironmentSpec::from_reader(bytes.as_slice())
        .map_err(|err| format!("invalid explicit spec: {err}"))?;

    let mut packages = Vec::with_capacity(spec.packages.len());
    for entry in &spec.packages {
        let hash = entry
            .package_archive_hash()
            .map_err(|err| format!("invalid package hash: {err}"))?;
        let (md5, sha256) = match hash {
            Some(PackageArchiveHash::Md5(digest)) => (Some(to_hex(digest.as_slice())), None),
            Some(PackageArchiveHash::Sha256(digest)) => (None, Some(to_hex(digest.as_slice()))),
            None => (None, None),
        };
        // Strip the fragment so the hash is reported only in its own field.
        let mut url = entry.url.clone();
        url.set_fragment(None);
        packages.push(json!({ "url": url.to_string(), "md5": md5, "sha256": sha256 }));
    }

    Ok(json!({
        "platform": spec.platform.as_ref().map(ToString::to_string),
        "packages": packages,
    }))
}

// ---------------------------------------------------------------------------
// OCI platform -> subdir (11)
// ---------------------------------------------------------------------------

#[derive(Deserialize)]
struct SubdirRequest {
    os: String,
    arch: String,
    #[serde(default)]
    variant: String,
}

/// Closed mapping from an OCI platform to a conda subdir. Unlisted combinations
/// are rejected; there is no string-based fallback.
fn subdir_from_oci(os: &str, arch: &str, variant: &str) -> Result<&'static str, String> {
    let subdir = match (os, arch, variant) {
        ("linux", "amd64", "") => "linux-64",
        ("linux", "arm64", "" | "v8") => "linux-aarch64",
        ("linux", "386", "") => "linux-32",
        ("linux", "ppc64le", "") => "linux-ppc64le",
        ("linux", "s390x", "") => "linux-s390x",
        ("linux", "riscv64", "") => "linux-riscv64",
        ("linux", "arm", "v6") => "linux-armv6l",
        ("linux", "arm", "v7") => "linux-armv7l",
        ("darwin", "amd64", "") => "osx-64",
        ("darwin", "arm64", "" | "v8") => "osx-arm64",
        ("windows", "amd64", "") => "win-64",
        ("windows", "386", "") => "win-32",
        _ => {
            return Err(format!(
                "unsupported OCI platform os={os:?} arch={arch:?} variant={variant:?}"
            ));
        }
    };
    // The table is closed and must only name subdirs rattler recognises.
    Subdir::from_str(subdir)
        .map(|_| subdir)
        .map_err(|_| format!("internal: mapping produced unknown subdir {subdir:?}"))
}

fn to_hex(bytes: &[u8]) -> String {
    bytes.iter().map(|b| format!("{b:02x}")).collect()
}

fn subdir(input: Value) -> Result<Value, String> {
    let req: SubdirRequest = decode(input)?;
    Ok(json!({ "subdir": subdir_from_oci(&req.os, &req.arch, &req.variant)? }))
}

// ---------------------------------------------------------------------------
// Compare versions (15)
// ---------------------------------------------------------------------------

#[derive(Deserialize)]
struct CompareRequest {
    a: String,
    b: String,
}

/// Compares two conda versions and returns -1, 0, or 1.
fn compare_versions(input: Value) -> Result<Value, String> {
    let req: CompareRequest = decode(input)?;
    let first = Version::from_str(&req.a)
        .map_err(|err| format!("invalid first conda version {:?}: {err}", req.a))?;
    let second = Version::from_str(&req.b)
        .map_err(|err| format!("invalid second conda version {:?}: {err}", req.b))?;
    let value: i64 = match first.cmp(&second) {
        Ordering::Less => -1,
        Ordering::Equal => 0,
        Ordering::Greater => 1,
    };
    Ok(json!(value))
}

#[cfg(test)]
mod tests {
    use super::*;

    fn record(name: &str, version: &str, build: &str, build_number: u64) -> Value {
        json!({"name": name, "version": version, "build": build, "build_number": build_number})
    }

    #[test]
    fn channel_name_and_url_canonicalise_identically() {
        assert_eq!(
            canonical_channel("conda-forge").unwrap(),
            canonical_channel("https://conda.anaconda.org/conda-forge/").unwrap()
        );
    }

    #[test]
    fn channel_redacts_credentials_and_token_path() {
        let out =
            canonical_channel("https://user:hunter2@repo.example.com/t/SECRETTOKEN/chan").unwrap();
        assert!(!out.contains("hunter2"), "password leaked: {out}");
        assert!(!out.contains("user"), "username leaked: {out}");
        assert!(!out.contains("SECRETTOKEN"), "token leaked: {out}");
    }

    #[test]
    fn channel_rejects_defaults_paths_and_schemes() {
        assert!(canonical_channel("defaults").is_err());
        assert!(canonical_channel("./local").is_err());
        assert!(canonical_channel("/abs/path").is_err());
        assert!(canonical_channel("file:///tmp/chan").is_err());
        assert!(canonical_channel("").is_err());
    }

    #[test]
    fn matchspec_parse_reports_pinned_only_for_exact_version_and_build() {
        let pinned = matchspec(json!({"mode": "parse", "spec": "numpy==1.2 py311_0"})).unwrap();
        assert_eq!(pinned["pinned"], true);
        let version_only = matchspec(json!({"mode": "parse", "spec": "numpy==1.2"})).unwrap();
        assert_eq!(version_only["pinned"], false);
        let range = matchspec(json!({"mode": "parse", "spec": "numpy >=1.2 py311_0"})).unwrap();
        assert_eq!(range["pinned"], false);
        let exclusion = matchspec(json!({"mode": "parse", "spec": "numpy!=1.2 py311_0"})).unwrap();
        assert_eq!(exclusion["pinned"], false);
    }

    #[test]
    fn matchspec_rejects_malformed_and_duplicate_fields() {
        assert!(matchspec(json!({"mode": "parse", "spec": "numpy ["})).is_err());
        assert!(
            matchspec(json!({"mode": "parse", "spec": "numpy 1.0 [build=a, build=b]"})).is_err()
        );
    }

    #[test]
    fn matchspec_matches_build_number_and_version() {
        let rec = record("numpy", "1.2", "py_0", 3);
        let ok = matchspec(
            json!({"mode": "matches", "spec": "numpy >=1.0[build_number=\">=3\"]", "record": rec.clone()}),
        );
        assert_eq!(ok.unwrap()["matches"], true);
        let build_bad = matchspec(
            json!({"mode": "matches", "spec": "numpy >=1.0[build_number=\">=4\"]", "record": rec.clone()}),
        );
        assert_eq!(build_bad.unwrap()["matches"], false);
        let version_bad =
            matchspec(json!({"mode": "matches", "spec": "numpy <1.0", "record": rec}));
        assert_eq!(version_bad.unwrap()["matches"], false);
    }

    #[test]
    fn matchspec_rejects_unverifiable_constraints_instead_of_matching() {
        let rec = record("numpy", "1.2", "py_0", 0);
        for spec in [
            "conda-forge::numpy",
            "numpy[subdir=linux-64]",
            "numpy[channel=conda-forge]",
        ] {
            let err = matchspec(json!({"mode": "matches", "spec": spec, "record": rec.clone()}))
                .unwrap_err();
            assert!(
                err.contains("unsupported matchspec constraint"),
                "{spec}: {err}"
            );
        }
    }

    #[test]
    fn versionspec_contains_and_rejects_invalid() {
        assert_eq!(
            versionspec(json!({"spec": ">=1.2", "version": "1.3"})).unwrap()["contains"],
            true
        );
        assert_eq!(
            versionspec(json!({"spec": "1.2.*", "version": "1.3"})).unwrap()["contains"],
            false
        );
        assert!(versionspec(json!({"spec": "", "version": "1"})).is_err());
        assert!(versionspec(json!({"spec": "!!bad", "version": "1"})).is_err());
        assert!(versionspec(json!({"spec": "1.0", "version": "bad version!"})).is_err());
    }

    #[test]
    fn compare_versions_orders_and_names_failing_argument() {
        assert_eq!(
            compare_versions(json!({"a": "1.0rc1", "b": "1.0"})).unwrap(),
            json!(-1)
        );
        assert_eq!(
            compare_versions(json!({"a": "2.0", "b": "1.0"})).unwrap(),
            json!(1)
        );
        assert_eq!(
            compare_versions(json!({"a": "1!1.0", "b": "1.99"})).unwrap(),
            json!(1)
        );
        assert!(
            compare_versions(json!({"a": "invalid version!", "b": "1"}))
                .unwrap_err()
                .contains("first")
        );
        assert!(
            compare_versions(json!({"a": "1", "b": ""}))
                .unwrap_err()
                .contains("second")
        );
    }

    #[test]
    fn oci_mapping_is_closed() {
        assert_eq!(subdir_from_oci("linux", "amd64", "").unwrap(), "linux-64");
        assert_eq!(
            subdir_from_oci("linux", "arm64", "").unwrap(),
            "linux-aarch64"
        );
        assert_eq!(
            subdir_from_oci("linux", "arm64", "v8").unwrap(),
            "linux-aarch64"
        );
        assert_eq!(subdir_from_oci("darwin", "arm64", "").unwrap(), "osx-arm64");
        assert_eq!(subdir_from_oci("windows", "amd64", "").unwrap(), "win-64");
        assert!(subdir_from_oci("linux", "arm64", "v9").is_err());
        assert!(subdir_from_oci("linux", "amd64", "v8").is_err());
        assert!(subdir_from_oci("linux", "arm", "").is_err());
        assert!(subdir_from_oci("", "amd64", "").is_err());
    }

    #[test]
    fn explicit_strips_fragment_into_hash_field_and_keeps_order() {
        let doc = b"# platform: linux-64\n@EXPLICIT\nhttps://h/a-1.tar.bz2#0123456789abcdef0123456789abcdef\nhttps://h/b-1.conda\n";
        let out = explicit(json!({"document": encode(doc)})).unwrap();
        assert_eq!(out["platform"], "linux-64");
        assert_eq!(out["packages"][0]["url"], "https://h/a-1.tar.bz2");
        assert_eq!(
            out["packages"][0]["md5"],
            "0123456789abcdef0123456789abcdef"
        );
        assert_eq!(out["packages"][1]["url"], "https://h/b-1.conda");
    }

    #[test]
    fn explicit_rejects_missing_tag() {
        assert!(explicit(json!({"document": encode(b"https://h/a.conda\n")})).is_err());
    }

    #[test]
    fn environment_preserves_all_pip_blocks_and_variables() {
        let doc = b"name: demo\nchannels: [conda-forge]\ndependencies:\n  - numpy\n  - pip:\n      - a\n  - pip:\n      - b\nvariables:\n  KEY: value\n";
        let out = environment(json!({"document": encode(doc)})).unwrap();
        assert_eq!(out["name"], "demo");
        assert_eq!(out["subsections"]["pip"], json!(["a", "b"]));
        assert_eq!(out["variables"]["KEY"], "value");
    }

    #[test]
    fn environment_rejects_invalid_utf8() {
        assert!(environment(json!({"document": encode(&[0xff, 0xfe])})).is_err());
    }

    fn encode(bytes: &[u8]) -> String {
        use base64::Engine;
        base64::engine::general_purpose::STANDARD.encode(bytes)
    }
}
