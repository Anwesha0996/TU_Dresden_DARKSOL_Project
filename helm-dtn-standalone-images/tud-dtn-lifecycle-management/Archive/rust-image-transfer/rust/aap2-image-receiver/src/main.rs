//! Space-side receiver: receives a container image over AAP2, verifies
//! it, and pushes it into Harbor via skopeo.
//!
//! CLI shape mirrors the existing Go receiver (`pkg/helmdtnt/receiver.go`):
//! `--socket`/`--tcp` select the AAP2 transport (subscribing/listening
//! side, like `registerInstallAgent`), `--registry` is the same concept as
//! Go's `-r, --registry` / `REGISTRY_IP:REGISTRY_PORT`. Registry
//! credentials are read from the `HARBOR_CREDS` environment variable
//! (`user:password`) so secrets do not appear in `ps` output.
//!
//! Process resilience (Phase 6): a single failed transfer (bad payload,
//! checksum mismatch, push failure) is logged and the agent loop moves on
//! to the next message - it does not `panic()` and take the whole process
//! down, unlike the Go receiver's `registerInstallAgent`/etc., which was
//! explicitly flagged as a known issue (Phase 0 notes) and is deliberately
//! not replicated here. A connection-level failure (the AAP2 socket itself
//! erroring or timing out mid-read) does tear down and reconnect - a
//! partially-read stream can't be trusted to resume framing correctly -
//! but that reconnect loop itself never exits either.

mod receive;

use std::time::Duration;

use clap::{ArgGroup, Parser};

use aap2_common::client::{AnyClient, ConfigureOptions};
use aap2_common::proto::{aap_message, ResponseStatus};
use receive::{handle_transfer, PushTarget, RegistryCreds};

/// How long to wait for a bundle ADU's payload bytes once its header has
/// already arrived, before giving up on this connection and reconnecting.
/// By the time the header is in hand, the sender (Go or Rust) has already
/// committed to writing the payload immediately after it on the same
/// stream (see `aap2-common::client`'s module docs) - a long stall here
/// means something is actually wrong with the connection, not that a
/// large transfer is legitimately still in flight over a slow DTN link
/// (that latency is absorbed by µD3TN before the bundle is even handed to
/// this local AAP2 subscriber).
const ADU_READ_TIMEOUT: Duration = Duration::from_secs(120);

/// Delay before attempting to reconnect after the AAP2 connection is lost.
const RECONNECT_DELAY: Duration = Duration::from_secs(5);

/// Space-side receiver: receives a container image over AAP2 and pushes
/// it into Harbor via skopeo.
#[derive(Parser, Debug)]
#[command(name = "aap2-image-receiver", version, about)]
#[command(group(
    ArgGroup::new("transport")
        .required(true)
        .args(["socket", "tcp"]),
))]
struct Cli {
    /// Path to the UNIX domain socket created by µD3TN's AAP 2.0 service.
    #[arg(short = 's', long, value_name = "PATH")]
    socket: Option<String>,

    /// IP and port µD3TN listens for incoming AAP2 TCP connections on.
    #[arg(short = 't', long, value_name = "IP:PORT")]
    tcp: Option<String>,

    /// Agent ID to register as (this is the endpoint senders address
    /// bundles to).
    #[arg(long, value_name = "ID", default_value = "image-transfer")]
    agent_id: String,

    /// host:port of the registry (Harbor) received images are pushed to.
    #[arg(short = 'r', long, value_name = "HOST:PORT")]
    registry: String,

    /// Skip TLS verification against the registry. Only for local test
    /// Harbor instances reachable over plain HTTP (see
    /// deploy/local-test/harbor/harbor.yml) - never for a production target.
    #[arg(long, default_value_t = false)]
    insecure_registry: bool,
}

#[tokio::main]
async fn main() -> std::process::ExitCode {
    aap2_common::logging::init();
    let cli = Cli::parse();

    if let Err(e) = run(cli).await {
        tracing::error!(error = %e, "aap2-image-receiver exiting");
        eprintln!("aap2-image-receiver: {e}");
        return std::process::ExitCode::FAILURE;
    }
    std::process::ExitCode::SUCCESS
}

async fn run(cli: Cli) -> anyhow::Result<()> {
    let creds_raw = std::env::var("HARBOR_CREDS")
        .map_err(|_| anyhow::anyhow!("HARBOR_CREDS must be set to 'user:password'"))?;
    let (user, password) = creds_raw
        .trim()
        .split_once(':')
        .ok_or_else(|| anyhow::anyhow!("HARBOR_CREDS must contain 'user:password'"))?;
    let creds = RegistryCreds {
        user: user.to_string(),
        password: password.to_string(),
    };

    tracing::info!(
        agent_id = %cli.agent_id,
        registry = %cli.registry,
        "aap2-image-receiver starting"
    );

    let target = PushTarget {
        registry: &cli.registry,
        creds: &creds,
        insecure: cli.insecure_registry,
    };

    // Outer loop: (re)connect to µD3TN. A connection/stream-level error
    // from the inner loop lands here, waits briefly, and reconnects -
    // this loop itself never exits, which is what makes the receiver
    // resilient to the AAP2 link (not just individual transfers) dropping.
    loop {
        match run_connection(&cli, &target).await {
            Ok(()) => unreachable!("run_connection's inner loop only returns via Err"),
            Err(e) => {
                tracing::error!(error = %e, reconnect_in = ?RECONNECT_DELAY, "AAP2 connection lost, will reconnect");
                tokio::time::sleep(RECONNECT_DELAY).await;
            }
        }
    }
}

async fn run_connection(cli: &Cli, target: &PushTarget<'_>) -> anyhow::Result<()> {
    let mut client = AnyClient::connect(cli.socket.as_deref(), cli.tcp.as_deref()).await?;
    client
        .configure(
            ConfigureOptions::new()
                .agent_id(cli.agent_id.clone())
                .subscribe(true),
        )
        .await?;
    tracing::info!(eid = %client.eid(), "registered with µD3TN, waiting for transfers");

    loop {
        let msg = client.receive_msg().await?;

        let adu = match msg.msg {
            Some(aap_message::Msg::Adu(adu)) => adu,
            other => {
                tracing::warn!(?other, "received a non-ADU message, ignoring");
                continue;
            }
        };

        client.send_response_status(ResponseStatus::Success).await?;

        let src_eid = adu.src_eid.clone();
        let declared_len = adu.payload_length;
        tracing::info!(%src_eid, payload_bytes = declared_len, "receiving transfer payload");

        let payload = tokio::time::timeout(ADU_READ_TIMEOUT, client.receive_adu(Some(adu)))
            .await
            .map_err(|_| {
                anyhow::anyhow!(
                    "timed out after {ADU_READ_TIMEOUT:?} reading ADU payload from {src_eid}"
                )
            })??
            .1;

        if let Err(e) = handle_transfer(&payload, target).await {
            tracing::error!(error = %e, "image transfer failed; waiting for next transfer");
        }
    }
}
