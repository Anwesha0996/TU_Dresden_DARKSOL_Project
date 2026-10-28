//! Structured logging setup shared by both binaries.
//!
//! Every later phase (Phase 6 in particular: correlating a transfer's full
//! lifecycle by its UUID) depends on being able to see what happened, so
//! this is wired up before any real client/transfer logic exists.

use tracing_subscriber::EnvFilter;

/// Initializes a `tracing` subscriber that writes structured, leveled logs
/// to stderr. Verbosity is controlled via the `RUST_LOG` environment
/// variable (e.g. `RUST_LOG=debug`); defaults to `info` if unset.
pub fn init() {
    tracing_subscriber::fmt()
        .with_env_filter(
            EnvFilter::try_from_default_env().unwrap_or_else(|_| EnvFilter::new("info")),
        )
        .with_target(true)
        .init();
}
