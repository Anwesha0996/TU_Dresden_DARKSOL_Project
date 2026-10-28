# Phase 0 notes: AAP2 in the existing Go code, and ground truth for the Rust port

This document answers the Phase 0 questions from `TASKS.md`: how an AAP2 client
registers/sends/receives today in `helm-dtn`, where the Rust protobuf
definition should come from, whether a usable Rust AAP2 client already
exists, and what `skopeo`/Harbor versions to target.

## 1. How the existing Go AAP2 client works

Code: `pkg/aap2_client/{aap2_base_client.go, aap2_sync_client.go, aap2_tcp_client.go, aap2_unix_client.go, utils.go}`.

### Transport
Two concrete client types, both embedding `AAP2SyncClient` (which embeds
`AAP2BaseClient`):

- `AAP2TCPClient` — `net.Dial("tcp", address)`, used when `--tcp host:port` is given.
- `AAP2UnixClient` — `net.DialUnix("unix", nil, raddr)`, used when `--socket path` is given.

`helmdtnt.send()` (`pkg/helmdtnt/helmdtnt.go`) picks one or the other based on
whether a unix socket path was configured — there's no automatic fallback,
it's an explicit if/else on the caller side. The receiver (`pkg/helmdtnt/receiver.go`)
always uses TCP (`dtnServiceAddress:dtnServicePort`), read from env vars
(`DTN_SERVICE_ADRESS`, `DTN_SERVICE_PORT` — note the existing typo in the env
var name, kept as-is on the Go side, not something to replicate in the Rust
naming).

### Connection lifecycle
1. `Connect()` opens the socket and calls the private `welcome()`:
   - reads exactly 1 byte and checks it's the AAP2 magic number `0x2f`
     (`checkFirstByte`).
   - calls `ReceiveMsg()` to get the next length-delimited `AAPMessage`.
   - `processWelcome()` asserts it's a `Welcome` variant and stores
     `nodeEid` from `welcome.node_id`.
2. `Configure(opts ...AAP2ConfigurationOption)`:
   - functional-options pattern (`WithAgentId`, `WithSubscribe`, `WithSecret`,
     `WithAuthType`, `WithKeepalive` — see `utils.go`).
   - if no agent ID given, one is generated (`generateAgentId`): a random
     uint32 string for `ipn:` EIDs, a UUID for `dtn:` EIDs.
   - if no secret given, a UUID is generated as the secret. The **secret is
     what lets a second connection "claim"/resume the same registration** —
     this matters for the Rust port's reconnect/retry story, since a crashed
     receiver can reconnect with the same agent ID + secret and keep
     listening on the same endpoint rather than being assigned a new one.
   - builds a `ConnectionConfig` protobuf message, wraps it as
     `AAPMessage{Msg: &AAPMessage_Config{...}}`, sends it, and waits for an
     `AAPResponse` with `ResponseStatus_RESPONSE_STATUS_SUCCESS`.
   - returns the secret (generated or supplied) so the caller can reuse it.

### Wire framing
Everything after the initial 1-byte magic number is **length-delimited
protobuf**: a protobuf varint (max 10 bytes) giving the message length,
followed by that many bytes of a marshaled `AAPMessage` or `AAPResponse`.
`receiveDelimited()` reads the varint one byte at a time (checking the
continuation bit `0x80`) then reads exactly that many bytes. `Send()` does
the mirror image: marshal, prepend a varint of the length, write.

`BundleADU` payloads are **not** part of the length-delimited protobuf
envelope — the `AAPMessage_Adu` submessage only carries metadata
(`SrcEid`/`DstEid`/`PayloadLength`/flags); the actual bundle bytes are sent
as a **separate raw write** immediately following the ADU message
(`SendAdu`/`SendFile` do `Send(aapMsg)` then `io.Copy` the raw payload;
`ReceiveAdu` mirrors this by reading `aduMsg.PayloadLength` raw bytes off the
connection right after receiving the ADU message). This means:
- there is **no chunking at the AAP2 layer** — one `SendAdu` call puts the
  entire payload on the wire as a single contiguous write, and
  `ReceiveAdu` does a single `a.connection.Read(bundleData)` for the whole
  expected length. **Caveat for Phase 3:** Go's `net.Conn.Read` is not
  guaranteed to fill the buffer in one call — `ReceiveAdu` as written only
  does one `Read()`, not a loop, so it is not actually watertight against a
  large payload arriving in multiple TCP segments. The Rust receiver should
  do a proper `read_exact`-equivalent (loop until `PayloadLength` bytes are
  read) rather than copying this pattern verbatim. Flag this as a known
  correctness gap in the Go code, don't carry it into Rust (same spirit as
  the `panic()`-on-error note below).
- large images therefore arrive as one giant unbroken read on the receiver
  side; Phase 3's "confirm chunking/reassembly explicitly" acceptance
  criterion should be read as: confirm there is *no* protocol-level chunking
  (unlike, say, gRPC streaming) and that the Rust receiver correctly reads
  the full `PayloadLength` regardless of how many TCP segments it arrives in
  — not that AAP2 does its own chunking.

### The four methods asked about specifically
- **`Configure`**: see above — registers/authenticates the connection and
  requests an agent ID + optional subscription.
- **`SendAdu(aduMsg *aap2.BundleADU, bundleData []byte)`**: sends a bundle.
  Validates `aduMsg.PayloadLength == len(bundleData)` first. Wraps the ADU
  metadata in an `AAPMessage`, sends it length-delimited, then writes the raw
  payload bytes directly to the connection (no additional framing).
- **`ReceiveAdu(aduMsg *aap2.BundleADU)`**: if `aduMsg` is `nil`, first calls
  `ReceiveMsg()` and asserts the result is an ADU variant; then reads exactly
  `aduMsg.PayloadLength` raw bytes (see caveat above) and returns
  `(aduMsg, bundleData, error)`.
- **`ReceiveMsg`**: reads one length-delimited frame off the socket and
  unmarshals it as a generic `AAPMessage` (the `oneof` wrapper — could be
  `Welcome`, `Config`, `Adu`, `DispatchEvent`, `Link`, or `Keepalive`).
- **`SendResponseStatus(status aap2.ResponseStatus)`**: sends a bare
  `AAPResponse{ResponseStatus: status}`, length-delimited, no payload. Used
  by the receiver's agent loop to ACK every message it gets (it always sends
  `RESPONSE_STATUS_SUCCESS` regardless of whether processing the ADU
  afterward actually succeeds — see the resilience note below).

### `WithAgentId` / `WithSubscribe` / `WithSecret`
- **`WithAgentId(id string)`**: requests this specific agent/demux name
  (e.g. `"install"`) rather than letting µD3TN/the client generate one. The
  full EID becomes `<node-eid-prefix><agent-id>` (or `prefix.agentId` for
  `ipn:` EIDs) — see `Eid()`/`EidPrefix()` in `aap2_base_client.go`.
- **`WithSubscribe(bool)`**: if `true`, this connection becomes a
  *subscriber* — after CONFIGURE succeeds, µD3TN (the server) becomes the
  Initiator of subsequent messages (it pushes ADUs and Keepalives to the
  client) instead of the client driving the conversation. This is what the
  receiver side uses (`registerInstallAgent` etc. all pass
  `WithSubscribe(true)`) so it can sit in a loop waiting for inbound bundles.
  The *sender* side (`helmdtnt.send()`) does **not** subscribe — it's a
  one-shot outbound connection.
- **`WithSecret(secret string)`**: see above — lets a new connection
  authenticate as/resume a previous registration under the same agent ID.
  The receiver reads this from `/etc/secret-volume/token` (a Kubernetes
  secret volume) rather than generating one, specifically so all three
  agents (`install`/`upgrade`/`uninstall`) and any reconnects use a stable,
  shared secret.

### Known issue to flag, not replicate: `panic()`-on-error in the receiver loop
`pkg/helmdtnt/receiver.go`'s `registerInstallAgent`/`registerUpgradeAgent`/
`registerUninstallAgent` all do:
```go
aduMsg, err := installClient.ReceiveMsg()
if err != nil {
    panic(err)
}
```
inside their `for {}` loops, and there are further `log.Fatalf` calls in the
per-image push logic. A single malformed message, a transient read error, or
one bad image push kills the entire process — including the other two agent
goroutines, since a `panic()` in one goroutine that isn't recovered brings
down the whole program. This is exactly the failure mode Phase 6 calls out
("replicate the existing Go code's `panic()`-on-error pattern is explicitly
**not** acceptable"). Confirmed present, noted here for the Phase 6/10
writeup rather than fixed in Go (out of scope per the non-goals section).

There's also a second-order bug worth noting for context: `handleCommand()`
in the same file is dead code (never called — each `register*Agent` inlines
its own copy of the same logic instead), and there's a `// FIXME: For some
reason the handleCommand function does not actually handle incoming
commands. Needs investigation.` comment above each call site marking exactly
that. Not in scope to fix, but explains why the three register functions
look like copy-pasted near-duplicates instead of calling a shared helper.

## 2. Source of the AAP2 `.proto` definition

The compiled `pkg/proto/aap2.pb.go` is generated (`protoc-gen-go v1.36.10`,
`protoc v3.21.12`) from `aap2.proto`, which is **not vendored in this repo**.
It originates from the µD3TN project itself:

- **Repo:** `https://gitlab.com/d3tn/ud3tn` (GitLab, not GitHub — there is a
  GitHub mirror/topic page but GitLab is canonical).
- **Path:** `components/aap2/aap2.proto` (found via the GitLab repository
  tree API; confirmed by diffing message/enum names against
  `pkg/proto/aap2.pb.go` — `AAPMessage`, `Welcome`, `ConnectionConfig`,
  `BundleADU`, `AuthType` with identical enum values (`AUTH_TYPE_DEFAULT = 0`,
  `AUTH_TYPE_FIB_CONTROL = 1`, ...) match exactly).
- **Pinned tag:** `v0.15.0`, commit `b27c1e28a14ed3c75bf96890f793638fc2f168fb`
  (tagged 2026-05-19). This postdates the last actual content change to
  `aap2.proto` (a comment typo fix, commit `64e56e09`, 2026-05-13), so
  `v0.15.0` carries the current proto as-is.
- **License:** `SPDX-License-Identifier: BSD-3-Clause OR Apache-2.0` (the
  project re-licensed its Python modules and Protobuf definitions from a
  more restrictive license specifically "to be permissive concerning client
  code and alternative client implementations" — commit `7778067`, Aug 2025.
  This is a deliberate green light for exactly what we're doing).
- **Raw fetch URL for `build.rs`/vendoring:**
  `https://gitlab.com/d3tn/ud3tn/-/raw/v0.15.0/components/aap2/aap2.proto`
  (and its sibling `components/aap2/aap2.options`, which carries
  nanopb-specific field options for µD3TN's embedded C implementation — not
  needed for `prost`, but worth vendoring alongside for provenance).

**Action for Phase 2:** vendor a copy of this exact file into
`rust/aap2-common/proto/aap2.proto` (do not fetch it at build time — pin it
like a dependency) with a comment noting the source URL, tag, and commit
above, so a future upstream proto change doesn't silently change our wire
format.

## 3. Existing Rust AAP2 client — evaluated, not adopted

µD3TN does maintain an official-looking Rust implementation:
**`https://gitlab.com/d3tn/aap2-rs`** (same GitLab group as µD3TN itself,
`d3tn/aap2-rs`, Apache-2.0, "AAP2 Rust implementation").

Decision: **do not depend on it; hand-roll the client in
`rust/aap2-common` instead**, for concrete reasons:

1. **It doesn't implement bundle transfer.** `src/api/socket/write.rs` only
   implements `Sendable` for `AapMessage`, `AapResponse`, and
   `ConnectionConfig` — there is a literal `// TODO: Bundles` at the end of
   the file and nothing implements sending a `BundleADU` + payload. On the
   read side, `read_msg()` explicitly *rejects* anything that isn't a
   `Welcome` or `Link` variant (`"Expected AAPMessage to be Welcome or
   Link"`, with a `//TODO: Add Bundle and others too`). Since sending and
   receiving bundle ADUs is the entire point of Phases 3–5, this crate
   cannot be used as-is for the image transfer path — we would have to
   rewrite exactly the parts we need anyway.
2. **It's purpose-built for a different use case.** `Cargo.toml`'s
   `description` is *"Hermes library to share DTN routing information
   between a DTN and a BGP implementation"* — the `init()` API hardcodes
   `AuthType::FibControl` and an endpoint ID of the form
   `dtn://hermes/<svc_name>`. `src/types.rs` is full of BGP/CLA/routing-table
   types unrelated to image transfer. Adopting it would mean depending on
   (and working around) a large surface of code we don't need, for the ~120
   lines of socket framing we do need.
3. **Transport is UNIX-socket only** (`tokio::net::UnixStream` is
   hardcoded throughout `src/api/mod.rs` and `src/api/socket/*.rs`). Our
   TASKS.md explicitly wants both TCP and Unix-socket transport (mirroring
   `aap2_tcp_client.go`/`aap2_unix_client.go`), and the Phase 1 Docker
   Compose harness will most naturally expose AAP2 over TCP between
   containers. Adding TCP support would mean forking most of the crate
   anyway.
4. **Explicitly labelled experimental with no stability promise.** The
   README states verbatim: *"This is WIP and experimental. Please reach out
   to contact (at) d3tn (dot) com in case of any questions about the status,
   technical details, or roadmap."* Not published to crates.io (would need a
   git dependency pinned to a commit); last repository activity is
   2025-08-01, exactly a year stale as of this writing (2026-08-01).

**What we do take from it:**
- Confirms `prost` + `tokio` + `async-trait` is the natural/idiomatic stack
  for this (same choice TASKS.md Phase 2/3 already specifies) — useful
  external validation of the approach.
- Its length-delimited read loop (`read_varint`, manually decoding the
  protobuf varint continuation bit) in `src/api/socket/read.rs` is a clean,
  correct small reference for the same varint-framing logic
  `aap2_sync_client.go`'s `receiveDelimited()` implements in Go — worth
  glancing at again during Phase 3 implementation, but not worth importing
  as a dependency.
- Confirms the magic init byte is `0x2F` (`pub const INIT_BYTE: u8 = 0x2F`),
  consistent with `checkFirstByte` in the Go code (`0x2f`).

If d3tn/aap2-rs is revisited later (e.g. bundle support lands upstream),
re-evaluate — but for this project, building our own minimal, fully-featured
(TCP + Unix, full ADU send/receive, generic response-status) client in
`aap2-common` is the pragmatic choice.

## 4. `skopeo` and Harbor versions to target

Checked via GitHub/GitLab release APIs on 2026-08-01 (not assumed):

- **skopeo:** latest upstream tag is `v1.24.0` (released 2026-07-30, one day
  before this check — likely too fresh to be packaged everywhere yet).
  `v1.23.0` (2026-05-26) is the more conservative pin and is already in
  Arch's `extra` repo. **Recommendation: target `v1.23.0`**, and build/run it
  inside containers (via `quay.io/skopeo/stable` or a Dockerfile installing a
  pinned skopeo build) rather than depending on a host install, since none of
  the tested environments are guaranteed to have it — see environment note
  below.
- **Harbor:** latest release is `v2.15.2` (2026-07-02); `v2.15.0`
  (2026-03-20) added a tag-deletion option for GC. **Recommendation: target
  `v2.15.2`** for the Phase 1 local Harbor instance, installed via Harbor's
  own offline/online installer (`harbor-offline-installer-v2.15.2.tgz` from
  `github.com/goharbor/harbor/releases`), which generates its own
  Compose config from `harbor.yml` rather than being a plain
  hand-written `docker-compose.yml` — Phase 1's compose file will need to
  either wrap/include Harbor's generated compose output or reference it as a
  sibling stack, not hand-roll Harbor's service definitions from scratch.

**Environment note (resolved 2026-08-01):** this development machine
initially had no Docker, Docker Compose, or local `skopeo` install. Docker
Desktop (`docker-desktop` cask, v4.84.0) has since been installed via
Homebrew and launched; `docker` v29.6.2 and `docker compose` v5.3.1 are now
functional (`docker info` succeeds, daemon reachable at
`desktop-linux` context). `skopeo` is still not installed on the host —
per the recommendation above, it will run inside containers rather than
depending on a host install, so this is not a blocker.

## Summary answers (for the Phase 0 acceptance criterion)

- **How does an AAP2 client register, send, and receive today?** Connect →
  read magic byte `0x2f` → receive WELCOME → send CONFIGURE (agent ID,
  subscribe flag, secret, auth type, keepalive) → receive success
  AAPResponse. Sending: marshal `AAPMessage{Adu: ...}` length-delimited, then
  write the raw payload bytes directly after it, no further framing.
  Receiving: read one length-delimited `AAPMessage`, if it's an ADU variant
  read exactly `PayloadLength` more raw bytes off the same connection.
  Responses to any received message are sent back as a bare
  `AAPResponse{ResponseStatus: ...}`, length-delimited, no payload.
- **Where does the Rust protobuf definition come from?** Vendor
  `components/aap2/aap2.proto` from `gitlab.com/d3tn/ud3tn` at tag `v0.15.0`
  (commit `b27c1e28a14ed3c75bf96890f793638fc2f168fb`), BSD-3-Clause/Apache-2.0
  licensed, generate with `prost-build` in `aap2-common/build.rs`. Do not
  depend on `gitlab.com/d3tn/aap2-rs` — reasons in §3 above.
