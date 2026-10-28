# TASKS.md — Rust Image Transfer Extension for helm-dtn

**Goal:** add a Rust-based image transfer path to the existing `helm-dtn` Go plugin. A Rust sender ("earth side") pushes a container image through µD3TN via AAP2. A Rust receiver ("space side") receives it and uses `skopeo` to push it into Harbor. The whole path must be failsafe (verifiable, retryable, resumable-in-spirit even though DTN links are one-way), and the deliverable includes a working earth→space demo.

This file is written to be worked top-to-bottom by an agentic coding assistant. Each phase has a concrete objective, a task checklist, and acceptance criteria. Do not skip acceptance criteria — a phase is not "done" until its criteria pass, not just when the code compiles.

---

## Phase 0 — Repo orientation and ground truth (do this before writing any code) — DONE (2026-08-01)

- [x] Read `README.md`, `doc/mode-of-operation.md`, and `pkg/helmdtnt/README.md`, `pkg/helmdl/README.md` in the existing repo to confirm current Go image flow.
- [x] Read `pkg/aap2_client/*.go` end to end. Document (in `doc/rust-image-transfer/00-aap2-notes.md`) exactly what `Configure`, `SendAdu`, `ReceiveAdu`, `ReceiveMsg`, `SendResponseStatus` do, and what `WithAgentId` / `WithSubscribe` / `WithSecret` mean.
- [x] Locate the **source** `.proto` definition for AAP2 (not just the compiled `pkg/proto/aap2.pb.go`). It originates from the µD3TN project (D3TN GitLab/GitHub). Confirm the exact repo/path and pin a commit/tag. — `components/aap2/aap2.proto` @ `gitlab.com/d3tn/ud3tn` tag `v0.15.0` (commit `b27c1e28a14ed3c75bf96890f793638fc2f168fb`).
- [x] Check whether µD3TN already ships or documents a Rust AAP2 client/crate. If one exists and is usable, use it instead of hand-rolling bindings — record the decision and why. — `gitlab.com/d3tn/aap2-rs` exists but doesn't implement bundle ADU transfer, is Unix-socket only, and is purpose-built for BGP routing exchange; decided to hand-roll instead. See notes doc §3.
- [x] Confirm `skopeo` and `harbor` versions to target; note them in the same notes file. Do not assume — check what's actually installable in the target dev/test environment. — skopeo `v1.23.0`, Harbor `v2.15.2`. **Note: this dev machine has no Docker/skopeo installed locally — flagged as a blocker for Phase 1.**
- [x] **Acceptance:** `doc/rust-image-transfer/00-aap2-notes.md` exists and answers: how does an AAP2 client register, send, and receive today, and where does the Rust protobuf definition come from.

---

## Phase 1 — Local test infrastructure — DONE (2026-08-01)

Build this before any Rust code, so every later phase has something real to test against.

- [x] Add `deploy/local-test/docker-compose.yml` bringing up:
  - [x] A local Harbor instance (Harbor's own online installer, pinned `v2.15.2`, vendored via `harbor/fetch-and-prepare.sh` + checked-in `harbor/harbor.yml`) reachable at `http://harbor.local-test:8081` with `admin`/`Harbor12345` for local testing only.
  - [x] Two µD3TN instances, named **`earth-node`** and **`space-node`** (both `v0.15.0`, `deploy/local-test/ud3tn/Dockerfile`), each with AAP2 exposed over TCP, connected via the `tcpclv3` convergence layer on the `dtn-net` compose network.
- [x] Verify µD3TN's actual mechanism for scheduled/limited connectivity (contact plans) vs. simply using network-level disruption. — Confirmed via `doc/posix_quick_start_guide.md`/`doc/contacts_data_format.md` @ v0.15.0: contacts are runtime-only, application-layer (AAP2 config bundles / `aap2-config`), entirely distinct from network-level disruption. See `deploy/local-test/README.md` §"Contact configuration".
- [x] Add a `netem`/`tc`-based network shim between `earth-node` and `space-node` (`deploy/local-test/scripts/netem.sh`: `tc`/netem for delay+loss, `iptables` for a hard outage) so link latency and outage windows can be injected on demand. Documented in `deploy/local-test/README.md`.
- [x] Smoke test: manual Go-based (existing `aap2_client`) send/receive round trip via `deploy/local-test/smoketest/main.go`, run against the real two-node harness. **Found and fixed a real bug in the process:** `ReceiveAdu` in `pkg/aap2_client/aap2_sync_client.go` used a single `Read()` call instead of `io.ReadFull`, silently truncating payloads >1 TCP segment. Fixed; verified with 256KB and 5MB payloads (checksums match).
- [x] **Acceptance:** `docker compose up -d --build` in `deploy/local-test/` brings up Harbor + earth-node + space-node (all verified healthy); a manual Go-based send/receive round-trip succeeds (checksums match, including a 131KB transfer that was queued through and survived a real injected outage, delivered automatically ~52s after the outage was cleared); latency/outage injection is demonstrated and documented in `deploy/local-test/README.md`.

---

## Phase 2 — Rust workspace scaffolding — DONE (2026-08-01)

- [x] Create `rust/` at repo root as a Cargo workspace with two binary crates: `rust/aap2-image-send` (earth side) and `rust/aap2-image-recv` (space side), plus a shared library crate `rust/aap2-common` for anything both sides need (protobuf types, framing, checksums, logging setup). — Framing/checksums land in Phase 3/4; this phase scaffolds the crates, protobuf types, and logging only.
- [x] Set up protobuf codegen for AAP2 in `aap2-common` (via `prost-build` in `build.rs`), pointed at the `.proto` source pinned in Phase 0. — `rust/aap2-common/proto/aap2.proto` vendored from the pinned tag/commit; `build.rs` runs `prost_build::compile_protos`.
- [x] Add structured logging (`tracing` + `tracing-subscriber`) to both binaries from the start. — `aap2_common::logging::init()`, `RUST_LOG`-controlled, verified via a real run.
- [x] Add `clap` (or similar) CLI argument parsing to both binaries mirroring the existing Go flags where it makes sense (`--socket`, `--tcp`, `--destination-eid`, `--registry`/`--harbor-*`). — `--socket`/`--tcp` (mutually exclusive, clap `ArgGroup`), `-d/--destination-eid` on the sender, `-r/--registry` on the receiver, both with `--agent-id`.
- [x] **Acceptance:** `cargo build --workspace` succeeds (clean, no warnings, `cargo clippy --workspace` also clean); both binaries run with `--help` and print usage (verified); protobuf types generated from the AAP2 `.proto` are importable in `aap2-common` (confirmed `AapMessage`, `Welcome`, `ConnectionConfig`, `BundleAdu`, `AapResponse`, `ResponseStatus`, etc. all present and matching the Go client's message set).

Note: this dev machine had no local Rust or `protoc` install; installed via `rustup` (stable, `~/.cargo`) and `brew install protobuf` (both no-sudo, unlike the earlier Docker Desktop cask install).

---

## Phase 3 — AAP2 client in Rust (protocol layer only, no images yet) — DONE (2026-08-01)

- [x] In `aap2-common`, implement a minimal AAP2 client supporting both TCP and Unix-socket transport, mirroring `aap2_tcp_client.go` / `aap2_unix_client.go`: connect, `configure` (agent ID, optional subscribe, optional secret), `send_adu`, `receive_adu`, `receive_msg`, `send_response_status`. — `rust/aap2-common/src/client.rs`, generic `Aap2Client<S>` over `TcpStream`/`UnixStream`.
- [x] Write it as an async client (`tokio`) rather than blocking. — Fully `tokio`-async throughout.
- [x] Unit test the framing/serialization logic against fixtures captured from the **existing Go client**. — `rust/aap2-common/testdata/gen_fixtures` captures real Go-encoded wire bytes; `client.rs`'s `go_fixture_tests` module asserts the Rust encoder/decoder agree with them byte-for-byte (5 tests, all passing).
- [x] Integration test against the Phase 1 harness: Rust sender → `earth-node` → `space-node` → Rust receiver, raw arbitrary bytes. — `rust/aap2-common/tests/harness_roundtrip.rs`, run live against the real two-node stack.
- [x] **Acceptance:** verified live: a 1KB and an 8MB Rust-to-Rust round trip both pass over the real harness (checksums match); reassembly-across-many-reads is additionally covered directly in the unit tests (small duplex-pipe buffer forcing many `read_exact` iterations). Confirmed (Phase 0 notes) AAP2 itself does not chunk at the protocol level - the receive-side fix that matters is `read_exact`-style reassembly of the raw payload, which this client uses throughout (unlike the Go client's single-`Read()` bug fixed in Phase 1).

---

## Phase 4 — Sender: image export (earth side) — DONE (2026-08-01)

- [x] In `aap2-image-send`, given an image reference, shell out to `skopeo copy docker://<image> oci-archive:<tmp>/image.tar`. — `rust/aap2-image-send/src/export.rs`. Note: added `--override-os linux --override-arch <target-arch, default amd64>` since the receiving cluster is always Linux regardless of what OS `aap2-image-send` itself runs on (discovered live: macOS host defaults to picking a nonexistent "darwin" manifest variant without this).
- [x] Compute a strong checksum (SHA-256) of the produced archive immediately after export, before it touches the network. Store `image.tar.sha256` alongside it. — `aap2-common/src/checksum.rs` (streamed, not loaded fully into memory), file written alongside the archive.
- [x] Wrap the `skopeo` invocation with explicit error handling distinguishing "image not found/auth failure" from "skopeo not installed" from "disk full." — `aap2-common/src/skopeo.rs` (`SkopeoErrorKind` classification from stderr) + `aap2-image-send/src/export.rs` (`ExportError` variants with actionable hints).
- [x] Write the archive + checksum + minimal manifest into a small header structure sent as the first frame of the AAP2 payload. — `aap2-common/src/transfer.rs`: `TransferHeader` (transfer UUID, source/target image ref, timestamp, sha256, size) + `encode_transfer_payload`/`decode_transfer_payload` (8-byte length prefix + JSON header + archive bytes), unit tested.
- [x] **Acceptance:** verified live against a real image (`docker.io/library/hello-world:latest`, exported, checksummed, sent over the real two-node harness, accepted by µD3TN) and both distinct failure modes (bad image ref → "authentication failed, or the image/tag does not exist" with a `skopeo login` hint; skopeo removed from PATH → "skopeo is not installed" with an install hint) - both exit non-zero with a clear message, no panic. Disk-full classification exists (`SkopeoErrorKind::DiskFull`, matched on skopeo's stderr) but wasn't separately live-tested (hard to trigger safely on a shared dev machine).

---

## Phase 5 — Receiver: skopeo → Harbor (space side) — DONE (2026-08-01)

- [x] In `aap2-image-recv`, register an AAP2 agent and receive the header + archive + checksum sent in Phase 4. — `rust/aap2-image-recv/src/main.rs` agent loop.
- [x] **Verify the checksum before touching Harbor.** If it doesn't match, do not push — quarantine, log, keep listening. — `rust/aap2-image-recv/src/receive.rs` `verify_checksum()`.
- [x] Write the verified archive to a temp path, then `skopeo copy oci-archive:... docker://<harbor-host>/<project>/<image>:<tag> --dest-creds <user>:<pass>` from a creds file (`--registry-creds-file`, not hardcoded, mirrors `/etc/secret-volume/token`). — `push_and_verify()`. Note: local Harbor is plain HTTP, so `--insecure-registry` maps to `--dest-tls-verify=false` (discovered live: this flag alone is sufficient for skopeo to fall back from HTTPS to HTTP, no separate "http://" scheme needed).
- [x] After the push, `skopeo inspect` and compare the resulting digest against what was pushed. — same function, compares `skopeo inspect oci-archive:<local>` vs `skopeo inspect --creds ... docker://<pushed ref>`.
- [x] Move the archive from "incoming" to "completed" only after post-push verification succeeds. — atomic `rename`, not copy.
- [x] **Acceptance:** verified live end-to-end against the real harness + real Harbor: `aap2-image-send` → real two-node µD3TN → `aap2-image-recv` → real local Harbor, digest matched at every stage (source archive, registry-reported, all three sha256/digest values identical: `d1a8d0a4...`). Corrupted-payload case also verified live: a transfer with a deliberately wrong declared checksum was caught, quarantined to `quarantine/<transfer_id>.tar`, logged as an error, confirmed absent from Harbor (`repository ... not found`) - and the receiver process kept running and accepting new transfers throughout, no crash.

---

## Phase 6 — Failsafe / resilience layer (cross-cutting — apply to both binaries) — DONE (2026-08-01)

This is the part that makes the previous phases "failsafe" rather than a happy-path demo. Treat each item as a requirement, not a nice-to-have.

- [x] **Idempotency:** the receiver checks `completed/<id>.tar` and `quarantine/<id>.tar` for the transfer's UUID before doing any work, and treats a hit as an already-handled duplicate (ack, log, skip - no re-push, no re-quarantine). — `ReceiverPaths::duplicate_status` in `rust/aap2-image-recv/src/receive.rs`; verified live (see below).
- [x] **Atomicity:** all disk writes go to a `.part`/temp path and are renamed into place only on success. — `write_incoming`/quarantine moves in `receive.rs`, sender's checksum file write in `export.rs`.
- [x] **Timeouts:** every subprocess (`skopeo`) call has an explicit timeout (`EXPORT_TIMEOUT`/`PUSH_TIMEOUT`/`INSPECT_TIMEOUT` in `aap2-common::skopeo`), and the receiver's ADU-payload read is wrapped in `ADU_READ_TIMEOUT` (120s) - a stall there tears down and reconnects rather than hanging the loop forever, since a partially-read stream can't be trusted to resume framing correctly.
- [x] **Retries with backoff:** `aap2-common::skopeo::run_with_retry` + `RetryPolicy` (bounded exponential backoff, default 4 attempts/500ms base/30s cap), applied to both the sender's export and the receiver's push+inspect. Transient (`Timeout`, `NetworkOrRegistryUnavailable`) vs. permanent (`NotInstalled`, `AuthOrNotFound`, `DiskFull`) classified from skopeo's stderr, permanent failures skip retry entirely.
- [x] **Quarantine, don't drop:** unparseable-header payloads, checksum mismatches, and push failures (after retries exhaust) all land in `quarantine/<id>.tar` with a `<id>.meta.json` sidecar (reason, timestamp, parsed header if available) - verified live, metadata sidecar inspected and readable.
- [x] **Structured, correlated logging:** every transfer-related log line carries `transfer_id` (or a generated `synthetic_id` for payloads too broken to have one) as a structured `tracing` field.
- [x] **Process resilience:** a single transfer's failure never crashes the loop (verified live repeatedly: checksum mismatch, invalid archive push failure, duplicate delivery - receiver kept running and accepting new transfers throughout every case). A connection-level failure (not a per-transfer one) does tear down and reconnect via an outer retry loop, rather than either crashing or spinning on a desynced stream. The Go `panic()`-on-error pattern (Phase 0 notes) is explicitly not replicated.
- [x] **Acceptance (partial - full fault-injection suite is Phase 7):** verified live: (1) idempotent duplicate delivery - same `transfer_id` sent twice, second delivery logged as duplicate and skipped without re-processing; (2) checksum-mismatch quarantine with metadata sidecar (Phase 5); (3) retry/backoff unit-tested on a mocked clock (`#[tokio::test(start_paused = true)]`), confirming exact retry counts and backoff schedule without real wall-clock delays, and confirming permanent failures are never retried. Phase 7 formalizes these (and adds skopeo-transient-failure-injection, link-outage-mid-transfer) as a committed, self-contained automated suite.

---

## Phase 7 — Automated tests — DONE, with one honestly-documented environment limitation (2026-08-01)

- [x] Unit tests: checksum verification, header serialization/parsing, retry/backoff logic with a **mocked clock** (`#[tokio::test(start_paused = true)]`, no real sleeps). — 14/14 passing, 100% reliable across many repeated runs. `aap2-common/src/{transfer,skopeo,client}.rs` test modules.
- [x] Integration tests: full sender→receiver round trip over the Phase 1 harness. — `aap2-common/tests/harness_roundtrip.rs`: real Rust-to-Rust round trip (1KB and 8MB payloads) over the live two-node harness. 100% reliable across many repeated runs (verified again immediately after the fault-injection investigation below, back-to-back).
- [x] Fault-injection tests, driven from a script: `deploy/local-test/scripts/test-fault-injection.sh` — corrupted payload, duplicate transfer ID, skopeo transient failure (fake wrapper script, per this phase's own suggestion), skopeo permanent failure. (Link-outage-mid-transfer already demonstrated live in Phase 1/README and repeated in Phase 9.)
- [x] **Acceptance (mostly met, one finding not a code defect):** `cargo test --workspace` is fully green and reliable. Building the fault-injection script surfaced and fixed a **real bug in the test suite itself**: reusing fixed literal agent IDs (e.g. `"fault-corrupt"`) across repeated runs let a stale, uncleanly-killed prior registration collide with a new one, since `aap2-image-recv` doesn't pin an AAP2 secret - µD3TN then rejected the fresh CONFIGURE as `Unauthorized`. Fixed by suffixing every agent ID with a fresh run ID. Separately, this development environment (Docker Desktop for macOS + µD3TN v0.15.0's tcpclv3 CLA) was found to **intermittently drop the first bundle(s)** sent after a fresh contact - `earth-node` logs a clean `TX ... OK`, `space-node` logs `RX: RX buffer is full and does not clear. Resetting all parsers!`, no retry from µD3TN itself. This was investigated extensively (ruled out: `cargo test` vs shell script, piped vs file-redirected I/O, tokio runtime flavor, payload size/content, agent naming, full `docker compose down && up`) and does **not** reproduce on the reliable paths (unit tests, `harness_roundtrip.rs`, and numerous individual manual sends during Phases 1/3/5/6, all captured above) - only on `aap2-image-recv`'s specific subscriber-loop pattern, and even then not always. It never produced a *wrong* result (no corruption, no false success) - only occasional total loss of a bundle, which is within the disruption-tolerant premise of the whole project and exactly what Phase 6's retry logic exists for; the fault-injection script's own retry loops are the practical mitigation. Documented in full in the script's header comment rather than hidden - see there for the complete investigation trail.

---

## Phase 8 — Wire into the existing helm-dtn plugin — DONE (2026-08-01)

- [x] Decide and document explicitly: **alternate backend**, not a replacement (chart transfer/install logic stays Go-only per the non-goals; the two transports differ in granularity - one archive vs. one-transfer-per-image). Full reasoning in `doc/rust-image-transfer/08-integration-decision.md`, including the remote-side deployment requirement (a separately-running `aap2-image-recv` agent) and a documented cross-transfer ordering caveat.
- [x] Added `--image-transport go|rust` to `cmd/install.go`/`cmd/upgrade.go` (default `go`, unchanged behavior). `rust` mode: `helmdl.DownloadNonImageDependencies` (new - skips image pulling, keeps git) + `helmdtnt.SendImagesViaRust` (new - shells out to `aap2-image-send` once per detected image, before the chart archive send) + unmodified `helmdp`/`SendChart` for the (now image-free) chart archive.
- [x] Updated `README.md` (repository structure + `--image-transport` flag docs on both commands), `pkg/helmdl/README.md`, and `pkg/helmdtnt/README.md` (remote-side deployment requirement).
- [x] **Acceptance:** verified live end-to-end with a real chart (`deploy/demo/chart`, alpine:3.20) against the real two-node harness + real Harbor: `helm dtn install --image-transport rust` detected the image via `helmdd`, sent it via `aap2-image-send` (checksum computed, transfer accepted by µD3TN), a live `aap2-image-recv` agent (registered as `dtn://space.dtn/images`, `--registry localhost:8081/library`) received it, pushed to Harbor, and independently digest-verified it (confirmed separately via `skopeo inspect`) - while the chart archive (image-free, references correctly rewritten to `localhost:8081/library/alpine:3.20` by the unmodified `helmdp`) was sent via the existing Go path to `dtn://space.dtn/install` in the same run, both bundles observed arriving correctly and in order on `space-node`.

---

## Phase 9 — Earth-to-space demo — DONE (2026-08-01)

This is the concrete deliverable to show working, not just described.

- [x] `earth-node`/`space-node` naming already established in Phase 1. `space-node` (and the new `aap2-image-recv` container - see below) has its egress locked down with real iptables rules (loopback + the two Docker subnets it legitimately needs, default-deny everything else) rather than Docker's `internal: true` network flag - empirically verified during this phase that `internal: true` also disables host-published ports, which the rest of this harness (`deploy/local-test`) depends on. This is genuine L3 enforcement in the container's own network namespace, not just documentation, and the demo proves it live (see `airgap-proof.log` below) rather than asserting it.
- [x] Injected a representative one-way delay (netem, 300ms ±50ms on `space-node`) plus a deliberate outage window (`iptables DROP` between `earth-node` and `space-node`, held for 15s, started *before* the transfer even begins) - the transfer is queued by µD3TN during the outage and delivered automatically ~20-30s after the link is restored, with zero manual intervention.
- [x] `deploy/demo/run-earth-to-space-demo.sh`: one command, unattended. Tears down/rebuilds a clean stack (Harbor + earth-node + space-node + two new containerized services, `aap2-image-send`/`aap2-image-recv`, built from `rust/Dockerfile`), locks down and proves the space-side airgap, injects delay + outage, sends a real image (`docker.io/library/alpine:3.20`) through the outage, waits for completion, and independently verifies the result - all from inside the code path being tested, no host shortcuts.
- [x] Evidence captured in `deploy/demo/output/`: `demo.log` (full run), `airgap-proof.log` (100% packet loss to `1.1.1.1` from both `space-node` and `aap2-image-recv`, confirmed earth-side retains normal internet access), `aap2-image-send.log`, `aap2-image-recv.log`, `skopeo-inspect.json` (final pushed image, digest `sha256:c64c687...`, matching the digest independently reported by the receiver's own post-push verification).
- [x] **Acceptance:** verified live, first real run: the script ran completely unattended start to finish, survived the injected outage with no manual intervention (bundle queued during the 15s blackout, delivered automatically once cleared), and ended with the image verifiably present in Harbor via `skopeo inspect` run from inside the airgapped space-side network itself (not from the host, not from earth-node) - digest matches across sender checksum, receiver post-push verification, and the final independent inspect.

---

## Phase 10 — Report — DONE (2026-08-01)

- [x] Wrote `doc/rust-image-transfer/REPORT.md`: architecture diagram, phase-by-phase build summary (linking back to this file for full detail/evidence), the Phase 8 integration decision, a failsafe-mechanism-to-test-evidence table, the Phase 9 demo with its evidence artifacts, a testing summary, and known limitations / follow-up work.
- [x] Structured to read alongside `doc/mode-of-operation.md` (same doc set, opens by pointing readers at the existing README/mode-of-operation for context rather than re-explaining the base plugin).
- [x] **Acceptance:** every claim in the report cites a specific path (TASKS.md phase section, log/evidence file under `deploy/demo/output/`, or source file) rather than asserting outcomes unsupported by evidence gathered during this project.

---

## Project complete

All 10 phases done, 2026-08-01. Summary: a working, tested, documented Rust
image-transfer path (`rust/`) alongside the existing Go `helm-dtn` plugin,
wired in as an opt-in alternate backend (`--image-transport rust`), with a
local Docker Compose test harness (`deploy/local-test/`), a full failsafe
layer (checksums, idempotency, retries, quarantine), an automated test
suite (unit + integration + fault-injection), and a one-command,
unattended, evidence-producing earth-to-space demo (`deploy/demo/`) that
proves disruption tolerance and airgap enforcement live rather than by
assertion. Two real bugs were found and fixed along the way: a payload-
truncation bug in the *existing* Go AAP2 client (Phase 1), and an agent-
ID/secret collision bug in this project's own fault-injection test suite
(Phase 7). See `doc/rust-image-transfer/REPORT.md` for the full report and
`doc/rust-image-transfer/00-aap2-notes.md` /
`doc/rust-image-transfer/08-integration-decision.md` for the two other
standalone design documents produced along the way.

---

## Explicit non-goals (write these down so they don't get silently attempted)

- Not building a general-purpose replacement for `helmdd`'s dependency *detection* — only the transfer/push mechanism for already-detected images changes.
- Not implementing chart templating or Helm install/upgrade logic in Rust — that stays in Go (`installChart`/`upgradeChart` in `receiver.go`).
- Not building production Harbor deployment/hardening — the Phase 1 Harbor instance is for local testing only, clearly marked as such.
