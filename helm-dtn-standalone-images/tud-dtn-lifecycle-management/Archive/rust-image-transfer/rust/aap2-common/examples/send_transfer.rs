//! Sends a crafted image-transfer AAP2 payload to a given destination EID.
//! General-purpose tool backing the Phase 7 fault-injection test script
//! (`deploy/local-test/scripts/test-fault-injection.sh`), which needs
//! precise control over transfer headers (a fixed `transfer_id` for
//! duplicate-delivery tests, a deliberately wrong checksum for
//! corruption tests) that `aap2-image-sender` itself doesn't expose - it
//! always computes the real checksum and generates a fresh UUID.
//!
//! Usage:
//!   send_transfer --tcp ADDR --dest EID
//!     [--transfer-id UUID]              (default: random)
//!     [--source-image-ref REF]          (default: docker.io/library/alpine:3.20)
//!     [--target-image-ref REF]          (default: library/fault-test:latest)
//!     [--archive-sha256 HEX|auto]       (default: auto = real sha256 of the archive)
//!     [--archive-file PATH]             (default: a small placeholder archive)
//!
//! Prints `transfer_id=<uuid>` and `response_status=<n>` on stdout.

use aap2_common::checksum::sha256_bytes;
use aap2_common::client::{Aap2Client, ConfigureOptions};
use aap2_common::transfer::{encode_transfer_payload, TransferHeader};
use uuid::Uuid;

fn arg(args: &[String], name: &str, default: Option<&str>) -> String {
    args.iter()
        .position(|a| a == name)
        .and_then(|i| args.get(i + 1))
        .cloned()
        .or_else(|| default.map(String::from))
        .unwrap_or_else(|| panic!("missing required argument {name}"))
}

fn opt_arg(args: &[String], name: &str) -> Option<String> {
    args.iter()
        .position(|a| a == name)
        .and_then(|i| args.get(i + 1))
        .cloned()
}

#[tokio::main]
async fn main() {
    let args: Vec<String> = std::env::args().collect();

    let tcp = arg(&args, "--tcp", None);
    let dest = arg(&args, "--dest", None);
    let transfer_id: Uuid = opt_arg(&args, "--transfer-id")
        .map(|s| s.parse().expect("--transfer-id must be a valid UUID"))
        .unwrap_or_else(Uuid::new_v4);
    let source_image_ref = arg(
        &args,
        "--source-image-ref",
        Some("docker.io/library/alpine:3.20"),
    );
    let target_image_ref = arg(
        &args,
        "--target-image-ref",
        Some("library/fault-test:latest"),
    );

    let archive = match opt_arg(&args, "--archive-file") {
        Some(path) => std::fs::read(&path).unwrap_or_else(|e| panic!("failed to read {path}: {e}")),
        None => b"placeholder archive bytes for a fault-injection test".to_vec(),
    };

    let archive_sha256 = match opt_arg(&args, "--archive-sha256").as_deref() {
        Some("auto") | None => sha256_bytes(&archive),
        Some(hex) => hex.to_string(),
    };

    let header = TransferHeader {
        transfer_id,
        source_image_ref,
        target_image_ref,
        created_at_unix: 0,
        archive_sha256,
        archive_size_bytes: archive.len() as u64,
        release_name: opt_arg(&args, "--release").unwrap_or_default(),
    };
    let payload = encode_transfer_payload(&header, &archive);

    let mut client = Aap2Client::connect_tcp(&tcp)
        .await
        .unwrap_or_else(|e| panic!("connect to {tcp}: {e}"));
    client
        .configure(ConfigureOptions::new().agent_id(format!("send-transfer-{transfer_id}")))
        .await
        .expect("configure");
    client.send_adu(&dest, &payload).await.expect("send_adu");
    let resp = client.receive_response().await.expect("receive_response");

    println!("transfer_id={transfer_id}");
    println!("response_status={}", resp.response_status);
}
