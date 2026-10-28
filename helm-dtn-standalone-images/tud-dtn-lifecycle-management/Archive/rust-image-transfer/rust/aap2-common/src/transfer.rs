//! The transfer header sent as the first frame of every image-transfer
//! AAP2 payload, so the receiver knows what it received without any
//! out-of-band coordination - the space-side link may be one-way and
//! can't ask questions back (TASKS.md Phase 4).
//!
//! Wire layout of an image-transfer ADU payload (this is all inside a
//! single AAP2 `BundleADU`'s payload - see `aap2-common::client`, this
//! module doesn't touch AAP2 framing at all):
//!
//! ```text
//! [8 bytes: header_len, big-endian u64]
//! [header_len bytes: JSON-encoded TransferHeader]
//! [remaining bytes: OCI archive tar data]
//! ```

use serde::{Deserialize, Serialize};
use uuid::Uuid;

/// Metadata describing one image transfer, sent ahead of the archive
/// bytes so the receiver can identify, verify, and route it without
/// needing to ask the sender anything.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct TransferHeader {
    /// Unique ID for this transfer. Carried through logs on both ends so a
    /// transfer's lifecycle can be correlated without persistent receiver
    /// state.
    pub transfer_id: Uuid,
    /// The image reference it was exported from (e.g. `docker.io/library/alpine:3.20`).
    pub source_image_ref: String,
    /// The reference/tag it should be pushed to in the destination registry
    /// (e.g. `library/alpine:3.20`).
    pub target_image_ref: String,
    /// Unix timestamp (seconds) of when the archive was created.
    pub created_at_unix: u64,
    /// SHA-256 of the OCI archive, hex-encoded. Computed by the sender
    /// immediately after export, verified by the receiver before it
    /// touches Harbor.
    pub archive_sha256: String,
    /// Size of the OCI archive in bytes.
    pub archive_size_bytes: u64,
    /// The Helm release this image belongs to (TASKS-PART-2.md Phase B) -
    /// purely informational/for correlating logs with a specific `helm dtn
    /// install`/`upgrade` invocation; the receiver doesn't act on it.
    /// Empty string if not supplied (e.g. when sending outside the Go CLI
    /// wiring, such as in tests).
    #[serde(default)]
    pub release_name: String,
}

#[derive(thiserror::Error, Debug)]
pub enum FrameError {
    #[error("payload too short to contain a header length prefix (need 8 bytes, got {0})")]
    TooShortForLengthPrefix(usize),
    #[error("declared header length ({declared}) exceeds remaining payload size ({remaining})")]
    HeaderLengthExceedsPayload { declared: u64, remaining: usize },
    #[error("failed to parse transfer header JSON: {0}")]
    InvalidJson(#[from] serde_json::Error),
}

/// Builds the `[header_len][header_json][archive_bytes]` payload described
/// in the module docs.
pub fn encode_transfer_payload(header: &TransferHeader, archive: &[u8]) -> Vec<u8> {
    let header_json = serde_json::to_vec(header).expect("TransferHeader always serializes");
    let mut out = Vec::with_capacity(8 + header_json.len() + archive.len());
    out.extend_from_slice(&(header_json.len() as u64).to_be_bytes());
    out.extend_from_slice(&header_json);
    out.extend_from_slice(archive);
    out
}

/// Splits a received payload back into its `TransferHeader` and the
/// archive bytes that follow it. The returned archive slice borrows from
/// `payload`, so callers that need an owned copy should clone/write it out
/// explicitly (the receiver writes it straight to a temp file rather than
/// copying it in memory again - see Phase 5).
pub fn decode_transfer_payload(payload: &[u8]) -> Result<(TransferHeader, &[u8]), FrameError> {
    if payload.len() < 8 {
        return Err(FrameError::TooShortForLengthPrefix(payload.len()));
    }
    let header_len = u64::from_be_bytes(payload[0..8].try_into().unwrap());
    let remaining = payload.len() - 8;
    if header_len > remaining as u64 {
        return Err(FrameError::HeaderLengthExceedsPayload {
            declared: header_len,
            remaining,
        });
    }
    let header_end = 8 + header_len as usize;
    let header: TransferHeader = serde_json::from_slice(&payload[8..header_end])?;
    Ok((header, &payload[header_end..]))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn round_trips_header_and_archive() {
        let header = TransferHeader {
            transfer_id: Uuid::new_v4(),
            source_image_ref: "docker.io/library/alpine:3.20".to_string(),
            target_image_ref: "library/alpine:3.20".to_string(),
            created_at_unix: 1_700_000_000,
            archive_sha256: "a".repeat(64),
            archive_size_bytes: 42,
            release_name: "demo-release".to_string(),
        };
        let archive = b"not a real tar, just archive bytes for the test";
        let payload = encode_transfer_payload(&header, archive);
        let (decoded_header, decoded_archive) = decode_transfer_payload(&payload).unwrap();
        assert_eq!(decoded_header, header);
        assert_eq!(decoded_archive, archive);
    }

    #[test]
    fn rejects_truncated_length_prefix() {
        let err = decode_transfer_payload(&[1, 2, 3]).unwrap_err();
        assert!(matches!(err, FrameError::TooShortForLengthPrefix(3)));
    }

    #[test]
    fn rejects_header_length_exceeding_payload() {
        let mut payload = 999_999u64.to_be_bytes().to_vec();
        payload.extend_from_slice(b"short");
        let err = decode_transfer_payload(&payload).unwrap_err();
        assert!(matches!(err, FrameError::HeaderLengthExceedsPayload { .. }));
    }
}
