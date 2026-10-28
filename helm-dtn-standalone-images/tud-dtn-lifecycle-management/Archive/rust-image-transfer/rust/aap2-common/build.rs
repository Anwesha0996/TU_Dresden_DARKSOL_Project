//! Generates Rust types from the vendored AAP2 protobuf definition.
//! See proto/aap2.proto for provenance (source repo, pinned tag/commit).

fn main() -> std::io::Result<()> {
    println!("cargo:rerun-if-changed=proto/aap2.proto");
    prost_build::compile_protos(&["proto/aap2.proto"], &["proto/"])?;
    Ok(())
}
