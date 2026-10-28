//! Per-transfer processing for the space-side image receiver.

use std::path::Path;
use std::time::Duration;

use aap2_common::checksum;
use aap2_common::skopeo::{self, SkopeoErrorKind};
use aap2_common::transfer::{decode_transfer_payload, TransferHeader};

const PUSH_TIMEOUT: Duration = Duration::from_secs(600);
const INSPECT_TIMEOUT: Duration = Duration::from_secs(60);

#[derive(Clone)]
pub struct RegistryCreds {
    pub user: String,
    pub password: String,
}

pub struct PushTarget<'a> {
    pub registry: &'a str,
    pub creds: &'a RegistryCreds,
    pub insecure: bool,
}

#[derive(thiserror::Error, Debug)]
pub enum ReceiveError {
    #[error("failed to parse transfer payload: {0}")]
    Decode(#[from] aap2_common::transfer::FrameError),
    #[error("failed to create temporary work directory: {0}")]
    TempDir(#[source] std::io::Error),
    #[error("failed to write received archive: {0}")]
    WriteArchive(#[source] std::io::Error),
    #[error("failed to checksum received archive: {0}")]
    Checksum(#[source] std::io::Error),
    #[error("failed to create temporary registry auth file: {0}")]
    AuthFile(#[source] std::io::Error),
    #[error("checksum mismatch for transfer {transfer_id}: expected {expected}, got {actual}")]
    ChecksumMismatch {
        transfer_id: uuid::Uuid,
        expected: String,
        actual: String,
    },
    #[error("skopeo is not installed or not on PATH")]
    SkopeoNotInstalled,
    #[error("skopeo push failed for transfer {transfer_id}: {message}")]
    PushFailed {
        transfer_id: uuid::Uuid,
        message: String,
    },
    #[error("skopeo inspect failed for transfer {transfer_id}: {message}")]
    InspectFailed {
        transfer_id: uuid::Uuid,
        message: String,
    },
}

pub async fn handle_transfer(payload: &[u8], target: &PushTarget<'_>) -> Result<(), ReceiveError> {
    let (header, archive) = decode_transfer_payload(payload)?;
    let temp_dir = tempfile::Builder::new()
        .prefix("aap2-image-receiver-")
        .tempdir()
        .map_err(ReceiveError::TempDir)?;
    let archive_path = temp_dir.path().join("image.tar");

    tokio::fs::write(&archive_path, archive)
        .await
        .map_err(ReceiveError::WriteArchive)?;

    let actual_sha256 = checksum::sha256_file(&archive_path)
        .await
        .map_err(ReceiveError::Checksum)?;
    if actual_sha256 != header.archive_sha256 {
        return Err(ReceiveError::ChecksumMismatch {
            transfer_id: header.transfer_id,
            expected: header.archive_sha256,
            actual: actual_sha256,
        });
    }

    tracing::info!(
        transfer_id = %header.transfer_id,
        source_image = %header.source_image_ref,
        target_image = %header.target_image_ref,
        release = %header.release_name,
        "received image archive; pushing to Harbor"
    );

    let auth_path = temp_dir.path().join("auth.json");
    write_registry_auth(&auth_path, target).await?;

    push_archive(&header, &archive_path, &auth_path, target).await?;
    let digest = inspect_pushed_image(&header, &auth_path, target).await?;

    tracing::info!(
        transfer_id = %header.transfer_id,
        digest = %digest,
        "transfer complete: image pushed and verified"
    );
    Ok(())
}

async fn write_registry_auth(
    auth_path: &Path,
    target: &PushTarget<'_>,
) -> Result<(), ReceiveError> {
    let encoded =
        encode_base64(format!("{}:{}", target.creds.user, target.creds.password).as_bytes());
    let auth = serde_json::json!({
        "auths": {
            target.registry: {
                "auth": encoded
            }
        }
    });
    tokio::fs::write(auth_path, auth.to_string())
        .await
        .map_err(ReceiveError::AuthFile)
}

fn encode_base64(input: &[u8]) -> String {
    const ALPHABET: &[u8; 64] = b"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
    let mut output = String::with_capacity(input.len().div_ceil(3) * 4);

    for chunk in input.chunks(3) {
        let first = chunk[0];
        let second = chunk.get(1).copied().unwrap_or(0);
        let third = chunk.get(2).copied().unwrap_or(0);

        output.push(ALPHABET[(first >> 2) as usize] as char);
        output.push(ALPHABET[(((first & 0x03) << 4) | (second >> 4)) as usize] as char);
        output.push(if chunk.len() > 1 {
            ALPHABET[(((second & 0x0f) << 2) | (third >> 6)) as usize] as char
        } else {
            '='
        });
        output.push(if chunk.len() > 2 {
            ALPHABET[(third & 0x3f) as usize] as char
        } else {
            '='
        });
    }

    output
}

async fn push_archive(
    header: &TransferHeader,
    archive_path: &Path,
    auth_path: &Path,
    target: &PushTarget<'_>,
) -> Result<(), ReceiveError> {
    let dest = format!("docker://{}/{}", target.registry, header.target_image_ref);
    let src = format!("oci-archive:{}", archive_path.display());
    let auth_file = auth_path.to_string_lossy();

    let mut args = vec!["copy", "--authfile", auth_file.as_ref()];
    if target.insecure {
        args.push("--dest-tls-verify=false");
    }
    args.push(&src);
    args.push(&dest);

    skopeo::run(&args, PUSH_TIMEOUT)
        .await
        .map_err(|e| match e.kind {
            SkopeoErrorKind::NotInstalled => ReceiveError::SkopeoNotInstalled,
            _ => ReceiveError::PushFailed {
                transfer_id: header.transfer_id,
                message: e.message,
            },
        })?;
    Ok(())
}

async fn inspect_pushed_image(
    header: &TransferHeader,
    auth_path: &Path,
    target: &PushTarget<'_>,
) -> Result<String, ReceiveError> {
    let reference = format!("docker://{}/{}", target.registry, header.target_image_ref);
    let auth_file = auth_path.to_string_lossy();
    let mut args = vec![
        "inspect",
        "--format",
        "{{.Digest}}",
        "--authfile",
        auth_file.as_ref(),
    ];
    if target.insecure {
        args.push("--tls-verify=false");
    }
    args.push(&reference);

    skopeo::run(&args, INSPECT_TIMEOUT)
        .await
        .map(|stdout| stdout.trim().to_string())
        .map_err(|e| match e.kind {
            SkopeoErrorKind::NotInstalled => ReceiveError::SkopeoNotInstalled,
            _ => ReceiveError::InspectFailed {
                transfer_id: header.transfer_id,
                message: e.message,
            },
        })
}

#[cfg(test)]
mod tests {
    use super::encode_base64;

    #[test]
    fn encodes_registry_credentials_for_auth_file() {
        assert_eq!(encode_base64(b"admin:password"), "YWRtaW46cGFzc3dvcmQ=");
    }
}
