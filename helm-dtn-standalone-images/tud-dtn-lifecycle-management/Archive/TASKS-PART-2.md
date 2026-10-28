# TASKS-PART-2.md — Rust-Native AAP2 Image Channel for helm-dtn

**Run this after the first task list is complete.** This revises the earlier "Part 2" plan: that version kept AAP2 entirely in Go and used Rust only as a skopeo-wrapping subprocess. This version corrects that — Rust owns AAP2 send/receive for images directly, confirmed against this flow:

```
Helm chart (image reference only)
   -> helm CLI starts helm-dtn plugin
   -> Rust reads image reference
   -> skopeo downloads the image as an OCI archive
   -> Rust adds metadata, sends via its own AAP2 client
   -> source µD3TN -> remote µD3TN
   -> Rust receiver (own AAP2 agent)
   -> skopeo pushes the image to Harbor
   -> Kubernetes uses the image
```

## Scope, precisely

- **`helm dtn install <chart>` stays the single entrypoint.** No separate CLI for images.
- **Reuse the existing µD3TN deployment** (`earth-node` / `space-node` from the Part 1 test harness). Rust registers as its **own AAP2 agent** (distinct agent ID, e.g. `image-transfer`) against the same µD3TN daemons the existing Go chart-transfer agent already talks to. This is not a second DTN deployment — one pair of µD3TN nodes, two AAP2 agents (Go for the chart, Rust for images).
- **Images no longer travel inside the chart bundle.** `helmp` stops packing an `Images/` directory into the `tar.gz` — that bundle now carries the chart only. Images travel exclusively through the new Rust/AAP2 channel, in parallel to (not nested inside) the chart transmission.
- **skopeo does both ends of the image's journey** — pulling the source image into an OCI archive on the sender side, and pushing that archive into Harbor on the receiver side. Rust's job is: read the image ref, invoke skopeo, attach minimal metadata, move bytes over AAP2. Keep it minimal — one struct for the metadata header (image ref, Harbor target ref/tag, a transfer ID, the associated chart/release name), not a general protocol.
- **No idempotency/quarantine/retry framework.** If a push fails, log it clearly and exit non-zero; that's sufficient for this scope. If this proves inadequate during Phase F testing, note it in the report as follow-up work rather than building it now.

If this reading is still off, stop before Phase B.

---

## Phase A — Confirm reused infrastructure — DONE (2026-08-01)

- [x] Part 1 harness confirmed running (`docker compose up -d` in `deploy/local-test/`, Harbor healthy, both nodes up).
- [x] Rust AAP2 bindings from Part 1 carried over as-is: `rust/aap2-common` already has a full async client (connect/configure/send_adu/receive_adu/receive_msg/send_response_status) generated from the pinned `.proto`, unit- and integration-tested. No rebuild needed.
- [x] Confirmed two independent AAP2 agents concurrently against the same µD3TN node: a Go agent (`deploy/local-test/smoketest`, agent ID `install`) and a Rust agent (`aap2-image-recv`, agent ID `image-transfer`) were both registered on `space-node` at the same time; a payload sent to each independently arrived at the correct one only, with both processes staying up throughout (µD3TN logs showed no rejection or interference).
- [x] `skopeo copy docker://<ref> oci-archive:<path>` confirmed working repeatedly throughout Part 1 (this is exactly what `aap2-image-send`'s export step already does).
- [x] **Acceptance met**, verified live (see above) — not just carried over from Part 1's own testing, re-confirmed fresh for this task list's specific concurrent-agents assumption.

Reading is not off — proceeding to Phase B on the existing `aap2-image-send`/`aap2-image-recv` crates from Part 1 (extended, not duplicated - see Phase B).

---

## Phase B — Rust sender: `aap2-image-sender` — DONE (2026-08-01)

- [x] New binary crate `rust/aap2-image-sender/` (workspace member, renamed from the Part 1 `aap2-image-send` to match the binary name the Go CLI now invokes). Takes `--image`, `--harbor-target`, `--release`, plus `--destination-eid`/`--socket`|`--tcp`/`--agent-id`/`--target-arch`/`--work-dir` as CLI args (see `pkg/helmdtnt/rust_image_transport.go` for how the Go CLI supplies these — Phase D).
- [x] `skopeo copy --override-os linux --override-arch <arch> docker://<image-ref> oci-archive:<tmp>/image.tar` (`rust/aap2-image-sender/src/export.rs`). Errors classified into distinct `ExportError` variants (`SkopeoNotInstalled`, `AuthOrImageNotFound`, `DiskFull`, `Timeout`, `Other`) via `aap2-common::skopeo`'s stderr classification, each surfaced with an actionable hint in `main.rs::explain_export_error`.
- [x] Metadata header (`TransferHeader`: `transfer_id`, `source_image_ref`, `target_image_ref`, `created_at_unix`, `archive_sha256`, `archive_size_bytes`, `release_name`) sent as the first frame via `encode_transfer_payload`, followed by the archive bytes, as a single AAP2 ADU to the same destination EID the Go chart transmission uses (`<destination-eid>/image-transfer`, see `imagesAgentEid` in `cmd/install.go`).
- [x] No retry/backoff/idempotency layer in `aap2-common::skopeo` per this task list's explicit non-goal — a single `skopeo` invocation with a timeout, classified failure, non-zero exit.
- [x] **Acceptance met, verified live** against the real Phase 1 harness (`docker compose up -d`, earth-node/space-node/Harbor all healthy): ran `aap2-image-sender --tcp 127.0.0.1:14242 --destination-eid dtn://space.dtn/image-transfer-<run> --image docker.io/library/alpine:3.20 --harbor-target library/alpine-p2test:3.20 --target-arch arm64 --release demo-release`. Exited 0 only after µD3TN acknowledged the send (`transfer accepted by µD3TN`), printing `transfer_id=...` and `sha256=...`.

---

## Phase C — Rust receiver: `aap2-image-receiver` — DONE (2026-08-01)

This is a new, separate, long-running process deployed on the remote/space side, alongside (not replacing) the existing Go `receiver.go` process.

- [x] New binary crate `rust/aap2-image-receiver/` (renamed from Part 1's `aap2-image-recv`). Registers its own AAP2 agent (`--agent-id`, default `image-transfer`, matching the ID the sender targets) against the local µD3TN node on the space side (`--socket`/`--tcp`), and loops in `run_connection` receiving header+archive pairs (`rust/aap2-image-receiver/src/main.rs`).
- [x] Per transfer (`rust/aap2-image-receiver/src/receive.rs::handle_transfer`): decode header, write archive to a temp dir, verify SHA-256 against the header before touching Harbor, then `skopeo copy --authfile <tmp>/auth.json oci-archive:<tmp>/image.tar docker://<harbor-host>/<target-image-ref>`. Harbor credentials come from the `HARBOR_CREDS=user:password` env var and are written to a temporary authfile, never passed as argv (not visible via `ps`).
- [x] After push, runs `skopeo inspect --format {{.Digest}} --authfile ...` and logs the resulting digest.
- [x] On failure (bad archive, checksum mismatch, skopeo error), `handle_transfer` returns `Err`, `run_connection`'s loop logs it (`tracing::error!`) with the transfer ID and continues to the next message — it does not `panic()`/exit, unlike the Go receiver's known issue documented in Phase 0. Simplified per this task list's explicit non-goal: no idempotency store, quarantine directory, or retry/backoff layer — a failed push is logged and the receiver moves on. (Part 1 had built exactly this framework for the old design; it was intentionally removed here per Part 2's scope.)
- [x] **Acceptance met, verified live**: ran `aap2-image-receiver --tcp 127.0.0.1:24242 --registry 127.0.0.1:8081 --insecure-registry --agent-id image-transfer-<run>` with `HARBOR_CREDS=admin:Harbor12345` against the real harness, then sent a transfer with the Phase B sender above. Receiver log showed `received image archive; pushing to Harbor` (with `release=demo-release` correlated via the shared `transfer_id`) followed by `transfer complete: image pushed and verified digest=sha256:45e09956...`. Independently confirmed via `skopeo inspect --tls-verify=false docker://127.0.0.1:8081/library/alpine-p2test:3.20`: same digest, `Architecture: arm64`, `Os: linux`.

---

## Phase D — Wire into the existing Go CLI and receiver — DONE (2026-08-01)

- [x] `cmd/install.go`/`cmd/upgrade.go`: after `helmdd.ResolveDependencies`, both unconditionally call `helmdtnt.SendImagesViaRust(dependencyPath, config, imagesAgentEid(config.DestinationEid), releaseName)` (one `aap2-image-sender` subprocess invocation per detected image, via `exec.Command` in `pkg/helmdtnt/rust_image_transport.go`), then `helmdl.DownloadNonImageDependencies` (git repos only) and `helmdp.BundleDependencies` for the chart-only archive. No `--image-transport` flag exists anywhere in the codebase (confirmed via repo-wide grep) — the Rust path is the only path, not a fallback.
- [x] `pkg/helmdp/helmdp.go::BundleDependencies` unconditionally runs `os.RemoveAll(filepath.Join(chartRoot, "Images"))` before archiving — the bundle `helmdtnt.SendChart` sends never contains image bytes.
- [x] `pkg/helmdtnt/receiver.go` contains no `tarball.ImageFromPath`/`remote.Write` anywhere (confirmed via repo-wide grep) — the `go-containerregistry` dependency is fully gone from `go.mod`. `registerInstallAgent`/`registerUpgradeAgent` only untar the chart-only archive and call `installChart`/`upgradeChart`.
- [x] No blocking coordination: `receiver.go` has a doc comment directly above `registerInstallAgent` explaining that images are pushed independently by `aap2-image-receiver`, that a chart may install before its image is visible in Harbor, and that this is intentional — Kubernetes' `ImagePullBackOff` retry is expected to recover once the image lands.
- [x] **Acceptance met, verified via build**: `docker run ... golang:1.24 sh -c "go build -o /tmp/helm-dtn-bin . && go vet ./..."` — both succeed with no errors, confirming the wiring compiles and there is no leftover reference to the old Go image-push path anywhere in the module.

---

## Phase E — Example chart — DONE (2026-08-01)

- [x] `examples/demo-chart/` exists: `Chart.yaml` (name `demo-chart`), `values.yaml` with `image.repository: docker.io/library/nginx`, `image.tag: alpine` (a plain, source-resolvable ref that `aap2-image-sender` can pull directly via skopeo), `templates/deployment.yaml` (one Deployment, `image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"`), `templates/service.yaml` (one ClusterIP Service on port 80), `templates/_helpers.tpl`.
- [x] Traced the rewrite path end-to-end against `pkg/helmdd`/`pkg/helmdp`/`pkg/helmdtnt` to confirm the image reference stays consistent between the two independent transports without any manual values override needed: `helmdd.extractDependencies` renders the chart, finds `image: docker.io/library/nginx:alpine` in the rendered manifest, and splits it into registry=`docker.io/library` / image=`nginx:alpine`. `SendImagesViaRust` sends `sourceRef=docker.io/library/nginx:alpine` and `--harbor-target nginx:alpine` (the same base `name:tag`, per its own comment about keeping both transports pointing at the identical final path). Separately, `helmdp.overrideRefs` walks `values.yaml` (a plain YAML file, not a template), finds the `image: {repository, tag}` mapping, and — since there's no `registry:` key — rewrites `repository` to `<configured-registry>/nginx`, leaving `tag: alpine` untouched. The rendered Deployment on the remote side therefore ends up as `<configured-registry>/nginx:alpine`, exactly matching `<configured-registry>/<harbor-target>`.
- [x] **Acceptance met**: `helm lint examples/demo-chart/` — 0 charts failed (only an informational "icon is recommended" note). `helm template demo examples/demo-chart/` renders a valid Deployment (`image: "docker.io/library/nginx:alpine"`) and Service. Full real-cluster `helm dtn install` is exercised in Phase F.

---

## Phase F — End-to-end test — DONE (2026-08-01)

Full setup, commands, and evidence in `deploy/demo-k8s/` (`README.md` + `output/`). Summary:

- [x] Stood up a real Kubernetes cluster (`kind`) network-joined to the Phase 1 harness (its node attached to Harbor's Docker network with a containerd insecure-registry mirror for `nginx:8080`), `aap2-image-receiver` running as a container on `space-node`'s network + Harbor's network, and the Go chart receiver running against the cluster via kind's internal kubeconfig. Ran `helm dtn install examples/demo-chart/ --destination-eid dtn://space.dtn/ --registry nginx:8080/library --tcp 127.0.0.1:14242` from the host (standing in for earth-node's side), using only existing/documented CLI flags.
- [x] Confirmed the full path live: `helmdd` detected `docker.io/library/nginx:alpine` → `aap2-image-sender` pulled it via skopeo and sent it over AAP2 → `aap2-image-receiver` pushed it into Harbor (`nginx:8080/library/nginx:alpine`) → in parallel the chart-only bundle (no `Images/` dir) went over the unmodified Go path → the Go receiver ran `helm install` → the pod came up `Running`. The very first run organically hit the ordering gap Phase D describes: the chart arrived and `helm install` ran before the (larger) image finished its journey, producing one real `ImagePullBackOff`/`BackOff` cycle in `kubectl get events`, which resolved itself within ~15s with no intervention once the image landed in Harbor — direct, live confirmation of Phase D's accepted behavior, not just a design assumption.
- [x] Re-ran with `deploy/local-test/scripts/netem.sh outage space-node` active *before* invoking `helm dtn install`: both the image send and the chart send were accepted locally by earth-node's µD3TN immediately (a local AAP2 send always succeeds regardless of far-end reachability — store-and-forward), confirmed nothing reached space-node for 15s while the outage held, then cleared the outage and confirmed both the image (pushed to Harbor, digest verified via `skopeo inspect` to match the first run's identical image content) and the chart (chart installed, pod `Running`) were delivered within seconds of the link coming back — with no code involved in the recovery beyond µD3TN's own store-and-forward and Kubernetes' own pull retry. Evidence: `deploy/demo-k8s/output/earth-side-install-during-outage.log`, `aap2-image-receiver.log`, `helm-dtn-receiver.log`, `kubectl-get-pods.txt`, `kubectl-describe-pod.txt` (`Pulled`/`Created`/`Started` events), `skopeo-inspect.json`.
- [x] **Two pre-existing bugs surfaced by this real test, unrelated to the image-transfer work itself, documented in `deploy/demo-k8s/README.md`:**
  1. **Fixed** (blocking - Phase F literally could not run without it): `pkg/helmdtnt/receiver.go` had no buildable entry point — it's `package helmdtnt` (shared with `SendChart`/`SendImagesViaRust`, imported as a library by `cmd/*.go`), but its `main()` was unexported, so nothing could build or call it despite `README.md`/`doc/example-deployment.md` describing it as a separately published binary. Fixed minimally: renamed to exported `RunReceiver()`, added a thin `cmd/receiver/main.go` (`package main`) that calls it. No logic changed.
  2. **Not fixed, confirmed real** (documented as follow-up in the report, per this task list's own non-goal guidance): the Go chart receiver's `panic(err)` on any Helm SDK error (the exact gap flagged in Phase 0/`00-aap2-notes.md`) crashed the *entire* receiver process — taking `install`, `upgrade`, and `uninstall` all down together — when triggered live by a genuine Helm error ("cannot re-use a name that is still in use", from reinstalling over release state that hadn't been cleaned up through Helm itself). This is exactly why the Rust image receiver was deliberately built to log-and-continue instead (Phase C) and is now confirmed as a live, reproducible failure mode for the Go side too, not just a theoretical one.
- [x] **Acceptance met**: one `helm dtn install examples/demo-chart/` command resulted in a running pod on the (kind-simulated) remote cluster pulling its image from Harbor, evidenced by logs and `kubectl get pods`/`skopeo inspect`, including a run where the image transfer was deliberately delayed by an injected outage and recovered once the image landed.

---

## Phase G — Report update — DONE (2026-08-01)

- [x] `doc/rust-image-transfer/REPORT.md` already described the corrected two-independent-agents architecture (per Phase B/C/D's work); added a new "Real Kubernetes end-to-end run" section under Verification with the Phase F results (both runs, both discovered issues, evidence file pointers) and a `deploy/demo-k8s` row in the verification table.
- [x] Phase F evidence included: baseline run (organic `ImagePullBackOff` recovery) and outage-injected run (µD3TN store-and-forward recovery), both summarized with pointers to `deploy/demo-k8s/output/` for full logs. The "Image and chart ordering" section's existing description of the accepted behavior is now backed by a live confirmation, not just a design statement.
- [x] Repo-wide grep confirms no remaining references to a "skopeo-subprocess-only" design or the old `aap2-image-send`/`aap2-image-recv` names outside of `TASKS.md` (Part 1's own historical log, accurately describing what was true at the time) and `DELETIONS.md`/`08-integration-decision.md`'s explicit "here's what changed" framing.
- [x] **Acceptance met**: the report accurately describes what's actually running (two independent AAP2 agents, images decoupled from the chart bundle, verified end-to-end against a real Kubernetes cluster including an outage-recovery run), with no leftover references to a superseded design.

---

## Explicit non-goals

- No blocking coordination between image arrival and chart install — Kubernetes's own retry handles the ordering gap.
- No idempotency tracking, quarantine directories, or retry/backoff layers beyond clear logging and a non-zero exit on failure.
- No changes to the µD3TN deployment itself — same nodes, same convergence layer config, just one more AAP2 agent on each side.
- No changes to how the chart itself is transmitted — that path is untouched Go code.
