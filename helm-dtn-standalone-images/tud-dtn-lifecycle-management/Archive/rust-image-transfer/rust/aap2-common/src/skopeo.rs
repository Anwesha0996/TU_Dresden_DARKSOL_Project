//! Thin wrapper around shelling out to `skopeo`, shared by the sender
//! (`copy docker://... oci-archive:...` in Phase 4) and the receiver
//! (`copy oci-archive:... docker://...` + `inspect` in Phase 5).
//!
//! Every invocation gets an explicit timeout so no subprocess call blocks
//! indefinitely, and its failure gets classified
//! into a [`SkopeoErrorKind`] so callers can log/act on "skopeo isn't
//! installed" differently from "bad credentials" differently from "disk
//! full" - see Phase 4's acceptance criterion.

use std::process::Stdio;
use std::time::Duration;

use tokio::process::Command;
use tokio::time::timeout;

/// Coarse classification of why a `skopeo` invocation failed, used to
/// produce an actionable log/error message instead of a raw exit code.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum SkopeoErrorKind {
    /// The `skopeo` binary itself could not be found/executed.
    NotInstalled,
    /// The invocation did not complete within the given timeout.
    Timeout,
    /// Authentication failed or the referenced image/tag does not exist.
    /// These are grouped because skopeo's error text for "wrong
    /// credentials" and "repository doesn't exist" is often
    /// indistinguishable without registry-specific parsing, and both are
    /// permanent (not worth retrying) from the caller's point of view.
    AuthOrNotFound,
    /// The local filesystem ran out of space while reading/writing the
    /// archive.
    DiskFull,
    /// Registry/network reachability issue.
    NetworkOrRegistryUnavailable,
    /// Anything else.
    Other,
}

#[derive(thiserror::Error, Debug)]
#[error("skopeo {args:?} failed ({kind:?}): {message}")]
pub struct SkopeoError {
    pub kind: SkopeoErrorKind,
    pub args: Vec<String>,
    pub message: String,
}

fn classify_stderr(stderr: &str) -> SkopeoErrorKind {
    let s = stderr.to_lowercase();
    if s.contains("no space left on device") {
        SkopeoErrorKind::DiskFull
    } else if s.contains("unauthorized")
        || s.contains("authentication required")
        || s.contains("manifest unknown")
        || s.contains("name unknown")
        || s.contains("not found")
        || s.contains("requested access to the resource is denied")
    {
        SkopeoErrorKind::AuthOrNotFound
    } else if s.contains("connection refused")
        || s.contains("no such host")
        || s.contains("timeout")
        || s.contains("temporary failure")
        || s.contains("i/o timeout")
        || s.contains("connection reset")
    {
        SkopeoErrorKind::NetworkOrRegistryUnavailable
    } else {
        SkopeoErrorKind::Other
    }
}

/// Runs `skopeo` with the given arguments, enforcing `timeout_dur` and
/// returning stdout on success or a classified [`SkopeoError`] otherwise.
pub async fn run(args: &[&str], timeout_dur: Duration) -> Result<String, SkopeoError> {
    run_program("skopeo", args, timeout_dur).await
}

/// Shared implementation behind [`run`].
async fn run_program(
    program: &str,
    args: &[&str],
    timeout_dur: Duration,
) -> Result<String, SkopeoError> {
    let args_owned: Vec<String> = args.iter().map(|s| s.to_string()).collect();
    tracing::debug!(program, ?args, timeout = ?timeout_dur, "running subprocess");

    let spawn_result = Command::new(program)
        .args(args)
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn();

    let child = match spawn_result {
        Ok(child) => child,
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => {
            return Err(SkopeoError {
                kind: SkopeoErrorKind::NotInstalled,
                args: args_owned,
                message: format!("the `{program}` binary was not found on PATH - is it installed?"),
            })
        }
        Err(e) => {
            return Err(SkopeoError {
                kind: SkopeoErrorKind::Other,
                args: args_owned,
                message: format!("failed to spawn {program}: {e}"),
            })
        }
    };

    let output = match timeout(timeout_dur, child.wait_with_output()).await {
        Ok(Ok(output)) => output,
        Ok(Err(e)) => {
            return Err(SkopeoError {
                kind: SkopeoErrorKind::Other,
                args: args_owned,
                message: format!("i/o error waiting for skopeo: {e}"),
            })
        }
        Err(_) => {
            return Err(SkopeoError {
                kind: SkopeoErrorKind::Timeout,
                args: args_owned,
                message: format!("skopeo did not complete within {timeout_dur:?}"),
            })
        }
    };

    if !output.status.success() {
        let stderr = String::from_utf8_lossy(&output.stderr).trim().to_string();
        return Err(SkopeoError {
            kind: classify_stderr(&stderr),
            args: args_owned,
            message: if stderr.is_empty() {
                format!("skopeo exited with status {:?}", output.status.code())
            } else {
                stderr
            },
        });
    }

    Ok(String::from_utf8_lossy(&output.stdout).to_string())
}
