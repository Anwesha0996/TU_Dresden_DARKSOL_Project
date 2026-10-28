//! Async AAP2 client, supporting both TCP and UNIX-socket transport.
//!
//! Mirrors the existing Go client (`pkg/aap2_client/*.go`) faithfully:
//! same handshake (magic byte 0x2f + WELCOME), same length-delimited
//! protobuf framing, same EID-derivation and secret/agent-id generation
//! rules in `configure()`. See
//! `../../../doc/rust-image-transfer/00-aap2-notes.md` for the full
//! protocol writeup this was implemented against.
//!
//! Deliberately different from the Go client in one place: `receive_adu`
//! here loops (`AsyncReadExt::read_exact`) until the full declared payload
//! length has been read, rather than trusting a single `read()` call to
//! fill the buffer. That single-`Read()` assumption is a real bug in the
//! Go client (found while building the Phase 1 test harness, fixed there
//! via `io.ReadFull`); the async equivalent is `read_exact`, used here from
//! the start.

use std::path::Path;

use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::{TcpStream, ToSocketAddrs, UnixStream};
use uuid::Uuid;

use crate::error::{Aap2Error, Result};
use crate::proto::{
    aap_message, AapMessage, AapResponse, AuthType, BundleAdu, ConnectionConfig, ResponseStatus,
    Welcome,
};

/// The first byte µD3TN always sends on a new AAP2 connection.
const MAGIC_BYTE: u8 = 0x2f;

/// Maximum number of bytes a protobuf varint can occupy (matches the Go
/// client's `PROTOBUF_VARINT_MAX_BYTES`).
const VARINT_MAX_BYTES: usize = 10;

/// Options for [`Aap2Client::configure`], mirroring the Go client's
/// functional options (`WithAgentId`, `WithSubscribe`, `WithSecret`,
/// `WithAuthType`, `WithKeepalive`).
#[derive(Debug, Clone)]
pub struct ConfigureOptions {
    pub agent_id: Option<String>,
    pub subscribe: bool,
    pub secret: Option<String>,
    pub auth_type: AuthType,
    pub keepalive_seconds: u32,
}

impl Default for ConfigureOptions {
    fn default() -> Self {
        Self {
            agent_id: None,
            subscribe: false,
            secret: None,
            auth_type: AuthType::Default,
            keepalive_seconds: 0,
        }
    }
}

impl ConfigureOptions {
    pub fn new() -> Self {
        Self::default()
    }

    pub fn agent_id(mut self, id: impl Into<String>) -> Self {
        self.agent_id = Some(id.into());
        self
    }

    pub fn subscribe(mut self, subscribe: bool) -> Self {
        self.subscribe = subscribe;
        self
    }

    pub fn secret(mut self, secret: impl Into<String>) -> Self {
        self.secret = Some(secret.into());
        self
    }

    pub fn auth_type(mut self, auth_type: AuthType) -> Self {
        self.auth_type = auth_type;
        self
    }

    pub fn keepalive_seconds(mut self, seconds: u32) -> Self {
        self.keepalive_seconds = seconds;
        self
    }
}

fn is_ipn_eid(node_eid: &str) -> bool {
    node_eid.starts_with("ipn")
}

fn eid_prefix(node_eid: &str) -> &str {
    if is_ipn_eid(node_eid) {
        node_eid.split('.').next().unwrap_or(node_eid)
    } else {
        node_eid
    }
}

fn full_eid(node_eid: &str, agent_id: &str) -> String {
    if is_ipn_eid(node_eid) {
        format!("{}.{}", eid_prefix(node_eid), agent_id)
    } else {
        format!("{}{}", eid_prefix(node_eid), agent_id)
    }
}

/// Generates a random agent ID, matching the Go client's
/// `generateAgentId`: a UUID for `dtn:` EIDs, a random positive number
/// (here derived from a UUID's bytes rather than pulling in a `rand`
/// dependency purely for this one uncommon path - both node harnesses used
/// throughout this project use `dtn:` EIDs) for `ipn:` EIDs.
fn generate_agent_id(node_eid: &str) -> String {
    if node_eid.is_empty() {
        return String::new();
    }
    if is_ipn_eid(node_eid) {
        let bytes = Uuid::new_v4().into_bytes();
        let n = u32::from_be_bytes([bytes[0], bytes[1], bytes[2], bytes[3]]);
        (n.max(1)).to_string()
    } else {
        Uuid::new_v4().to_string()
    }
}

/// An AAP2 client connected over some async, ordered, reliable byte stream
/// (TCP or a UNIX domain socket).
pub struct Aap2Client<S> {
    stream: S,
    node_eid: String,
    agent_id: String,
}

impl Aap2Client<TcpStream> {
    /// Connects to µD3TN's AAP 2.0 TCP socket at `addr` and performs the
    /// initial handshake (magic byte + WELCOME).
    pub async fn connect_tcp(addr: impl ToSocketAddrs) -> Result<Self> {
        let stream = TcpStream::connect(addr).await?;
        Self::handshake(stream).await
    }
}

impl Aap2Client<UnixStream> {
    /// Connects to µD3TN's AAP 2.0 UNIX domain socket at `path` and
    /// performs the initial handshake (magic byte + WELCOME).
    pub async fn connect_unix(path: impl AsRef<Path>) -> Result<Self> {
        let stream = UnixStream::connect(path).await?;
        Self::handshake(stream).await
    }
}

impl<S: AsyncReadExt + AsyncWriteExt + Unpin> Aap2Client<S> {
    async fn handshake(mut stream: S) -> Result<Self> {
        let mut magic = [0u8; 1];
        match stream.read_exact(&mut magic).await {
            Ok(_) => {}
            Err(e) if e.kind() == std::io::ErrorKind::UnexpectedEof => {
                return Err(Aap2Error::ConnectionClosedDuringHandshake)
            }
            Err(e) => return Err(e.into()),
        }
        if magic[0] != MAGIC_BYTE {
            return Err(Aap2Error::BadMagicByte(magic[0]));
        }

        let mut client = Aap2Client {
            stream,
            node_eid: String::new(),
            agent_id: String::new(),
        };

        let msg = client.receive_msg().await?;
        let welcome: Welcome = match msg.msg {
            Some(aap_message::Msg::Welcome(w)) => w,
            other => {
                return Err(Aap2Error::UnexpectedMessage {
                    expected: "Welcome",
                    got: format!("{other:?}"),
                })
            }
        };
        client.node_eid = welcome.node_id;
        tracing::debug!(node_eid = %client.node_eid, "received WELCOME");
        Ok(client)
    }

    /// The full EID this client is/will be registered as (node EID prefix +
    /// agent ID), once configured.
    pub fn eid(&self) -> String {
        full_eid(&self.node_eid, &self.agent_id)
    }

    /// The remote node's EID, as announced in the WELCOME message.
    pub fn node_eid(&self) -> &str {
        &self.node_eid
    }

    /// Registers/authenticates this connection with µD3TN. Returns the
    /// secret that can be used by another connection to resume/claim the
    /// same registration.
    pub async fn configure(&mut self, opts: ConfigureOptions) -> Result<String> {
        self.agent_id = match opts.agent_id {
            Some(id) if !id.is_empty() => id,
            _ => generate_agent_id(&self.node_eid),
        };
        let endpoint_id = full_eid(&self.node_eid, &self.agent_id);
        let secret = opts
            .secret
            .filter(|s| !s.is_empty())
            .unwrap_or_else(|| Uuid::new_v4().to_string());

        let config = ConnectionConfig {
            is_subscriber: opts.subscribe,
            auth_type: opts.auth_type as i32,
            secret: secret.clone(),
            endpoint_id,
            keepalive_seconds: opts.keepalive_seconds,
        };
        tracing::debug!(agent_id = %self.agent_id, subscribe = opts.subscribe, "sending CONFIGURE");
        self.send_msg(&AapMessage {
            msg: Some(aap_message::Msg::Config(config)),
        })
        .await?;

        let response = self.receive_response().await?;
        check_success(&response)?;
        Ok(secret)
    }

    /// Sends a bundle ADU: `payload` addressed to `dst_eid`.
    pub async fn send_adu(&mut self, dst_eid: &str, payload: &[u8]) -> Result<()> {
        let adu = BundleAdu {
            src_eid: String::new(),
            dst_eid: dst_eid.to_string(),
            report_to_eid: String::new(),
            creation_timestamp_ms: 0,
            sequence_number: 0,
            payload_length: payload.len() as u64,
            adu_flags: Vec::new(),
        };
        self.send_msg(&AapMessage {
            msg: Some(aap_message::Msg::Adu(adu)),
        })
        .await?;
        self.stream.write_all(payload).await?;
        Ok(())
    }

    /// Receives a bundle ADU's payload. If `adu` is `None`, first calls
    /// [`receive_msg`](Self::receive_msg) and requires the result to be a
    /// `BundleADU` variant; otherwise uses the already-received header as
    /// is, matching the Go client's `ReceiveAdu(aduMsg *aap2.BundleADU)`.
    /// This lets a caller inspect/validate the header, and send its
    /// `AAPResponse` ack, before committing to read a possibly-large
    /// payload. Loops via `read_exact` until the full declared payload has
    /// been read; see the module-level doc comment for why that matters.
    pub async fn receive_adu(&mut self, adu: Option<BundleAdu>) -> Result<(BundleAdu, Vec<u8>)> {
        let adu = match adu {
            Some(adu) => adu,
            None => {
                let msg = self.receive_msg().await?;
                match msg.msg {
                    Some(aap_message::Msg::Adu(adu)) => adu,
                    other => {
                        return Err(Aap2Error::UnexpectedMessage {
                            expected: "BundleADU",
                            got: format!("{other:?}"),
                        })
                    }
                }
            }
        };
        let mut payload = vec![0u8; adu.payload_length as usize];
        self.stream.read_exact(&mut payload).await?;
        Ok((adu, payload))
    }

    /// Receives and returns the next `AAPMessage`.
    pub async fn receive_msg(&mut self) -> Result<AapMessage> {
        self.receive_delimited().await
    }

    /// Receives and returns the next `AAPResponse`.
    pub async fn receive_response(&mut self) -> Result<AapResponse> {
        self.receive_delimited().await
    }

    /// Sends a bare `AAPResponse` with the given status - used to
    /// acknowledge a received message.
    pub async fn send_response_status(&mut self, status: ResponseStatus) -> Result<()> {
        self.send_msg(&AapResponse {
            response_status: status as i32,
            dispatch_result: None,
            bundle_headers: None,
        })
        .await
    }

    async fn send_msg(&mut self, msg: &impl prost::Message) -> Result<()> {
        let buf = msg.encode_length_delimited_to_vec();
        self.stream.write_all(&buf).await?;
        Ok(())
    }

    async fn receive_delimited<M: prost::Message + Default>(&mut self) -> Result<M> {
        let len = self.read_varint().await?;
        let mut buf = vec![0u8; len as usize];
        self.stream.read_exact(&mut buf).await?;
        Ok(M::decode(buf.as_slice())?)
    }

    /// Reads a protobuf varint one byte at a time (matches the Go client's
    /// `receiveDelimited`/aap2-rs's `read_varint`: no `prost` API reads a
    /// varint directly off an async stream, since decoding is normally
    /// done on an already-buffered slice).
    async fn read_varint(&mut self) -> Result<u64> {
        let mut value: u64 = 0;
        let mut shift = 0u32;
        for _ in 0..VARINT_MAX_BYTES {
            let mut byte = [0u8; 1];
            self.stream.read_exact(&mut byte).await?;
            let b = byte[0];
            value |= ((b & 0x7f) as u64) << shift;
            if b & 0x80 == 0 {
                return Ok(value);
            }
            shift += 7;
        }
        Err(Aap2Error::Decode(prost::DecodeError::new(
            "invalid varint: exceeded maximum length",
        )))
    }
}

fn check_success(response: &AapResponse) -> Result<()> {
    let status =
        ResponseStatus::try_from(response.response_status).unwrap_or(ResponseStatus::Unspecified);
    if status != ResponseStatus::Success {
        return Err(Aap2Error::ResponseNotSuccess(status));
    }
    Ok(())
}

/// An AAP2 client connected over either transport, for callers (both
/// binaries take `--socket`/`--tcp` and don't know at compile time which
/// one they'll get) that would otherwise need to duplicate their whole
/// call sequence in a `match` just to pick a stream type - `Aap2Client<S>`
/// is generic over `S`, so a bare `match` producing either variant doesn't
/// unify without this wrapper.
pub enum AnyClient {
    Tcp(Aap2Client<TcpStream>),
    Unix(Aap2Client<UnixStream>),
}

impl AnyClient {
    /// Connects using whichever of `socket`/`tcp` is `Some`. Exactly one
    /// must be set - mirrors the `--socket`/`--tcp` `ArgGroup` both
    /// binaries' CLIs enforce.
    pub async fn connect(socket: Option<&str>, tcp: Option<&str>) -> Result<Self> {
        match (socket, tcp) {
            (Some(path), _) => Ok(Self::Unix(Aap2Client::connect_unix(path).await?)),
            (None, Some(addr)) => Ok(Self::Tcp(Aap2Client::connect_tcp(addr).await?)),
            (None, None) => panic!(
                "AnyClient::connect called with neither socket nor tcp set - this is a caller bug, not a runtime condition (the CLI's ArgGroup should have already required one)"
            ),
        }
    }

    pub fn eid(&self) -> String {
        match self {
            Self::Tcp(c) => c.eid(),
            Self::Unix(c) => c.eid(),
        }
    }

    pub fn node_eid(&self) -> &str {
        match self {
            Self::Tcp(c) => c.node_eid(),
            Self::Unix(c) => c.node_eid(),
        }
    }

    pub async fn configure(&mut self, opts: ConfigureOptions) -> Result<String> {
        match self {
            Self::Tcp(c) => c.configure(opts).await,
            Self::Unix(c) => c.configure(opts).await,
        }
    }

    pub async fn send_adu(&mut self, dst_eid: &str, payload: &[u8]) -> Result<()> {
        match self {
            Self::Tcp(c) => c.send_adu(dst_eid, payload).await,
            Self::Unix(c) => c.send_adu(dst_eid, payload).await,
        }
    }

    pub async fn receive_adu(&mut self, adu: Option<BundleAdu>) -> Result<(BundleAdu, Vec<u8>)> {
        match self {
            Self::Tcp(c) => c.receive_adu(adu).await,
            Self::Unix(c) => c.receive_adu(adu).await,
        }
    }

    pub async fn receive_msg(&mut self) -> Result<AapMessage> {
        match self {
            Self::Tcp(c) => c.receive_msg().await,
            Self::Unix(c) => c.receive_msg().await,
        }
    }

    pub async fn receive_response(&mut self) -> Result<AapResponse> {
        match self {
            Self::Tcp(c) => c.receive_response().await,
            Self::Unix(c) => c.receive_response().await,
        }
    }

    pub async fn send_response_status(&mut self, status: ResponseStatus) -> Result<()> {
        match self {
            Self::Tcp(c) => c.send_response_status(status).await,
            Self::Unix(c) => c.send_response_status(status).await,
        }
    }
}

/// Wire-format fixtures captured from the **existing Go client**
/// (`pkg/aap2_client`), not re-derived from the protobuf spec. Regenerate
/// with:
///
/// ```sh
/// docker run --rm -v "$(pwd)":/src -w /src golang:1.24 \
///   go run ./rust/aap2-common/testdata/gen_fixtures
/// ```
///
/// (run from the repo root). See `testdata/gen_fixtures/main.go`. The
/// point of testing against these rather than just testing "does our own
/// encoder round-trip through our own decoder" is to catch the case where
/// the Rust and Go implementations silently agree with each other but
/// disagree with the real protocol - these tests fail if that happens.
#[cfg(test)]
mod go_fixture_tests {
    use super::*;
    use tokio::io::{AsyncReadExt, AsyncWriteExt};

    const CONFIG_MSG_HEX: &str = "37123508011a12666978747572652d7365637265742d313233221d64746e3a2f2f65617274682e64746e2f666978747572652d6167656e74";
    const RESPONSE_SUCCESS_HEX: &str = "020801";
    const ADU_MSG_HEX: &str = "1b1a19121464746e3a2f2f73706163652e64746e2f73696e6b28b960";
    const WELCOME_MSG_HEX: &str = "140a120a1064746e3a2f2f65617274682e64746e2f";

    fn decode_hex(s: &str) -> Vec<u8> {
        hex::decode(s).expect("fixture hex is malformed")
    }

    /// Drives the client-side handshake against a fixture-produced WELCOME
    /// message, returning the connected client plus the "server" end of
    /// the duplex pipe to keep driving the exchange from the test.
    ///
    /// `buf_size` is intentionally configurable and used small in some
    /// tests specifically to force `read_exact` to loop across multiple
    /// underlying reads, so the reassembly behavior is actually exercised
    /// rather than assumed to work because everything fit in one read.
    async fn handshake_over_duplex(
        buf_size: usize,
    ) -> (Aap2Client<tokio::io::DuplexStream>, tokio::io::DuplexStream) {
        let (client_io, mut server_io) = tokio::io::duplex(buf_size);
        server_io.write_all(&[MAGIC_BYTE]).await.unwrap();
        server_io
            .write_all(&decode_hex(WELCOME_MSG_HEX))
            .await
            .unwrap();
        let client = Aap2Client::handshake(client_io).await.unwrap();
        assert_eq!(client.node_eid(), "dtn://earth.dtn/");
        (client, server_io)
    }

    #[tokio::test]
    async fn handshake_decodes_go_welcome_message() {
        // The assertion is inside handshake_over_duplex(); reaching here
        // without panicking is the pass condition.
        let _ = handshake_over_duplex(4096).await;
    }

    #[tokio::test]
    async fn configure_encodes_identically_to_go_client() {
        let (mut client, mut server_io) = handshake_over_duplex(4096).await;
        let expected_config_bytes = decode_hex(CONFIG_MSG_HEX);
        let response_bytes = decode_hex(RESPONSE_SUCCESS_HEX);

        let client_task = tokio::spawn(async move {
            client
                .configure(
                    ConfigureOptions::new()
                        .agent_id("fixture-agent")
                        .subscribe(true)
                        .secret("fixture-secret-123"),
                )
                .await
                .map(|secret| (secret, client))
        });

        let mut got = vec![0u8; expected_config_bytes.len()];
        server_io.read_exact(&mut got).await.unwrap();
        assert_eq!(
            got, expected_config_bytes,
            "CONFIG message bytes differ from the Go client's own encoding"
        );

        server_io.write_all(&response_bytes).await.unwrap();

        let (secret, _client) = client_task.await.unwrap().unwrap();
        assert_eq!(secret, "fixture-secret-123");
    }

    #[tokio::test]
    async fn send_response_status_matches_go_encoding() {
        let (mut client, mut server_io) = handshake_over_duplex(4096).await;
        let expected = decode_hex(RESPONSE_SUCCESS_HEX);

        client
            .send_response_status(ResponseStatus::Success)
            .await
            .unwrap();

        let mut got = vec![0u8; expected.len()];
        server_io.read_exact(&mut got).await.unwrap();
        assert_eq!(got, expected);
    }

    #[tokio::test]
    async fn send_adu_header_matches_go_encoding() {
        let (mut client, mut server_io) = handshake_over_duplex(4096).await;
        let expected_header = decode_hex(ADU_MSG_HEX); // declares payload_length = 12345
                                                       // Must match the fixture's declared payload_length (12345) exactly,
                                                       // since that length is itself part of the encoded header bytes.
        let payload = vec![0x42u8; 12345];

        let client_task =
            tokio::spawn(async move { client.send_adu("dtn://space.dtn/sink", &payload).await });

        let mut got_header = vec![0u8; expected_header.len()];
        server_io.read_exact(&mut got_header).await.unwrap();
        assert_eq!(
            got_header, expected_header,
            "ADU header bytes differ from the Go client's own encoding"
        );

        let mut got_payload = vec![0u8; 12345];
        server_io.read_exact(&mut got_payload).await.unwrap();
        assert_eq!(got_payload, vec![0x42u8; 12345]);

        client_task.await.unwrap().unwrap();
    }

    /// Decodes a Go-produced ADU header + a payload considerably larger
    /// than the duplex pipe's internal buffer, forcing `receive_adu`'s
    /// `read_exact` to loop across many underlying reads rather than
    /// completing in one. This is the explicit chunking/reassembly check
    /// Phase 3's acceptance criterion calls for.
    #[tokio::test]
    async fn receive_adu_reassembles_payload_across_many_reads() {
        // Buffer far smaller than the payload, so tokio::io::duplex will
        // only ever hand back small slices per read.
        let (mut client, mut server_io) = handshake_over_duplex(64).await;

        let header = decode_hex(ADU_MSG_HEX); // declares payload_length = 12345
        let payload: Vec<u8> = (0..12345u32).map(|i| (i % 251) as u8).collect();
        let payload_for_write = payload.clone();

        let server_task = tokio::spawn(async move {
            server_io.write_all(&header).await.unwrap();
            server_io.write_all(&payload_for_write).await.unwrap();
        });

        let (adu, got_payload) = client.receive_adu(None).await.unwrap();
        assert_eq!(adu.dst_eid, "dtn://space.dtn/sink");
        assert_eq!(adu.payload_length, 12345);
        assert_eq!(got_payload.len(), 12345);
        assert_eq!(got_payload, payload);

        server_task.await.unwrap();
    }
}
