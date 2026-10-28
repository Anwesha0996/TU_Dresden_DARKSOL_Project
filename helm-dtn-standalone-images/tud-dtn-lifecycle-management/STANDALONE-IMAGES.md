# Standalone OCI image transfer

This feature is a minimal extension of the existing Go-based `helm-dtn`
implementation. It transfers one or more OCI container images without a Helm
chart and stores them in a registry beside the remote/Space node. It is intended
for transferring the Talos and Kubernetes images required before a Kubernetes
cluster exists.

## Scope

```text
source registry -> Go OCI archive -> existing AAP2 client -> Earth µD3TN
-> Bundle Protocol -> Space µD3TN -> Go receiver -> destination registry
```

The registry is the terminal destination. This feature does not require Helm,
a Helm chart, kubectl, or a running Kubernetes cluster.

## Provenance

The AAP2 implementation in `pkg/aap2_client` and generated AAP2 messages in
`pkg/proto` are existing `helm-dtn` code. This feature does not create a new
AAP2 library. OCI operations use the `github.com/google/go-containerregistry`
module already declared by the original project. `google.golang.org/protobuf`
and `github.com/google/uuid` were also existing direct dependencies.

No new third-party Go module is required.

## Build

```bash
go build -o bin/image-transfer-send ./cmd/image-sender
go build -o bin/image-transfer-receive ./cmd/image-receiver
go build -o bin/image-transfer-verify ./cmd/image-verify
```

## Space side

For an authenticated Harbor or Registry V2 endpoint:

```bash
REGISTRY_CREDS='user:password' ./bin/image-transfer-receive \
  -tcp 127.0.0.1:24242 \
  -agent-id image-transfer \
  -registry harbor.space.example/provisioning
```

For an anonymous local test registry, omit `REGISTRY_CREDS`. Use
`-insecure-registry` only for a plain-HTTP development registry.

## Earth side: one image

```bash
./bin/image-transfer-send \
  -tcp 127.0.0.1:14242 \
  -destination-eid dtn://space.dtn/image-transfer \
  -image registry.k8s.io/pause:3.10 \
  -registry-target kubernetes/pause:3.10
```

If `-registry-target` is omitted, the source repository path and tag are
preserved without the source registry hostname.

## Earth side: several images

Repeat `-image` and, when explicit destination names are needed, repeat
`-registry-target` in the same order:

```bash
./bin/image-transfer-send \
  -tcp 127.0.0.1:14242 \
  -destination-eid dtn://space.dtn/image-transfer \
  -image registry.k8s.io/pause:3.10 \
  -image registry.k8s.io/coredns/coredns:v1.12.1
```

Alternatively, use a text file. Empty lines and lines beginning with `#` are
ignored. Each entry is either `SOURCE` or `SOURCE=TARGET`:

```text
registry.k8s.io/pause:3.10=kubernetes/pause:3.10
registry.k8s.io/coredns/coredns:v1.12.1=kubernetes/coredns:v1.12.1
```

```bash
./bin/image-transfer-send \
  -tcp 127.0.0.1:14242 \
  -destination-eid dtn://space.dtn/image-transfer \
  -images-file examples/kubernetes-images.txt
```

For a complete pinned provisioning example, use:

```bash
./bin/image-transfer-send \
  -tcp 127.0.0.1:14242 \
  -destination-eid dtn://space.dtn/image-transfer \
  -images-file examples/talos-v1.13.6-kubernetes-v1.36.2-images.txt
```

That file follows the image list published for Talos v1.13.6 and Kubernetes
v1.36.2. Always confirm the list for the exact Talos version being provisioned.

The independent verifier compares every source manifest digest with the
destination registry manifest digest:

```bash
REGISTRY_CREDS='user:password' ./bin/image-transfer-verify \
  -images-file examples/talos-v1.13.6-kubernetes-v1.36.2-images.txt \
  -registry harbor.space.example/provisioning
```

For the repository's existing local µD3TN harness, a reproducible acceptance
script is provided. It does not create or use a Kubernetes cluster:

```bash
./scripts/run-local-acceptance.sh
```

The script configures a one-hour test contact at 10,000,000 bytes/second. The
rate matters: the original harness default of 100,000 bytes/second provided only
about 360 MB of total contact capacity, which is insufficient for the complete
provisioning set and can reject a single large image once capacity is consumed.
Production contact plans must budget for the complete encoded payload volume.

Each image is transferred independently as one AAP2 ADU. The implementation
streams files to avoid holding an entire image in memory; it does not implement
application-level chunking or partial resume.

## Payload and integrity

Each ADU payload is:

```text
[8-byte big-endian JSON length][JSON transfer header][OCI image-layout TAR]
```

The header contains the source, source manifest digest, target, UUID, creation
time, archive byte size, and archive SHA-256. The receiver recomputes size and
SHA-256 before opening the OCI layout and pushing it to the destination
registry. After the push, it reads the destination manifest back and requires
its digest to match the transferred image digest.

## Completion semantics

The Earth sender's success means the local Earth µD3TN accepted an ADU. In a
one-way DTN, that is not proof that the remote registry push completed. Final
success is recorded by the Space receiver only after registry read-back digest
verification. A return-status bundle would require a configured reverse DTN
path and is intentionally outside this one-way first stage.

## Relevant changes

- `cmd/image-sender`: standalone and multi-image command.
- `cmd/image-receiver`: registry-only Space-side receiver.
- `pkg/imagetransfer`: OCI export, framing, validation, and registry push.
- `pkg/aap2_client/aap2_sync_client.go`: reuses the existing `SendFile`, adds
  its missing length check, fixes exact-length reads, and adds the corresponding
  disk-streaming receive method.
- `examples/kubernetes-images.txt`: provisioning-image list example.

## Verified first provisioning image

The first live proof transferred:

```text
source: registry.k8s.io/pause:3.10
target: library/kubernetes/pause:3.10
payload: 324,390 bytes
destination digest: sha256:7c38f24774e3cbd906d2d33c38354ccf787635581c122965132c9bd309754d4a
```

The transfer used the existing AAP2/µD3TN path and ended at the destination
registry; no Kubernetes cluster was involved.
