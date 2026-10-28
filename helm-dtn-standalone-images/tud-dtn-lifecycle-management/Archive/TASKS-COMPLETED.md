# Completed Tasks

This file summarizes the work completed for the Rust-native AAP2 image
channel described in `TASKS-PART-2.md`.

## Infrastructure and AAP2

- [x] Reused the existing `earth-node` and `space-node` micro-D3TN harness.
- [x] Reused the pinned AAP2 protobuf source and Rust `prost` code generation.
- [x] Verified Rust AAP2 transfers over the real two-node harness with 1 KiB
  and 8 MiB payloads.
- [x] Verified independent Go and Rust AAP2 agents can remain registered on
  the same micro-D3TN node concurrently.
- [x] Verified sender and receiver payload integrity with SHA-256.

## Rust Image Sender

- [x] Created the `rust/aap2-image-sender/` binary crate.
- [x] Added CLI inputs for source image, Harbor target, release name,
  destination EID, transport and target architecture.
- [x] Added `skopeo copy docker://... oci-archive:...` image export.
- [x] Added distinct errors for missing `skopeo`, authentication or missing
  images, disk exhaustion, timeout and other export failures.
- [x] Added a temporary work directory that is removed automatically after a
  normal invocation.
- [x] Added transfer metadata containing source reference, target reference,
  UUID, release name, timestamp, archive size and SHA-256.
- [x] Added AAP2 transfer framing and sender acknowledgement validation.
- [x] Made the sender exit successfully only after micro-D3TN accepts the ADU.

## Rust Image Receiver

- [x] Created the `rust/aap2-image-receiver/` binary crate.
- [x] Registered the receiver under the independent `image-transfer` AAP2
  agent ID.
- [x] Added a long-running receive loop with AAP2 reconnection handling.
- [x] Added transfer decoding, temporary archive storage and SHA-256
  verification.
- [x] Added `skopeo copy oci-archive:... docker://...` Harbor push behavior.
- [x] Added post-push `skopeo inspect` digest confirmation.
- [x] Read Harbor credentials from `HARBOR_CREDS=user:password`.
- [x] Converted credentials into a temporary registry auth file so passwords
  do not appear in `skopeo` process arguments.
- [x] Logged failed transfers and continued receiving subsequent messages.
- [x] Removed persistent idempotency state, quarantine storage and application
  retry or backoff behavior.

## Go CLI Integration

- [x] Kept `helm dtn install <chart>` as the single installation entrypoint.
- [x] Updated install and upgrade to invoke `aap2-image-sender` once per
  detected image.
- [x] Passed image source, Harbor target, release metadata and the
  `<destination-eid>/image-transfer` destination to the Rust process.
- [x] Removed the `--image-transport` selector and legacy Go fallback.
- [x] Kept `helmdtnt.SendChart` as the separate Go chart-transfer path.
- [x] Corrected upgrade argument handling to use
  `upgrade <release-name> <chart>`.

## Chart Packaging and Go Receiver

- [x] Removed the Go image downloader and Docker image serialization path
  from `helmdl`.
- [x] Kept non-image dependency downloads through
  `DownloadNonImageDependencies`.
- [x] Prevented `helmdp` from including an `Images/` directory in chart
  archives.
- [x] Removed all `tarball.ImageFromPath` and `remote.Write` image-push blocks
  from the Go receiver.
- [x] Removed registry credential and image-push plumbing from the Go receiver.
- [x] Documented that chart and image messages may arrive in either order.
- [x] Documented Kubernetes `ImagePullBackOff` recovery as accepted behavior
  when the chart arrives before its image.

## Example and Deployment Files

- [x] Added `examples/demo-chart/` with one nginx Deployment and Service.
- [x] Used the source-resolvable image
  `docker.io/library/nginx:alpine` in the example values.
- [x] Updated the Rust Dockerfile for the renamed binaries and locked builds.
- [x] Added `rust/.dockerignore` to exclude the Cargo `target/` directory.
- [x] Updated the Docker Compose demo and outage script for the new binary
  names, agent ID and credential environment variable.
- [x] Removed the obsolete retry and quarantine fault-injection script.

## Documentation

- [x] Updated the root README for the automatic Rust image path.
- [x] Updated the `helmdl` and `helmdtnt` package documentation.
- [x] Replaced the old alternate-backend integration decision.
- [x] Updated `doc/rust-image-transfer/REPORT.md` with the implemented
  architecture, ordering behavior and verification evidence.
- [x] Added `doc/rust-image-transfer/DELETIONS.md` to record removals made
  before local Git history was initialized.

## Local Git

- [x] Initialized a local Git repository on branch `main`.
- [x] Added ignore rules for Rust build output, Harbor runtime data, generated
  dependency charts, local binaries and environment files.
- [x] Created initial commit `0675db3` with the Rust-native image-channel
  implementation and a deletion summary in the commit message.

## Verification Performed

- [x] `cargo check --workspace --offline`
- [x] `cargo test --workspace --offline` with 9 unit and compatibility tests
  passing.
- [x] Real ignored AAP2 harness tests with 1 KiB and 8 MiB payloads passing.
- [x] Concurrent Go and Rust AAP2 agent test passing.
- [x] `cargo clippy --workspace --all-targets --offline -- -D warnings`
- [x] Focused Go tests for `cmd`, `helmdl`, `helmdp` and `helmdtnt` passing.
- [x] `go vet` for the changed Go packages passing.
- [x] Demo shell syntax validation passing.
- [x] Docker Compose configuration validation passing with `HARBOR_CREDS` set.

## Environment-Dependent Verification Remaining

- [ ] Run the complete Phase F flow through a Kubernetes-capable harness:
  `helm dtn install examples/demo-chart/`, Harbor inspection and
  `kubectl get pods`.
- [ ] Repeat the complete installation while injecting an outage during the
  image transfer and capture pod recovery evidence.
- [ ] Rebuild the demo containers after Docker BuildKit can resolve
  `index.crates.io`.
- [ ] Run standalone `helm lint` after a Helm executable is available on the
  host.
- [ ] Repair the two unrelated `pkg/helmdd` fixtures so the full
  `go test ./...` command passes. The current failures are a generated
  `Chart.lock` collision and a stale rendered-manifest expectation.
