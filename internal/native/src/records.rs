//! Conda record parsing operations backed by canonical rattler types.
//!
//! Operations: 1 parse one prefix record, 2 parse a batch of prefix records,
//! 3 minimal prefix projection, 4 archive filename, 5 index.json, 14 collect
//! conda-meta records from a prefix directory. Operation numbers are routed by
//! `crate::bridge::dispatch`.

use std::fs;
use std::path::Path;
use std::str::FromStr;

use rattler_conda_types::package::{
    BuildString, CondaArchiveIdentifier, CondaArchiveType, IndexJson,
};
use rattler_conda_types::{PackageName, PrefixRecord, VersionWithSource};
use serde::Deserialize;
use serde_json::{Value, json};

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct DocumentRequest {
    document: String,
}

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct BatchRequest {
    documents: Vec<String>,
}

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct FilenameRequest {
    filename: String,
}

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct PrefixRequest {
    prefix: String,
}

/// Dispatches a records operation. `input` is the operation request JSON.
pub fn dispatch(operation: u32, input: Value) -> Result<Value, String> {
    match operation {
        1 => {
            let req: DocumentRequest = decode_request(input)?;
            let bytes = crate::bridge::decode_document(&req.document)?;
            parse_prefix_value(&bytes)
        }
        2 => {
            let req: BatchRequest = decode_request(input)?;
            batch_prefix(&req.documents)
        }
        3 => {
            let req: DocumentRequest = decode_request(input)?;
            let bytes = crate::bridge::decode_document(&req.document)?;
            minimal_prefix_value(&bytes)
        }
        4 => {
            let req: FilenameRequest = decode_request(input)?;
            archive_filename_value(&req.filename)
        }
        5 => {
            let req: DocumentRequest = decode_request(input)?;
            let bytes = crate::bridge::decode_document(&req.document)?;
            index_json_value(&bytes)
        }
        14 => {
            let req: PrefixRequest = decode_request(input)?;
            collect_prefix_value(&req.prefix)
        }
        other => Err(format!("unsupported records operation {other}")),
    }
}

fn decode_request<T: for<'de> Deserialize<'de>>(input: Value) -> Result<T, String> {
    serde_json::from_value(input).map_err(|err| format!("invalid request: {err}"))
}

/// Parses one full conda-meta document with the canonical serde model and
/// returns the record as JSON. Rattler's serde model validates known fields;
/// unknown fields are ignored according to that model's forward-compatible policy.
fn parse_prefix_value(bytes: &[u8]) -> Result<Value, String> {
    let record: PrefixRecord = serde_json::from_slice(bytes).map_err(|err| err.to_string())?;
    let package = &record.repodata_record.package_record;
    validate_package_record(&package.name, &package.version, package.build.as_str())?;
    serde_json::to_value(&record).map_err(|err| err.to_string())
}

/// Projects the canonical full record to the minimal fields. A full canonical
/// parse is performed first; this is a projection, not a sparse parser.
fn minimal_prefix_value(bytes: &[u8]) -> Result<Value, String> {
    let record: PrefixRecord = serde_json::from_slice(bytes).map_err(|err| err.to_string())?;
    let package = &record.repodata_record.package_record;
    validate_package_record(&package.name, &package.version, package.build.as_str())?;
    Ok(json!({
        "name": package.name.as_normalized(),
        "version": package.version.as_str().into_owned(),
        "build": package.build,
        "sha256": package.sha256.map(|h| hex_string(h.as_slice())),
        "md5": package.md5.map(|h| hex_string(h.as_slice())),
        "size": package.size,
    }))
}

fn hex_string(bytes: &[u8]) -> String {
    bytes.iter().map(|b| format!("{b:02x}")).collect()
}

/// Validates that name, version and build are canonical. Serde keeps
/// `PackageName`, `VersionWithSource` and `BuildString` loose in some paths,
/// so emptiness and character rules are enforced explicitly here.
fn validate_package_record(
    name: &PackageName,
    version: &VersionWithSource,
    build: &str,
) -> Result<(), String> {
    if name.as_normalized().is_empty() {
        return Err("package name must not be empty".into());
    }
    PackageName::try_from(name.as_source().to_owned())
        .map_err(|err| format!("invalid package name: {err}"))?;
    if version.as_str().is_empty() {
        return Err("package version must not be empty".into());
    }
    VersionWithSource::from_str(&version.as_str())
        .map_err(|err| format!("invalid package version: {err}"))?;
    if build.is_empty() {
        return Err("package build must not be empty".into());
    }
    BuildString::from_str(build).map_err(|err| format!("invalid build string: {err}"))?;
    Ok(())
}

/// Batch parsing. Each document is decoded and parsed independently; a failure
/// is reported as an indexed error and never fails the batch. Go encodes nil
/// documents as empty bytes, producing an error for that index alone.
fn batch_prefix(documents: &[String]) -> Result<Value, String> {
    let mut records = Vec::new();
    let mut errors = Vec::new();
    for (index, document) in documents.iter().enumerate() {
        let result =
            crate::bridge::decode_document(document).and_then(|bytes| parse_prefix_value(&bytes));
        match result {
            Ok(mut value) => {
                if let Value::Object(map) = &mut value {
                    map.insert("source_index".into(), json!(index));
                }
                records.push(value);
            }
            Err(message) => errors.push(json!({ "index": index, "message": message })),
        }
    }
    Ok(json!({ "records": records, "errors": errors }))
}

/// Parses a bare conda archive filename with canonical validation.
fn archive_filename_value(filename: &str) -> Result<Value, String> {
    if filename.is_empty() {
        return Err("archive filename must not be empty".into());
    }
    if filename.contains('/') || filename.contains('\\') {
        return Err("archive filename must not contain path separators".into());
    }
    let id = CondaArchiveIdentifier::try_from_filename(filename)
        .ok_or_else(|| "invalid conda archive filename".to_string())?;
    let archive_type = match id.archive_type {
        CondaArchiveType::Conda => "conda",
        CondaArchiveType::TarBz2 => "tar.bz2",
    };
    validate_package_record(
        &PackageName::try_from(id.identifier.name.clone())
            .map_err(|err| format!("invalid package name: {err}"))?,
        &VersionWithSource::from_str(&id.identifier.version)
            .map_err(|err| format!("invalid package version: {err}"))?,
        &id.identifier.build_string,
    )?;
    Ok(json!({
        "name": id.identifier.name,
        "version": id.identifier.version,
        "build": id.identifier.build_string,
        "type": archive_type,
        "filename": id.to_file_name(),
    }))
}

/// Parses info/index.json with the canonical serde model and validates it.
fn index_json_value(bytes: &[u8]) -> Result<Value, String> {
    let index: IndexJson = serde_json::from_slice(bytes).map_err(|err| err.to_string())?;
    validate_package_record(&index.name, &index.version, index.build.as_ref())?;
    serde_json::to_value(&index).map_err(|err| err.to_string())
}

/// Collects conda-meta/*.json records. Symlinks and non-regular entries are
/// skipped. Directory-read failures are returned as errors, not swallowed.
/// Each file's read/parse failure is reported with its sorted index and path.
fn collect_prefix_value(prefix: &str) -> Result<Value, String> {
    if prefix.is_empty() {
        return Err("prefix must not be empty".into());
    }
    let meta_dir = Path::new(prefix).join("conda-meta");
    let meta = fs::symlink_metadata(&meta_dir).map_err(|err| err.to_string())?;
    if meta.file_type().is_symlink() || !meta.is_dir() {
        return Err("conda-meta must be a real directory".into());
    }

    let mut entries = Vec::new();
    for entry in fs::read_dir(&meta_dir).map_err(|err| err.to_string())? {
        let entry = entry.map_err(|err| err.to_string())?;
        let path = entry.path();
        if path.extension().is_some_and(|ext| ext == "json") {
            entries.push(path);
        }
    }
    entries.sort();

    let mut records = Vec::new();
    let mut errors = Vec::new();
    let mut index = 0usize;
    for path in entries {
        let file_meta = match fs::symlink_metadata(&path) {
            Ok(meta) => meta,
            Err(err) => {
                errors.push(json!({ "index": index, "path": path.display().to_string(), "message": err.to_string() }));
                index += 1;
                continue;
            }
        };
        if file_meta.file_type().is_symlink() || !file_meta.is_file() {
            continue;
        }
        let result = fs::read(&path)
            .map_err(|err| err.to_string())
            .and_then(|bytes| parse_prefix_value(&bytes));
        match result {
            Ok(mut value) => {
                if let Value::Object(map) = &mut value {
                    map.insert("source_index".into(), json!(index));
                    map.insert("source_path".into(), json!(path.display().to_string()));
                }
                records.push(value);
            }
            Err(message) => errors.push(json!({
                "index": index,
                "path": path.display().to_string(),
                "message": message,
            })),
        }
        index += 1;
    }
    Ok(json!({ "records": records, "errors": errors }))
}

#[cfg(test)]
mod fixture_tests {
    use std::fs;
    use std::path::PathBuf;

    use serde_json::json;

    use crate::bridge::decode_document;
    use crate::records::dispatch;

    fn fixture(path: &str) -> Vec<u8> {
        let root =
            PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../../testdata/records/fixtures");
        fs::read(root.join(path)).unwrap()
    }

    fn b64(bytes: &[u8]) -> String {
        use base64::Engine;
        base64::engine::general_purpose::STANDARD.encode(bytes)
    }

    #[test]
    fn parses_full_prefix_record_fields() {
        let doc = fixture("conda-meta/envscout-demo-1.2.3-py311_0.json");
        let value = dispatch(1, json!({ "document": b64(&doc) })).unwrap();
        assert_eq!(value["name"], "envscout-demo");
        assert_eq!(value["version"], "1.2.3");
        assert_eq!(value["build"], "py311_0");
        assert_eq!(value["fn"], "envscout-demo-1.2.3-py311_0.conda");
        assert_eq!(value["requested_specs"][0], "envscout-demo ==1.2.3");
        assert_eq!(value["files"][0], "bin/envscout-demo");
        assert_eq!(
            value["paths_data"]["paths"][0]["_path"],
            "bin/envscout-demo"
        );
        assert_eq!(value["paths_data"]["paths"][0]["size_in_bytes"], 1024);
        assert_eq!(value["link"]["type"], 1);
    }

    #[test]
    fn rejects_invalid_utf8_and_trailing_json() {
        assert!(dispatch(1, json!({ "document": b64(&[0xff, 0xfe]) })).is_err());
        let trailing = format!(
            "{} {{}}",
            String::from_utf8(fixture("index/index.json")).unwrap()
        );
        assert!(dispatch(1, json!({ "document": b64(trailing.as_bytes()) })).is_err());
    }

    #[test]
    fn index_json_parses_and_validates_name() {
        let doc = fixture("index/index.json");
        let value = dispatch(5, json!({ "document": b64(&doc) })).unwrap();
        assert_eq!(value["name"], "envscout-demo");
        assert_eq!(value["build_number"], 0);

        let empty_name = br#"{"name":"","version":"1.0","build":"0","build_number":0}"#;
        assert!(dispatch(5, json!({ "document": b64(empty_name) })).is_err());
    }

    #[test]
    fn minimal_projection_matches_full_parse() {
        let doc = fixture("conda-meta/envscout-demo-1.2.3-py311_0.json");
        let minimal = dispatch(3, json!({ "document": b64(&doc) })).unwrap();
        let full = dispatch(1, json!({ "document": b64(&doc) })).unwrap();
        assert_eq!(minimal["name"], full["name"]);
        assert_eq!(minimal["version"], full["version"]);
        assert_eq!(minimal["build"], full["build"]);
        assert_eq!(minimal["size"], full["size"]);
        assert_eq!(minimal["sha256"], full["sha256"]);
        assert!(minimal.get("files").is_none());
        assert!(minimal.get("paths_data").is_none());
    }

    #[test]
    fn archive_filename_rules() {
        let ok = dispatch(4, json!({ "filename": "my-pkg-1.0-py_0.conda" })).unwrap();
        assert_eq!(ok["type"], "conda");
        assert_eq!(ok["name"], "my-pkg");
        let tar = dispatch(4, json!({ "filename": "foo-2.0-0.tar.bz2" })).unwrap();
        assert_eq!(tar["type"], "tar.bz2");
        for bad in [
            "",
            "dir/foo-1-0.conda",
            "foo-1-0.zip",
            "foo-1.conda",
            "foo-1-0.tar",
        ] {
            assert!(dispatch(4, json!({ "filename": bad })).is_err(), "{bad}");
        }
    }

    #[test]
    fn collect_reports_bad_files_and_skips_symlinks() {
        let dir = tempfile::tempdir().unwrap();
        let meta = dir.path().join("conda-meta");
        fs::create_dir(&meta).unwrap();
        fs::write(
            meta.join("envscout-demo-1.2.3-py311_0.json"),
            fixture("conda-meta/envscout-demo-1.2.3-py311_0.json"),
        )
        .unwrap();
        fs::write(
            meta.join("bad-1.0-0.json"),
            fixture("conda-meta/bad-1.0-0.json"),
        )
        .unwrap();
        #[cfg(unix)]
        std::os::unix::fs::symlink(
            meta.join("envscout-demo-1.2.3-py311_0.json"),
            meta.join("link-1.0-0.json"),
        )
        .unwrap();

        let prefix = dir.path().to_str().unwrap();
        let value = dispatch(14, json!({ "prefix": prefix })).unwrap();
        let records = value["records"].as_array().unwrap();
        let errors = value["errors"].as_array().unwrap();
        assert_eq!(records.len(), 1);
        assert_eq!(records[0]["name"], "envscout-demo");
        assert_eq!(errors.len(), 1);
        // Files are sorted by path; "bad-1.0-0.json" sorts before "envscout-...".
        assert_eq!(errors[0]["index"], 0);
        assert!(
            errors[0]["path"]
                .as_str()
                .unwrap()
                .ends_with("bad-1.0-0.json")
        );
        assert_eq!(records[0]["source_index"], 1);
    }

    #[test]
    fn collect_rejects_empty_and_symlinked_meta_dir() {
        assert!(dispatch(14, json!({ "prefix": "" })).is_err());
        #[cfg(unix)]
        {
            let dir = tempfile::tempdir().unwrap();
            let real = dir.path().join("real");
            fs::create_dir(&real).unwrap();
            std::os::unix::fs::symlink(&real, dir.path().join("conda-meta")).unwrap();
            assert!(dispatch(14, json!({ "prefix": dir.path() })).is_err());
        }
    }

    #[test]
    fn batch_isolates_failures_and_keeps_indices() {
        let good = b64(&fixture("conda-meta/envscout-demo-1.2.3-py311_0.json"));
        let bad = b64(&fixture("conda-meta/bad-1.0-0.json"));
        let value = dispatch(2, json!({ "documents": [good, bad, good] })).unwrap();
        let records = value["records"].as_array().unwrap();
        let errors = value["errors"].as_array().unwrap();
        assert_eq!(records.len(), 2);
        assert_eq!(records[0]["source_index"], 0);
        assert_eq!(records[1]["source_index"], 2);
        assert_eq!(errors.len(), 1);
        assert_eq!(errors[0]["index"], 1);
        let _ = decode_document(&good).unwrap();
    }

    #[test]
    fn batch_all_bad_returns_errors_not_failure() {
        let value = dispatch(2, json!({ "documents": ["!!!", b64(b"{")] })).unwrap();
        assert_eq!(value["records"], json!([]));
        assert_eq!(value["errors"].as_array().unwrap().len(), 2);
        assert_eq!(value["errors"][0]["index"], 0);
        assert_eq!(value["errors"][1]["index"], 1);
    }

    #[test]
    fn empty_batch_returns_empty_arrays() {
        let value = dispatch(2, json!({ "documents": [] })).unwrap();
        assert_eq!(value, json!({ "records": [], "errors": [] }));
    }
}
