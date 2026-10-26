//! JSON operations over rattler_lock: enumerate packages of one locked
//! environment/platform (op 12) and compare a lock with an installed set (op 13).
//!
//! Errors are returned as `Err(String)`; nothing in this module panics on
//! caller input.

use std::collections::{BTreeMap, BTreeSet};
use std::str::FromStr;

use rattler_conda_types::{PackageName as CondaPackageName, Subdir, package::BuildString};
use rattler_lock::{CondaPackageData, LockFile, LockedPackage, PypiPackageData};
use rattler_redaction::Redact;
use serde::{Deserialize, Serialize};
use serde_json::Value;

/// Locked package as seen by the Go caller. Field names match the Go struct tags.
#[derive(Debug, Clone, Serialize, PartialEq, Eq, PartialOrd, Ord)]
pub struct LockedPackageJson {
    pub kind: String,
    pub name: String,
    pub version: String,
    pub build: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub build_number: Option<u64>,
    pub subdir: String,
    pub url: String,
    pub sha256: String,
    pub md5: String,
    pub source: bool,
    pub depends: Vec<String>,
    /// Set only for source records whose version cannot be determined.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub reason: Option<String>,
}

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
pub struct LockPackagesParams {
    pub lock: String,
    pub environment: String,
    pub subdir: String,
}

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
pub struct InstalledPrefixRecord {
    pub name: String,
    pub version: String,
    pub build: String,
    pub build_number: u64,
    pub subdir: String,
    #[serde(default)]
    pub url: String,
    #[serde(default)]
    pub sha256: String,
    #[serde(default)]
    pub md5: String,
}

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
pub struct InstalledPyPI {
    pub name: String,
    pub version: String,
    #[serde(default)]
    pub url: String,
    #[serde(default)]
    pub sha256: String,
    #[serde(default)]
    pub md5: String,
    #[serde(default)]
    pub editable: bool,
}

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
pub struct LockSyncParams {
    pub lock: String,
    pub environment: String,
    pub subdir: String,
    /// Installed conda records. Absent means "not provided".
    #[serde(default)]
    pub installed: Vec<InstalledPrefixRecord>,
    /// Caller assertion that `installed` is the complete conda set.
    #[serde(default)]
    pub conda_complete: bool,
    /// Scope chosen by the entry point: "conda" or "conda+pypi". Never inferred.
    pub scope: String,
    /// Installed PyPI distributions; meaningful only when `pypi_provided` is true.
    #[serde(default)]
    pub pypi: Vec<InstalledPyPI>,
    /// True when the caller supplied a PyPI set (possibly empty).
    #[serde(default)]
    pub pypi_provided: bool,
    /// Caller assertion that `pypi` is the complete PyPI set.
    #[serde(default)]
    pub pypi_complete: bool,
}

/// Dispatch entry for op 12.
pub fn lock_packages(params: Value) -> Result<Value, String> {
    let params: LockPackagesParams =
        serde_json::from_value(params).map_err(|e| format!("invalid input: {e}"))?;
    let packages = enumerate(&params.lock, &params.environment, &params.subdir)?;
    serde_json::to_value(packages).map_err(|e| format!("encode: {e}"))
}

/// Parse and select packages for one environment/platform.
fn enumerate(
    lock: &str,
    environment: &str,
    subdir: &str,
) -> Result<Vec<LockedPackageJson>, String> {
    if environment.is_empty() {
        return Err("invalid input: environment is required".into());
    }
    if subdir.is_empty() {
        return Err("invalid input: subdir is required".into());
    }
    let subdir = Subdir::from_str(subdir)
        .map_err(|_| format!("invalid input: unknown subdir {subdir:?}"))?;
    let file = LockFile::from_str_with_base_directory(lock, None)
        .map_err(|e| format!("malformed lock: {e}"))?;
    let env = file.environment(environment).ok_or_else(|| {
        format!("invalid input: environment {environment:?} is missing from lock")
    })?;
    let platform_name = subdir.to_string();
    let platform = file
        .platform(&platform_name)
        .ok_or_else(|| format!("invalid input: platform {platform_name:?} is missing from lock"))?;
    let packages = env.packages(platform).ok_or_else(|| {
        format!("invalid input: platform {platform_name:?} is not in environment {environment:?}")
    })?;
    let mut out: Vec<LockedPackageJson> = packages.map(to_json).collect();
    out.sort();
    Ok(out)
}

/// Lowercase hex for digest output (rattler hashes are GenericArray<u8, _>).
fn hex_lower<T: AsRef<[u8]>>(bytes: &T) -> String {
    use std::fmt::Write;
    bytes.as_ref().iter().fold(String::new(), |mut s, b| {
        let _ = write!(s, "{b:02x}");
        s
    })
}

fn to_json(package: &LockedPackage) -> LockedPackageJson {
    match package {
        LockedPackage::Conda(data) => conda_json(data),
        LockedPackage::Pypi(data) => pypi_json(data),
    }
}

fn conda_json(data: &CondaPackageData) -> LockedPackageJson {
    let name = data.name().as_normalized().to_string();
    let url = safe_url(&data.location().to_string());
    match data.record() {
        Some(record) => {
            let (sha256, md5) = (
                record.sha256.as_ref().map(hex_lower).unwrap_or_default(),
                record.md5.as_ref().map(hex_lower).unwrap_or_default(),
            );
            LockedPackageJson {
                kind: "conda".into(),
                name,
                version: record.version.to_string(),
                build: record.build.to_string(),
                build_number: Some(record.build_number),
                subdir: record.subdir.clone(),
                url,
                sha256,
                md5,
                source: data.as_source().is_some(),
                depends: record.depends.clone(),
                reason: None,
            }
        }
        None => LockedPackageJson {
            kind: "conda".into(),
            name,
            version: String::new(),
            build: String::new(),
            build_number: None,
            subdir: String::new(),
            url,
            sha256: String::new(),
            md5: String::new(),
            source: true,
            depends: data.depends().to_vec(),
            reason: Some("source record has no resolved version".into()),
        },
    }
}

fn pypi_json(data: &PypiPackageData) -> LockedPackageJson {
    let (name, version, url, sha256, md5, source) = match data {
        PypiPackageData::Distribution(d) => {
            let (sha256, md5) = match &d.hash {
                Some(h) => (
                    h.sha256().map(hex_lower).unwrap_or_default(),
                    h.md5().map(hex_lower).unwrap_or_default(),
                ),
                None => (String::new(), String::new()),
            };
            (
                d.name.as_ref().to_string(),
                d.version.to_string(),
                safe_url(&d.location.to_string()),
                sha256,
                md5,
                false,
            )
        }
        PypiPackageData::Source(s) => (
            s.name.as_ref().to_string(),
            String::new(),
            safe_url(&s.location.to_string()),
            String::new(),
            String::new(),
            true,
        ),
    };
    LockedPackageJson {
        kind: "pypi".into(),
        name,
        version,
        build: String::new(),
        build_number: None,
        subdir: String::new(),
        url,
        sha256,
        md5,
        source,
        depends: Vec::new(),
        reason: if source {
            Some("source directory; version not fixed by lock".into())
        } else {
            None
        },
    }
}

// ---------------------------------------------------------------- op 13

#[derive(Serialize, Clone, PartialEq, Eq, Debug)]
pub struct InstalledOut {
    pub kind: String,
    pub name: String,
    pub version: String,
    pub build: String,
    /// Comparison-only build number; not part of the Go wire shape.
    #[serde(skip)]
    pub build_number: Option<u64>,
    pub subdir: String,
    pub url: String,
    pub sha256: String,
    pub md5: String,
}

#[derive(Serialize, Debug, PartialEq)]
pub struct PackageChange {
    pub locked: LockedPackageJson,
    pub installed: InstalledOut,
    pub reasons: Vec<String>,
}

#[derive(Serialize, Debug, PartialEq)]
pub struct ComparisonIssue {
    pub kind: String,
    pub name: String,
    pub reason: String,
}

#[derive(Serialize, Debug, PartialEq)]
pub struct SyncReport {
    pub scope: String,
    pub missing: Vec<LockedPackageJson>,
    pub extra: Vec<InstalledOut>,
    pub changed: Vec<PackageChange>,
    pub unknown: Vec<ComparisonIssue>,
    pub complete: bool,
    pub matches: bool,
}

/// Dispatch entry for op 13.
pub fn lock_sync(params: Value) -> Result<Value, String> {
    let params: LockSyncParams =
        serde_json::from_value(params).map_err(|e| format!("invalid input: {e}"))?;
    let locked = enumerate(&params.lock, &params.environment, &params.subdir)?;
    if params.scope != "conda" && params.scope != "conda+pypi" {
        return Err("invalid comparison scope".into());
    }

    // Scope is explicit, never inferred from lock contents. PyPI counts as complete only
    // when the entry point put it in scope, the caller provided a set, and asserted it
    // complete. A conda-only scope never proves PyPI absence.
    let pypi_in_scope = params.scope == "conda+pypi";
    let pypi_scope_complete = pypi_in_scope && params.pypi_provided && params.pypi_complete;
    let conda_scope_complete = params.conda_complete;

    let mut report = SyncReport {
        scope: params.scope.clone(),
        missing: Vec::new(),
        extra: Vec::new(),
        changed: Vec::new(),
        unknown: Vec::new(),
        complete: false,
        matches: false,
    };
    if !conda_scope_complete {
        report.unknown.push(ComparisonIssue {
            kind: "conda".into(),
            name: String::new(),
            reason: "installed conda observations are not asserted complete".into(),
        });
    }
    if pypi_in_scope && !pypi_scope_complete {
        report.unknown.push(ComparisonIssue {
            kind: "pypi".into(),
            name: String::new(),
            reason: "installed PyPI observations are absent or not asserted complete".into(),
        });
    }

    // Duplicate normalized names in the lock make matching ambiguous.
    let mut seen_locked: BTreeMap<(String, String), usize> = BTreeMap::new();
    for p in &locked {
        if p.kind == "pypi" && !pypi_in_scope {
            continue;
        }
        *seen_locked
            .entry((p.kind.clone(), p.name.clone()))
            .or_default() += 1;
    }
    for ((kind, name), n) in &seen_locked {
        if *n > 1 {
            report.unknown.push(ComparisonIssue {
                kind: kind.clone(),
                name: name.clone(),
                reason: "duplicate locked name; cannot match uniquely".into(),
            });
        }
    }

    let installed_conda = params.installed;
    let mut installed_conda_map: BTreeMap<String, Vec<InstalledOut>> = BTreeMap::new();
    for rec in installed_conda {
        match validate_installed(rec) {
            Ok(out) => installed_conda_map
                .entry(normalize_conda_name(&out.name))
                .or_default()
                .push(out),
            Err(reason) => report.unknown.push(ComparisonIssue {
                kind: "conda".into(),
                name: String::new(),
                reason,
            }),
        }
    }
    for (name, recs) in &installed_conda_map {
        if recs.len() > 1 {
            report.unknown.push(ComparisonIssue {
                kind: "conda".into(),
                name: name.clone(),
                reason: "duplicate installed name; cannot match uniquely".into(),
            });
        }
    }

    let mut matched_installed: BTreeSet<String> = BTreeSet::new();
    for lp in locked.iter().filter(|p| p.kind == "conda") {
        let key = normalize_conda_name(&lp.name);
        matched_installed.insert(key.clone());
        if seen_locked
            .get(&("conda".to_string(), lp.name.clone()))
            .copied()
            .unwrap_or(0)
            > 1
        {
            continue;
        }
        if lp.source || lp.reason.is_some() {
            report.unknown.push(ComparisonIssue {
                kind: "conda".into(),
                name: lp.name.clone(),
                reason: "locked source record has no resolved version; not verified".into(),
            });
            continue;
        }
        match installed_conda_map.get(&key) {
            None => {
                if conda_scope_complete {
                    report.missing.push(lp.clone());
                } else {
                    report.unknown.push(ComparisonIssue {
                        kind: "conda".into(),
                        name: lp.name.clone(),
                        reason: "installed conda set not asserted complete; absence not proven"
                            .into(),
                    });
                }
            }
            Some(recs) if recs.len() == 1 => {
                matched_installed.insert(key.clone());
                compare_conda(lp, &recs[0], &mut report);
            }
            Some(_) => {}
        }
    }
    if conda_scope_complete {
        for (key, recs) in &installed_conda_map {
            if !matched_installed.contains(key) && recs.len() == 1 {
                report.extra.push(recs[0].clone());
            }
        }
    }

    // PyPI: when the caller provided no PyPI set, every locked PyPI package is Unknown
    // rather than silently ignored.
    if pypi_in_scope && !params.pypi_provided {
        for lp in locked.iter().filter(|p| p.kind == "pypi") {
            report.unknown.push(ComparisonIssue {
                kind: "pypi".into(),
                name: lp.name.clone(),
                reason: "installed PyPI set not provided; not compared".into(),
            });
        }
    } else if pypi_in_scope {
        let pypi = params.pypi;
        let mut installed_pypi: BTreeMap<String, Vec<InstalledPyPI>> = BTreeMap::new();
        for p in pypi {
            if pep508_rs::PackageName::from_str(&p.name).is_err()
                || pep440_rs::Version::from_str(&p.version).is_err()
                || !valid_hash(&p.sha256, 64)
                || !valid_hash(&p.md5, 32)
            {
                report.unknown.push(ComparisonIssue {
                    kind: "pypi".into(),
                    name: p.name.clone(),
                    reason: "installed PyPI name, version or hash is invalid".into(),
                });
                continue;
            }
            installed_pypi
                .entry(normalize_pypi_name(&p.name))
                .or_default()
                .push(p);
        }
        for (name, records) in &installed_pypi {
            if records.len() > 1 {
                report.unknown.push(ComparisonIssue {
                    kind: "pypi".into(),
                    name: name.clone(),
                    reason: "duplicate installed name; cannot match uniquely".into(),
                });
            }
        }
        for lp in locked.iter().filter(|p| p.kind == "pypi") {
            let key = normalize_pypi_name(&lp.name);
            if seen_locked
                .get(&(lp.kind.clone(), lp.name.clone()))
                .copied()
                .unwrap_or(0)
                > 1
            {
                continue;
            }
            if lp.source || lp.reason.is_some() {
                report.unknown.push(ComparisonIssue {
                    kind: "pypi".into(),
                    name: lp.name.clone(),
                    reason: "locked source directory; not verified".into(),
                });
                continue;
            }
            match installed_pypi.get(&key) {
                None if pypi_scope_complete => report.missing.push(lp.clone()),
                None => report.unknown.push(ComparisonIssue {
                    kind: "pypi".into(),
                    name: lp.name.clone(),
                    reason: "installed PyPI set not asserted complete; absence not proven".into(),
                }),
                Some(recs) if recs.len() == 1 => {
                    let rec = &recs[0];
                    compare_pypi(lp, rec, &mut report);
                }
                Some(_) => report.unknown.push(ComparisonIssue {
                    kind: "pypi".into(),
                    name: lp.name.clone(),
                    reason: "duplicate installed name; cannot match uniquely".into(),
                }),
            }
        }
        if pypi_scope_complete {
            let locked_pypi: BTreeSet<String> = locked
                .iter()
                .filter(|p| p.kind == "pypi")
                .map(|p| normalize_pypi_name(&p.name))
                .collect();
            for (key, recs) in &installed_pypi {
                if !locked_pypi.contains(key) && recs.len() == 1 {
                    let r = &recs[0];
                    report.extra.push(InstalledOut {
                        build_number: None,
                        kind: "pypi".into(),
                        name: r.name.clone(),
                        version: r.version.clone(),
                        build: String::new(),
                        subdir: String::new(),
                        url: safe_url(&r.url),
                        sha256: r.sha256.clone(),
                        md5: r.md5.clone(),
                    });
                }
            }
        }
    }

    // PyPI only constrains completeness when it is in scope.
    let pypi_ok = !pypi_in_scope || pypi_scope_complete;
    report.complete = report.unknown.is_empty() && conda_scope_complete && pypi_ok;
    report.matches = report.complete
        && report.missing.is_empty()
        && report.extra.is_empty()
        && report.changed.is_empty();

    report.missing.sort();
    report.extra.sort_by(|a, b| {
        (&a.kind, &a.name, &a.version, &a.build).cmp(&(&b.kind, &b.name, &b.version, &b.build))
    });
    report.changed.sort_by(|a, b| a.locked.cmp(&b.locked));
    report
        .unknown
        .sort_by(|a, b| (&a.kind, &a.name, &a.reason).cmp(&(&b.kind, &b.name, &b.reason)));
    serde_json::to_value(report).map_err(|e| format!("encode: {e}"))
}

fn validate_installed(rec: InstalledPrefixRecord) -> Result<InstalledOut, String> {
    let name = CondaPackageName::from_str(&rec.name)
        .map_err(|_| format!("installed conda record has invalid name {:?}", rec.name))?;
    let version = rattler_conda_types::Version::from_str(&rec.version).map_err(|_| {
        format!(
            "installed conda record {:?} has invalid version {:?}",
            rec.name, rec.version
        )
    })?;
    Subdir::from_str(&rec.subdir).map_err(|_| {
        format!(
            "installed conda record {:?} has invalid subdir {:?}",
            rec.name, rec.subdir
        )
    })?;
    if rec.build.is_empty()
        || BuildString::from_str(&rec.build).is_err()
        || !valid_hash(&rec.sha256, 64)
        || !valid_hash(&rec.md5, 32)
    {
        return Err(format!(
            "installed conda record {:?} has invalid build or hash",
            rec.name
        ));
    }
    Ok(InstalledOut {
        kind: "conda".into(),
        name: name.as_normalized().to_string(),
        version: version.to_string(),
        build: rec.build,
        build_number: Some(rec.build_number),
        subdir: rec.subdir,
        url: safe_url(&rec.url),
        sha256: rec.sha256,
        md5: rec.md5,
    })
}

fn normalize_conda_name(name: &str) -> String {
    CondaPackageName::from_str(name)
        .map(|n| n.as_normalized().to_string())
        .unwrap_or_else(|_| name.to_string())
}

/// Canonical PEP 503 normalization, shared with rattler's PyPI lock types.
fn normalize_pypi_name(name: &str) -> String {
    pep508_rs::PackageName::from_str(name)
        .map(|name| name.to_string())
        .unwrap_or_default()
}

fn valid_hash(value: &str, length: usize) -> bool {
    value.is_empty() || (value.len() == length && value.bytes().all(|b| b.is_ascii_hexdigit()))
}

fn safe_url(value: &str) -> String {
    match url::Url::parse(value) {
        Ok(mut parsed) => {
            let _ = parsed.set_username("");
            let _ = parsed.set_password(None);
            parsed.redact().to_string()
        }
        Err(_) if value.contains("://") => "<invalid URL>".into(),
        Err(_) => value.into(),
    }
}

fn compare_conda(lp: &LockedPackageJson, inst: &InstalledOut, report: &mut SyncReport) {
    let mut reasons = Vec::new();
    let mut unknown = Vec::new();

    let locked_ver = rattler_conda_types::Version::from_str(&lp.version).ok();
    let inst_ver = rattler_conda_types::Version::from_str(&inst.version).ok();
    match (locked_ver, inst_ver) {
        (Some(a), Some(b)) => {
            if a != b {
                reasons.push("version".to_string());
            }
        }
        _ => unknown.push("version not parseable".to_string()),
    }
    if lp.build != inst.build {
        reasons.push("build".into());
    }
    if matches!((lp.build_number, inst.build_number), (Some(a), Some(b)) if a != b) {
        reasons.push("build_number".into());
    }
    // noarch packages are relocatable across subdirs; only compare subdir otherwise.
    let lock_is_noarch = lp.subdir == "noarch";
    if !lock_is_noarch && lp.subdir != inst.subdir {
        reasons.push("subdir".into());
    }

    compare_hashes(
        &lp.sha256,
        &lp.md5,
        &inst.sha256,
        &inst.md5,
        &mut reasons,
        &mut unknown,
    );

    finish(lp.clone(), inst.clone(), reasons, unknown, report);
}

fn compare_pypi(lp: &LockedPackageJson, rec: &InstalledPyPI, report: &mut SyncReport) {
    let mut reasons = Vec::new();
    let mut unknown = Vec::new();
    if rec.editable {
        unknown.push("editable install; source provenance not comparable".to_string());
    }
    // PEP 440 equivalence: "1.0" == "1.0.0". An unparseable installed version is unknown,
    // not changed, because its identity cannot be established.
    match (
        pep440_rs::Version::from_str(&lp.version),
        pep440_rs::Version::from_str(&rec.version),
    ) {
        (Ok(a), Ok(b)) => {
            if a != b {
                reasons.push("version".into());
            }
        }
        _ => unknown.push("version not parseable as PEP 440".into()),
    }
    compare_hashes(
        &lp.sha256,
        &lp.md5,
        &rec.sha256,
        &rec.md5,
        &mut reasons,
        &mut unknown,
    );
    let inst = InstalledOut {
        build_number: None,
        kind: "pypi".into(),
        name: rec.name.clone(),
        version: rec.version.clone(),
        build: String::new(),
        subdir: String::new(),
        url: safe_url(&rec.url),
        sha256: rec.sha256.clone(),
        md5: rec.md5.clone(),
    };
    finish(lp.clone(), inst, reasons, unknown, report);
}

/// Hash rules: compare only fields present on both sides. A matching strong hash
/// is sufficient; a locked hash with no installed counterpart is unknown, not changed.
fn compare_hashes(
    lock_sha: &str,
    lock_md5: &str,
    inst_sha: &str,
    inst_md5: &str,
    reasons: &mut Vec<String>,
    unknown: &mut Vec<String>,
) {
    if !lock_sha.is_empty() && !inst_sha.is_empty() {
        if !lock_sha.eq_ignore_ascii_case(inst_sha) {
            reasons.push("sha256".into());
        }
        return;
    }
    if !lock_md5.is_empty() && !inst_md5.is_empty() {
        if !lock_md5.eq_ignore_ascii_case(inst_md5) {
            reasons.push("md5".into());
        }
        if !lock_sha.is_empty() {
            unknown.push("locked SHA256 is absent from installed metadata".into());
        }
        return;
    }
    if !lock_sha.is_empty() || !lock_md5.is_empty() {
        unknown.push("no common hash to verify artifact identity".into());
    }
}

fn finish(
    lp: LockedPackageJson,
    inst: InstalledOut,
    reasons: Vec<String>,
    unknown: Vec<String>,
    report: &mut SyncReport,
) {
    for reason in unknown {
        report.unknown.push(ComparisonIssue {
            kind: lp.kind.clone(),
            name: lp.name.clone(),
            reason,
        });
    }
    if !reasons.is_empty() {
        report.changed.push(PackageChange {
            locked: lp,
            installed: inst,
            reasons,
        });
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    const LOCK: &str = include_str!("../../../testdata/pixi.lock");

    #[test]
    fn enumerates_default_environment() {
        let v =
            lock_packages(json!({"lock": LOCK, "environment": "default", "subdir": "osx-arm64"}))
                .unwrap();
        let arr = v.as_array().unwrap();
        assert_eq!(arr.len(), 2);
        assert_eq!(arr[0]["kind"], "conda");
        assert_eq!(arr[1]["kind"], "pypi");
    }

    #[test]
    fn missing_environment_and_platform_are_distinct() {
        let e1 = lock_packages(json!({"lock": LOCK, "environment": "nope", "subdir": "osx-arm64"}))
            .unwrap_err();
        let e2 =
            lock_packages(json!({"lock": LOCK, "environment": "default", "subdir": "linux-64"}))
                .unwrap_err();
        assert!(e1.contains("environment"));
        assert!(e2.contains("platform"));
    }

    #[test]
    fn rejects_malformed_and_empty_input_without_panic() {
        assert!(
            lock_packages(
                json!({"lock": "not: [valid", "environment": "d", "subdir": "osx-arm64"})
            )
            .is_err()
        );
        assert!(
            lock_packages(json!({"lock": LOCK, "environment": "", "subdir": "osx-arm64"})).is_err()
        );
        assert!(
            lock_packages(json!({"lock": LOCK, "environment": "default", "subdir": ""})).is_err()
        );
    }

    #[test]
    fn pep503_normalization() {
        assert_eq!(normalize_pypi_name("Foo_Bar.baz"), "foo-bar-baz");
        assert_eq!(normalize_pypi_name("a--b"), "a-b");
    }

    #[test]
    fn hash_mismatch_is_changed_and_absent_hash_is_unknown() {
        let mut r = SyncReport {
            scope: String::new(),
            missing: vec![],
            extra: vec![],
            changed: vec![],
            unknown: vec![],
            complete: false,
            matches: false,
        };
        let lp = LockedPackageJson {
            kind: "conda".into(),
            name: "x".into(),
            version: "1".into(),
            build: "b".into(),
            build_number: None,
            subdir: "linux-64".into(),
            url: "u".into(),
            sha256: "aa".into(),
            md5: String::new(),
            source: false,
            depends: vec![],
            reason: None,
        };
        let inst_bad = InstalledOut {
            build_number: None,
            kind: "conda".into(),
            name: "x".into(),
            version: "1".into(),
            build: "b".into(),
            subdir: "linux-64".into(),
            url: "u".into(),
            sha256: "bb".into(),
            md5: String::new(),
        };
        compare_conda(&lp, &inst_bad, &mut r);
        assert_eq!(r.changed.len(), 1);
        assert_eq!(r.changed[0].reasons, vec!["sha256".to_string()]);

        let inst_nohash = InstalledOut {
            sha256: String::new(),
            ..inst_bad.clone()
        };
        let mut r2 = SyncReport { ..r };
        r2.changed.clear();
        compare_conda(&lp, &inst_nohash, &mut r2);
        assert!(r2.changed.is_empty());
        assert_eq!(r2.unknown.len(), 1);
    }

    #[test]
    fn sync_reports_changed_extra_and_pypi_unknown() {
        let v = lock_sync(json!({
            "lock": include_str!("../../../testdata/lock/pypi_mixed.lock"),
            "environment": "default", "subdir": "linux-64",
            "installed": [{"name": "python", "version": "3.12.1", "build": "h123_0",
                "build_number": 0, "subdir": "linux-64",
                "sha256": "9999999999999999999999999999999999999999999999999999999999999999"},
                {"name": "stray", "version": "1", "build": "0", "build_number": 0, "subdir": "linux-64"}],
            "conda_complete": true, "scope": "conda+pypi", "pypi_provided": false
        }))
        .unwrap();
        assert_eq!(v["changed"][0]["reasons"][0], "sha256");
        assert_eq!(v["extra"][0]["name"], "stray");
        assert_eq!(v["matches"], false);
        assert_eq!(v["complete"], false);
        assert_eq!(v["unknown"].as_array().unwrap().len(), 3);
    }

    #[test]
    fn invalid_installed_record_is_unknown_not_fatal() {
        let v = lock_sync(json!({
            "lock": include_str!("../../../testdata/lock/conda_only.lock"),
            "environment": "default", "subdir": "linux-64",
            "installed": [{"name": "Bad Name!", "version": "1", "build": "0", "build_number": 0, "subdir": "linux-64"}],
            "conda_complete": true, "scope": "conda", "pypi_provided": false
        }))
        .unwrap();
        assert_eq!(v["complete"], false);
        assert!(
            v["unknown"]
                .as_array()
                .unwrap()
                .iter()
                .any(|u| u["reason"].as_str().unwrap().contains("invalid name"))
        );
    }

    #[test]
    fn noarch_installed_subdir_is_not_a_change() {
        let mut r = SyncReport {
            scope: String::new(),
            missing: vec![],
            extra: vec![],
            changed: vec![],
            unknown: vec![],
            complete: false,
            matches: false,
        };
        let lp = LockedPackageJson {
            kind: "conda".into(),
            name: "x".into(),
            version: "1".into(),
            build: "b".into(),
            build_number: None,
            subdir: "noarch".into(),
            url: String::new(),
            sha256: String::new(),
            md5: String::new(),
            source: false,
            depends: vec![],
            reason: None,
        };
        let inst = InstalledOut {
            build_number: None,
            kind: "conda".into(),
            name: "x".into(),
            version: "1".into(),
            build: "b".into(),
            subdir: "linux-64".into(),
            url: String::new(),
            sha256: String::new(),
            md5: String::new(),
        };
        compare_conda(&lp, &inst, &mut r);
        assert!(r.changed.is_empty());
    }

    #[test]
    fn resolved_source_records_remain_unverified() {
        let document = include_str!("../../../testdata/lock/conda_only.lock");
        let file = LockFile::from_str_with_base_directory(document, None).unwrap();
        let env = file.environment("default").unwrap();
        let platform = file.platform("linux-64").unwrap();
        let record = env
            .packages(platform)
            .unwrap()
            .find_map(|package| {
                if let LockedPackage::Conda(data) = package {
                    data.record().cloned()
                } else {
                    None
                }
            })
            .unwrap();
        let source = rattler_lock::CondaSourceData::full(
            rattler_lock::UrlOrPath::from_str("./source").unwrap(),
            None,
            BTreeMap::new(),
            None,
            record,
            BTreeMap::new(),
        );
        let projected = conda_json(&CondaPackageData::from(source));
        assert!(projected.source);
        assert!(!projected.version.is_empty());
    }
}
