//! Image export: `skopeo copy docker://<image> oci-archive:...`, checksum,
//! and transfer-header construction.

use std::path::{Path, PathBuf};
use std::time::{Duration, SystemTime, UNIX_EPOCH};

use aap2_common::skopeo::{self, SkopeoErrorKind};
use aap2_common::transfer::TransferHeader;
use aap2_common::{checksum, error::Result as Aap2Result};
use uuid::Uuid;

/// Default timeout for the `skopeo copy` export step. Large images over a
/// slow/rate-limited registry connection can legitimately take a while;
/// this is intentionally generous (Phase 6 formalizes timeout policy
/// across all subprocess calls).
const EXPORT_TIMEOUT: Duration = Duration::from_secs(600);

#[derive(thiserror::Error, Debug)]
pub enum ExportError {
    #[error("skopeo is not installed or not on PATH - install it and make sure `skopeo --version` works")]
    SkopeoNotInstalled,

    #[error("could not export image '{image_ref}': authentication failed, or the image/tag does not exist ({message})")]
    AuthOrImageNotFound { image_ref: String, message: String },

    #[error("ran out of disk space exporting '{image_ref}' to {dest_dir}: {message}")]
    DiskFull {
        image_ref: String,
        dest_dir: String,
        message: String,
    },

    #[error("timed out exporting '{image_ref}' after {timeout:?}")]
    Timeout {
        image_ref: String,
        timeout: Duration,
    },

    #[error("skopeo export of '{image_ref}' failed: {message}")]
    Other { image_ref: String, message: String },

    #[error("failed to compute checksum of exported archive: {0}")]
    Checksum(#[source] std::io::Error),

    #[error("failed to write checksum file: {0}")]
    WriteChecksumFile(#[source] std::io::Error),

    #[error("failed to stat exported archive: {0}")]
    StatArchive(#[source] std::io::Error),

    #[error("failed to create working directory {0}: {1}")]
    CreateWorkDir(String, #[source] std::io::Error),
}

/// The result of a successful export: the archive on disk, its checksum,
/// and the header that will be sent ahead of it.
pub struct ExportedImage {
    pub archive_path: PathBuf,
    pub header: TransferHeader,
}

/// Exports `image_ref` to an OCI archive under `work_dir`, computes its
/// SHA-256, writes `image.tar.sha256` alongside it, and returns a
/// [`TransferHeader`] ready to be sent. `work_dir` is created if it
/// doesn't exist.
pub async fn export_image(
    image_ref: &str,
    target_image_ref: &str,
    target_arch: &str,
    work_dir: &Path,
    transfer_id: Uuid,
    release_name: &str,
) -> Result<ExportedImage, ExportError> {
    tokio::fs::create_dir_all(work_dir)
        .await
        .map_err(|e| ExportError::CreateWorkDir(work_dir.display().to_string(), e))?;

    let archive_path = work_dir.join("image.tar");
    let dest = format!("oci-archive:{}", archive_path.display());
    let src = format!("docker://{image_ref}");

    tracing::info!(transfer_id = %transfer_id, %image_ref, %target_arch, dest = %archive_path.display(), "exporting image via skopeo");

    // The receiving cluster is always Linux (µD3TN/Harbor/skopeo all run in
    // Linux containers there), regardless of what OS `aap2-image-sender`
    // itself happens to be built/run on - so pin the export to linux/<target_arch>
    // rather than letting skopeo pick based on the local host (which,
    // developing on macOS, would otherwise select a "darwin" image variant
    // that doesn't exist for container images and fail outright).
    //
    skopeo::run(
        &[
            "copy",
            "--override-os",
            "linux",
            "--override-arch",
            target_arch,
            &src,
            &dest,
        ],
        EXPORT_TIMEOUT,
    )
    .await
    .map_err(|e| match e.kind {
        SkopeoErrorKind::NotInstalled => ExportError::SkopeoNotInstalled,
        SkopeoErrorKind::AuthOrNotFound => ExportError::AuthOrImageNotFound {
            image_ref: image_ref.to_string(),
            message: e.message,
        },
        SkopeoErrorKind::DiskFull => ExportError::DiskFull {
            image_ref: image_ref.to_string(),
            dest_dir: work_dir.display().to_string(),
            message: e.message,
        },
        SkopeoErrorKind::Timeout => ExportError::Timeout {
            image_ref: image_ref.to_string(),
            timeout: EXPORT_TIMEOUT,
        },
        SkopeoErrorKind::NetworkOrRegistryUnavailable | SkopeoErrorKind::Other => {
            ExportError::Other {
                image_ref: image_ref.to_string(),
                message: e.message,
            }
        }
    })?;

    let metadata = tokio::fs::metadata(&archive_path)
        .await
        .map_err(ExportError::StatArchive)?;
    let archive_size_bytes = metadata.len();

    tracing::info!(transfer_id = %transfer_id, size_bytes = archive_size_bytes, "computing checksum");
    let archive_sha256 = checksum::sha256_file(&archive_path)
        .await
        .map_err(ExportError::Checksum)?;

    let sha256_path = work_dir.join("image.tar.sha256");
    tokio::fs::write(&sha256_path, format!("{archive_sha256}  image.tar\n"))
        .await
        .map_err(ExportError::WriteChecksumFile)?;
    tracing::info!(transfer_id = %transfer_id, sha256 = %archive_sha256, checksum_file = %sha256_path.display(), "checksum computed and recorded");

    let created_at_unix = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs();

    let header = TransferHeader {
        transfer_id,
        source_image_ref: image_ref.to_string(),
        target_image_ref: target_image_ref.to_string(),
        created_at_unix,
        archive_sha256,
        archive_size_bytes,
        release_name: release_name.to_string(),
    };

    Ok(ExportedImage {
        archive_path,
        header,
    })
}

/// Reads the exported archive back into memory so it can be framed with
/// its header and sent as a single AAP2 ADU payload.
pub async fn read_archive(path: &Path) -> Aap2Result<Vec<u8>> {
    tokio::fs::read(path)
        .await
        .map_err(aap2_common::Aap2Error::Io)
}
