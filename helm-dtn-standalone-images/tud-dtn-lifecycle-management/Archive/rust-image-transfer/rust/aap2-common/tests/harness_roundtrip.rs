//! Integration test (TASKS.md Phase 3): a Rust sender and a Rust receiver,
//! talking real AAP2 over TCP to the real two-node µD3TN harness from
//! `deploy/local-test/` (earth-node / space-node, connected via tcpclv3),
//! sending raw arbitrary bytes - not yet an image (that's Phase 4/5).
//!
//! Ignored by default (`#[ignore]`) since it requires that harness to
//! already be up: `docker compose up -d` in `deploy/local-test/`. Run
//! explicitly with:
//!
//! ```sh
//! cargo test -p aap2-common --test harness_roundtrip -- --ignored --nocapture
//! ```
//!
//! Assumes the host-mapped AAP2 ports from `deploy/local-test/docker-compose.yml`:
//! earth-node -> 127.0.0.1:14242, space-node -> 127.0.0.1:24242.

use std::process::Command;

use aap2_common::client::Aap2Client;
use aap2_common::client::ConfigureOptions;
use aap2_common::proto::ResponseStatus;
use sha2::{Digest, Sha256};

const EARTH_AAP2: &str = "127.0.0.1:14242";
const SPACE_AAP2: &str = "127.0.0.1:24242";

/// (Re-)configures the earth-node -> space-node contact via µD3TN's own
/// `aap2-config` tool inside the earth-node container - same mechanism as
/// `deploy/local-test/scripts/configure-contact.sh`. Contacts are
/// runtime-only (not persisted), so this needs to run before every test
/// invocation rather than being assumed still active from a prior manual
/// run - see doc/rust-image-transfer/00-aap2-notes.md.
/// Configures the contact at most once per test-binary run - see the
/// detailed explanation in the Rust image receiver integration tests:
/// re-running `aap2-config` once per test accumulates distinct contact
/// entries in µD3TN and eventually breaks delivery entirely ("too many
/// contacts are already active").
fn configure_contact() {
    static ONCE: std::sync::Once = std::sync::Once::new();
    ONCE.call_once(|| {
        let status = Command::new("docker")
            .args([
                "exec",
                "earth-node",
                "aap2-config",
                "--tcp",
                "127.0.0.1",
                "4242",
                "-s",
                "1",
                "3600",
                "100000",
                "dtn://space.dtn/",
                "tcpclv3:space-node:4556",
            ])
            .status()
            .expect("failed to run `docker exec earth-node aap2-config` - is the Phase 1 harness up? (docker compose up -d in deploy/local-test/)");
        assert!(status.success(), "aap2-config exited non-zero");
        std::thread::sleep(std::time::Duration::from_millis(5000));
    });
}

async fn roundtrip(agent_suffix: &str, payload_size: usize) {
    configure_contact();

    let dest_agent = format!("rust-harness-test-{agent_suffix}");
    let dest_eid = format!("dtn://space.dtn/{dest_agent}");

    let mut receiver = Aap2Client::connect_tcp(SPACE_AAP2)
        .await
        .expect("connect to space-node AAP2");
    receiver
        .configure(
            ConfigureOptions::new()
                .agent_id(dest_agent.clone())
                .subscribe(true),
        )
        .await
        .expect("configure receiver");

    let mut sender = Aap2Client::connect_tcp(EARTH_AAP2)
        .await
        .expect("connect to earth-node AAP2");
    sender
        .configure(ConfigureOptions::new().agent_id(format!("rust-sender-{agent_suffix}")))
        .await
        .expect("configure sender");

    let payload: Vec<u8> = (0..payload_size).map(|i| (i % 256) as u8).collect();
    let expected_sha256 = hex::encode(Sha256::digest(&payload));

    sender
        .send_adu(&dest_eid, &payload)
        .await
        .expect("send_adu");
    let send_response = sender
        .receive_response()
        .await
        .expect("receive send response");
    assert_eq!(
        ResponseStatus::try_from(send_response.response_status).unwrap(),
        ResponseStatus::Success,
        "µD3TN rejected the send"
    );

    let msg = receiver
        .receive_msg()
        .await
        .expect("receive_msg on receiver");
    receiver
        .send_response_status(ResponseStatus::Success)
        .await
        .expect("ack the ADU message");
    let adu = match msg.msg {
        Some(aap2_common::proto::aap_message::Msg::Adu(adu)) => adu,
        other => panic!("expected BundleADU, got {other:?}"),
    };
    let (_, got_payload) = receiver
        .receive_adu(Some(adu))
        .await
        .expect("receive_adu payload");

    assert_eq!(got_payload.len(), payload_size);
    let got_sha256 = hex::encode(Sha256::digest(&got_payload));
    assert_eq!(
        got_sha256, expected_sha256,
        "byte-for-byte integrity check failed for a {payload_size}-byte payload"
    );
}

#[tokio::test]
#[ignore]
async fn small_payload_roundtrips_over_real_harness() {
    roundtrip("small", 1024).await;
}

/// Deliberately larger than a single AAP2 ADU/TCP-segment read would cover,
/// to confirm reassembly over a REAL socket (not just the in-memory duplex
/// pipe used in client.rs's unit tests) - this is the "don't assume
/// small-payload testing generalizes" acceptance criterion from Phase 3.
#[tokio::test]
#[ignore]
async fn large_payload_roundtrips_over_real_harness() {
    roundtrip("large", 8 * 1024 * 1024).await;
}
