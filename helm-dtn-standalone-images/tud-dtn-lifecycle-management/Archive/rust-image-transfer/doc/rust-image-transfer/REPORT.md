# Rust-native AAP2 image channel report

## Result

The image path is now independent from the chart path while retaining
`helm dtn install <chart>` as the single user entrypoint.

```text
helm dtn install / upgrade
  |
  +-- helmdd image references
  |     `-- aap2-image-sender -> source µD3TN -> remote µD3TN
  |                                  `-- aap2-image-receiver -> Harbor
  |
  `-- chart-only archive -> existing Go chart AAP2 agent -> Helm lifecycle
```

The Go and Rust processes register different AAP2 agents against the same
µD3TN daemons. No second DTN deployment is introduced.

## Implemented components

| Area | Implementation |
| --- | --- |
| Shared Rust AAP2 | `rust/aap2-common`: pinned AAP2 protobuf generation, socket/TCP client, Go-compatible framing fixtures, transfer metadata framing, checksum and bounded `skopeo` execution |
| Sender | `rust/aap2-image-sender`: accepts source image, Harbor target and release metadata; exports an OCI archive with `skopeo`; sends one AAP2 ADU; exits successfully only after the local µD3TN send acknowledgement |
| Receiver | `rust/aap2-image-receiver`: subscribes as `image-transfer`; validates checksum; writes only to a temporary directory; pushes and inspects with `skopeo`; logs failures and continues receiving |
| Go CLI | `cmd/install.go` and `cmd/upgrade.go` invoke `aap2-image-sender` once per detected image, then send the chart separately through `helmdtnt.SendChart` |
| Packaging | `helmdl` no longer contains a Go image downloader; `helmdp` removes any `Images/` directory before creating the chart archive |
| Go receiver | All `tarball.ImageFromPath` / `remote.Write` image-push paths were removed; it handles chart lifecycle only |
| Example | `examples/demo-chart` contains one nginx Deployment and Service using the source-resolvable `docker.io/library/nginx:alpine` reference |

Each transfer header contains the source image reference, destination image
reference, UUID transfer ID, associated release name, creation timestamp,
archive size and SHA-256 checksum. This is deliberately a small transfer
format rather than a general image protocol.

## Runtime contract

The local Helm environment must have `aap2-image-sender` and `skopeo` on
`PATH`. The remote environment runs the receiver alongside the Go lifecycle
receiver:

```sh
HARBOR_CREDS='user:password' aap2-image-receiver \
  --tcp space-node:4242 \
  --agent-id image-transfer \
  --registry harbor.example/library
```

The receiver converts `HARBOR_CREDS` into a short-lived registry auth file.
The password is not passed in `skopeo`'s process arguments.

## Image and chart ordering

Image and chart messages are intentionally not coordinated. The chart can be
installed before its image is visible in Harbor. In that case a pod may enter
`ImagePullBackOff`; Kubernetes's normal pull retries are expected to recover
after the image receiver finishes. This is a known characteristic of the
design, not a receiver error.

## Scope limits

The implementation has no idempotency state, quarantine storage, application
retry/backoff, or blocking image/chart completion handshake. A bad transfer is
logged with its transfer ID and does not terminate the receiver loop.

## Verification

The following checks were run after the integration change:

| Check | Result |
| --- | --- |
| `cargo check --workspace --offline` | Passed for all three Rust crates |
| `cargo test --workspace --offline` | Passed: 9 unit/compatibility tests |
| Real µD3TN AAP2 round-trip tests | Passed against the running `earth-node` / `space-node` pair for both 1 KiB and 8 MiB payloads |
| Concurrent Go and Rust agents | Passed: the Go `go-concurrent` subscriber remained registered on `space-node` while the independent Rust round-trip agent completed; a subsequent 1 KiB Go transfer also completed with matching sender/receiver SHA-256 `ae3f6446a77a4f8f94a89bcf7bd460908dacc54990b15b4a82bdcddd06956040` |
| `cargo clippy --workspace --all-targets --offline -- -D warnings` | Passed |
| Focused Go package tests | `go test ./cmd ./pkg/helmdl ./pkg/helmdp ./pkg/helmdtnt` passed |
| Full Go suite | All packages compiled; the overall command failed in two unrelated `pkg/helmdd` fixture tests (`Chart.lock` collision and stale rendered-template expectation) |
| `bash -n deploy/demo/run-earth-to-space-demo.sh` | Passed |
| `docker compose ... config --quiet` with `HARBOR_CREDS` set | Passed |
| **Real Kubernetes end-to-end** (`deploy/demo-k8s/`) | **Passed** — see below |

### Real Kubernetes end-to-end run

A `kind` cluster was network-joined to the Phase 1 harness (its node attached
to Harbor's Docker network with a containerd insecure-registry mirror) to
stand in for the remote airgapped cluster, satisfying the earlier-noted gap.
`aap2-image-receiver` and the Go chart receiver both ran as containers
against this cluster. `helm dtn install examples/demo-chart/` was run twice
from the host, using only documented CLI flags:

1. **Baseline run**: `helmdd` detected `docker.io/library/nginx:alpine`,
   `aap2-image-sender` exported and sent it, `aap2-image-receiver` pushed it
   to Harbor, the chart-only bundle went over the unmodified Go path, and the
   pod came up `Running`. The chart happened to arrive and install before the
   (larger) image finished its journey, producing one real
   `ImagePullBackOff` cycle in `kubectl get events` that resolved itself
   within ~15 seconds once the image landed — a live confirmation of the
   accepted ordering behavior described above, not just a design assumption.
2. **Outage-injected run**: `deploy/local-test/scripts/netem.sh outage
   space-node` was run *before* `helm dtn install`. Both the image and chart
   sends were accepted locally by earth-node's µD3TN immediately
   (store-and-forward); nothing reached space-node for 15 seconds while the
   outage held (confirmed via receiver logs); clearing the outage delivered
   both within seconds, with the image pushed to Harbor (digest verified via
   `skopeo inspect` to match the baseline run's identical image content) and
   the pod `Running` shortly after.

Full setup, exact commands, and captured logs/`kubectl`/`skopeo` evidence are
in `deploy/demo-k8s/README.md` and `deploy/demo-k8s/output/`. This run also
surfaced two issues unrelated to the image-transfer design itself, both
recorded in `deploy/demo-k8s/README.md`:

- **Fixed**: `pkg/helmdtnt/receiver.go` had no buildable entry point (it
  declared `package helmdtnt`, shared with the library code `cmd/*.go`
  imports, but its `main()` was unexported) despite being documented as a
  separately published binary. Fixed by exporting it as `RunReceiver()` and
  adding a thin `cmd/receiver/main.go`. No behavior changed.
- **Not fixed — follow-up work**: the Go chart receiver's `panic(err)` on any
  Helm SDK error (flagged as a risk in Phase 0's `00-aap2-notes.md`) was
  confirmed live: a genuine Helm error crashed the entire receiver process,
  taking the `install`, `upgrade`, and `uninstall` agents down together, not
  just the one failed operation. This is precisely why the Rust image
  receiver was deliberately built to log-and-continue instead. Fixing the Go
  side to match (recover from per-transfer errors without killing the whole
  process) is recommended follow-up work, out of scope for this task list.

The earlier Rust AAP2/Harbor run artifacts under `deploy/demo/output/`
(image-only, no Kubernetes) remain valid evidence of the underlying transfer
mechanics but predate this full-stack run and are superseded by it for
end-to-end verification purposes.
