# Source and dependency provenance

## Project baseline

The baseline was supplied as `helm-dtn-main (2).zip` and declares the module:

```text
github.com/darksol-cloud/helm-dtn
```

The exact supplied ZIP contents are committed unchanged on local branch `main`
at commit `ad39896`. Standalone-image work is isolated on branch
`feature/standalone-image-transfer` so that `git diff main...HEAD` shows only
the proposed feature.

The supplied archive contains no `LICENSE` file. Before public redistribution
or an upstream pull request, the project owner must confirm the repository's
license and contribution policy. This implementation does not guess or assign a
license to upstream code.

## Reused project code

The following code comes from the supplied Helm-DTN baseline:

- `pkg/aap2_client`: AAP2 TCP/Unix clients and message framing.
- `pkg/proto/aap2.pb.go`: generated AAP2 Protocol Buffer types. Its generated
  descriptor names the upstream Go package
  `gitlab.dtnsat.de/aap2-client-go/thirdparty/proto`.
- `pkg/helmdtnt`: the original chart-oriented DTN transmission and receiver
  logic, retained unchanged by the standalone commands.
- The existing `go.mod` dependency set.

The feature does not implement a new AAP2 protocol library.

## Direct libraries used by the standalone path

| Module | Version | Existing in ZIP | Use in this feature | License |
|---|---:|---:|---|---|
| `github.com/google/go-containerregistry` | `v0.20.7` | Yes | Registry pull/push and OCI image layout | Apache-2.0 |
| `github.com/google/uuid` | `v1.6.0` | Yes | Transfer identifiers | BSD-3-Clause |
| `google.golang.org/protobuf` | `v1.36.9` | Yes | Existing AAP2 serialization | BSD-3-Clause |
| Go standard library | Go `1.24.2` module target | Yes | TAR, JSON, SHA-256, files, TCP streaming | Go license |

No new third-party Go module was added.

## External services and protocols

- µD3TN provides AAP2, Bundle Protocol processing, routing, storage and
  forwarding. It is external software, not embedded or copied into this feature.
- The source and destination services implement the OCI/Docker Registry HTTP
  API V2. Harbor is a compatible production destination.
- OCI image-layout and image-manifest formats are used through
  `go-containerregistry`.

## Provisioning image-set source

`examples/talos-v1.13.6-kubernetes-v1.36.2-images.txt` is transcribed from the
official Talos v1.13.6 release notes:

https://github.com/siderolabs/talos/releases/tag/v1.13.6

The list is intentionally version-pinned. It is an example acceptance set;
operators must generate/confirm the set for the exact Talos version they deploy.
