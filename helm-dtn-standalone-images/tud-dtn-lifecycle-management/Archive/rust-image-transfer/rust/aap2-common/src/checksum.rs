//! SHA-256 checksums for transfer archives - shared by the sender
//! (computed right after export) and the receiver (verified before
//! anything touches Harbor).

use std::path::Path;

use sha2::{Digest, Sha256};
use tokio::fs::File;
use tokio::io::AsyncReadExt;

/// Computes the SHA-256 digest of a file, streaming it in chunks rather
/// than loading it fully into memory - image archives can be large.
pub async fn sha256_file(path: &Path) -> std::io::Result<String> {
    let mut file = File::open(path).await?;
    let mut hasher = Sha256::new();
    let mut buf = vec![0u8; 1024 * 1024];
    loop {
        let n = file.read(&mut buf).await?;
        if n == 0 {
            break;
        }
        hasher.update(&buf[..n]);
    }
    Ok(hex::encode(hasher.finalize()))
}

/// Computes the SHA-256 digest of an in-memory buffer.
pub fn sha256_bytes(data: &[u8]) -> String {
    hex::encode(Sha256::digest(data))
}
