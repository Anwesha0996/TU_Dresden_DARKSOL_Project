//! Error type for the AAP2 client.

/// Errors returned by the AAP2 client.
#[derive(thiserror::Error, Debug)]
pub enum Aap2Error {
    #[error("I/O error talking to µD3TN: {0}")]
    Io(#[from] std::io::Error),

    #[error("failed to decode protobuf message: {0}")]
    Decode(#[from] prost::DecodeError),

    #[error("failed to encode protobuf message: {0}")]
    Encode(#[from] prost::EncodeError),

    #[error("did not receive the AAP 2.0 magic number (0x2f) on connect, got: {0:#04x}")]
    BadMagicByte(u8),

    #[error("connection closed by peer during handshake")]
    ConnectionClosedDuringHandshake,

    #[error("expected a {expected} message, got: {got}")]
    UnexpectedMessage { expected: &'static str, got: String },

    #[error("µD3TN rejected the request: {0:?}")]
    ResponseNotSuccess(crate::proto::ResponseStatus),

    #[error("SendAdu(): payload length {declared} does not match actual payload length {actual}")]
    PayloadLengthMismatch { declared: u64, actual: usize },
}

pub type Result<T> = std::result::Result<T, Aap2Error>;
