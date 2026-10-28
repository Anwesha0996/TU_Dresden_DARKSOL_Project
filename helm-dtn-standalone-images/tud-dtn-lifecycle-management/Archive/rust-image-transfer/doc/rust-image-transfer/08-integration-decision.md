# Integration decision: Rust owns image AAP2 transport

## Decision

`helm dtn install` and `helm dtn upgrade` always use the Rust image channel.
There is no image-transport selector and no Go fallback that embeds image
archives in the chart bundle.

After `helmdd` detects dependencies, the Go CLI invokes
`aap2-image-sender` once per image. The sender uses `skopeo` to create an OCI
archive and sends a metadata header plus the archive through its own AAP2
client to `<destination-eid>/image-transfer`. The chart is rewritten to use
the destination registry, packaged without an `Images/` directory, and sent
unchanged through the existing Go chart-transfer agent.

On the remote side, `aap2-image-receiver` is a separate long-running process
alongside the Go lifecycle receiver. It subscribes to the `image-transfer`
agent, verifies the received archive checksum, pushes it to Harbor with
`skopeo`, and confirms the pushed digest with `skopeo inspect`. Harbor
credentials come from `HARBOR_CREDS=user:password`; the receiver writes a
temporary registry auth file so the password is not exposed in subprocess
arguments.

## Ordering

Image and chart bundles are independent messages on the same pair of µD3TN
nodes. Either can arrive first. The Go receiver does not coordinate with the
Rust receiver before running Helm. If the chart arrives first, Kubernetes may
show `ImagePullBackOff` until the image reaches Harbor, then recover through
its normal image-pull retry behavior.

## Deliberate limits

There is no idempotency database, quarantine directory, application retry or
backoff layer, or cross-channel completion protocol. A malformed transfer or
failed `skopeo` operation is logged with its transfer ID and discarded; the
receiver remains alive for the next AAP2 message. Connection failures cause
the long-running receiver to reconnect to its local µD3TN daemon.
