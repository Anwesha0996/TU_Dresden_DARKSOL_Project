//! Earth-side sender: exports a container image with `skopeo`, attaches
//! a minimal transfer header, and pushes it through µD3TN via AAP2.
//!
//! CLI shape mirrors the existing Go `helm dtn install`/`upgrade` flags
//! (`pkg/aap2_client`, `cmd/install.go`) where it makes sense: `--socket`/
//! `--tcp` select the AAP2 transport exactly like the Go client's
//! `AAP2UnixClient`/`AAP2TCPClient`, and `--destination-eid` is the same
//! concept as Go's `-d, --destination-eid`.

mod export;

use std::path::PathBuf;
use std::process::ExitCode;
use std::time::Duration;

use clap::{ArgGroup, Parser};
use uuid::Uuid;

use aap2_common::client::{AnyClient, ConfigureOptions};
use aap2_common::proto::ResponseStatus;
use aap2_common::transfer::encode_transfer_payload;
use export::{export_image, read_archive, ExportError};

/// Earth-side sender: exports a container image and pushes it through
/// µD3TN via AAP2.
#[derive(Parser, Debug)]
#[command(name = "aap2-image-sender", version, about)]
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
    /// Example: 127.0.0.1:4242.
    #[arg(short = 't', long, value_name = "IP:PORT")]
    tcp: Option<String>,

    /// EID of the receiving agent (aap2-image-receiver) in the remote cluster.
    #[arg(short = 'd', long, value_name = "EID")]
    destination_eid: String,

    /// Agent ID to register as when connecting to µD3TN.
    #[arg(long, value_name = "ID", default_value = "image-transfer")]
    agent_id: String,

    /// Image reference to export, e.g. `docker.io/library/alpine:3.20`.
    #[arg(long, value_name = "REF")]
    image: String,

    /// Target reference/tag in the destination registry, e.g.
    /// `library/alpine:3.20`.
    #[arg(long, value_name = "PROJECT/NAME:TAG")]
    harbor_target: String,

    /// Directory to export the OCI archive into. Defaults to a fresh
    /// temp directory.
    #[arg(long, value_name = "DIR")]
    work_dir: Option<PathBuf>,

    /// Target architecture to export (the receiving cluster's
    /// architecture, not necessarily this machine's - see export.rs).
    #[arg(long, value_name = "ARCH", default_value = "amd64")]
    target_arch: String,

    /// Helm release associated with the image, used for receiver log
    /// correlation.
    #[arg(long, value_name = "NAME", default_value = "")]
    release: String,
}

#[tokio::main]
async fn main() -> ExitCode {
    aap2_common::logging::init();
    let cli = Cli::parse();
    let transfer_id = Uuid::new_v4();

    match run(cli, transfer_id).await {
        Ok(()) => ExitCode::SUCCESS,
        Err(e) => {
            tracing::error!(transfer_id = %transfer_id, error = %e, "aap2-image-sender failed");
            eprintln!("aap2-image-sender: {e}");
            ExitCode::FAILURE
        }
    }
}

async fn run(cli: Cli, transfer_id: Uuid) -> anyhow::Result<()> {
    tracing::info!(
        transfer_id = %transfer_id,
        image = %cli.image,
        harbor_target = %cli.harbor_target,
        destination_eid = %cli.destination_eid,
        release = %cli.release,
        "aap2-image-sender starting"
    );

    let temp_work_dir = if cli.work_dir.is_none() {
        Some(
            tempfile::Builder::new()
                .prefix("aap2-image-sender-")
                .tempdir()?,
        )
    } else {
        None
    };
    let work_dir = cli.work_dir.unwrap_or_else(|| {
        temp_work_dir
            .as_ref()
            .expect("temporary work directory was just created")
            .path()
            .to_path_buf()
    });

    let exported = export_image(
        &cli.image,
        &cli.harbor_target,
        &cli.target_arch,
        &work_dir,
        transfer_id,
        &cli.release,
    )
    .await
    .map_err(explain_export_error)?;

    tracing::info!(
        transfer_id = %transfer_id,
        sha256 = %exported.header.archive_sha256,
        size_bytes = exported.header.archive_size_bytes,
        "export complete, reading archive for transmission"
    );

    let archive_bytes = read_archive(&exported.archive_path).await?;
    let payload = encode_transfer_payload(&exported.header, &archive_bytes);

    tracing::info!(
        transfer_id = %transfer_id,
        payload_bytes = payload.len(),
        "connecting to µD3TN"
    );

    let mut client = AnyClient::connect(cli.socket.as_deref(), cli.tcp.as_deref()).await?;
    client
        .configure(ConfigureOptions::new().agent_id(cli.agent_id))
        .await?;

    tracing::info!(transfer_id = %transfer_id, "sending transfer ADU");
    client.send_adu(&cli.destination_eid, &payload).await?;

    let response = tokio::time::timeout(Duration::from_secs(30), client.receive_response())
        .await
        .map_err(|_| anyhow::anyhow!("timed out waiting for µD3TN to acknowledge the send"))??;
    let status =
        ResponseStatus::try_from(response.response_status).unwrap_or(ResponseStatus::Unspecified);
    if status != ResponseStatus::Success {
        anyhow::bail!("µD3TN rejected the send: {status:?}");
    }

    tracing::info!(
        transfer_id = %transfer_id,
        source_image = %exported.header.source_image_ref,
        target_image = %exported.header.target_image_ref,
        sha256 = %exported.header.archive_sha256,
        "transfer accepted by µD3TN"
    );
    println!("transfer_id={transfer_id}");
    println!("sha256={}", exported.header.archive_sha256);

    Ok(())
}

/// Turns an [`ExportError`] into a top-level error message that's
/// immediately actionable - Phase 4's explicit requirement that "bad image
/// ref", "skopeo missing", and "disk full" each read distinctly rather
/// than as an opaque failure.
fn explain_export_error(e: ExportError) -> anyhow::Error {
    let hint = match &e {
        ExportError::SkopeoNotInstalled => {
            "install skopeo (e.g. `brew install skopeo` / `apt install skopeo`) and retry"
        }
        ExportError::AuthOrImageNotFound { .. } => {
            "check the image reference is correct and, for private images, that credentials are configured (e.g. `skopeo login`)"
        }
        ExportError::DiskFull { .. } => "free up disk space in the working directory and retry",
        ExportError::Timeout { .. } => {
            "the registry may be slow or unreachable; retry, or check network connectivity"
        }
        _ => "",
    };
    if hint.is_empty() {
        anyhow::anyhow!(e)
    } else {
        anyhow::anyhow!("{e} ({hint})")
    }
}
