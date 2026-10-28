//! Shared code for the earth-side (`aap2-image-sender`) and space-side
//! (`aap2-image-receiver`) binaries: the AAP2 client, generated protobuf
//! types, and logging setup.

pub mod checksum;
pub mod client;
pub mod error;
pub mod logging;
pub mod proto;
pub mod skopeo;
pub mod transfer;

pub use client::{Aap2Client, AnyClient, ConfigureOptions};
pub use error::{Aap2Error, Result};
