# Speaker notes — Helm-DTN OCI image-transfer experiments

## Slide 1 — Title

This work evaluates the existing standalone OCI image-transfer route under
intermittent LEO, Lunar and Mars-style contacts. The central question is whether
required Talos/Kubernetes images can reach a Space-side registry intact when a
continuous end-to-end network path cannot be assumed.

## Slide 2 — Motivation

A Space-side cluster may be isolated from public registries. It still requires
its networking, DNS, control-plane and node images. Ordinary image pulls assume
continuous connectivity; DTN instead targets eventual delivery across delay and
disruption.

## Slide 3 — Scope

I did not create the Kubernetes cluster or rewrite the transfer software. I
identified the required images and evaluated the existing implementation using
repeatable measurements and digest verification. Missing results were not
fabricated.

## Slide 4 — Why Helm-DTN

Helm itself does not transfer an image. It provides the repeatable deployment
and configuration around µD3TN. The existing Go sender/receiver perform image
pull, OCI packaging, extraction and registry push. AAP2/BPv7/µD3TN/TCPCL carry
the ADU.

## Slide 5 — Architecture

The sender pulls from `registry.k8s.io` or `ghcr.io`, creates an OCI layout and
TAR, adds metadata, and submits one AAP2 ADU. Earth µD3TN stores/routes the BPv7
bundle. When contact is active, TCPCL transfers it to Space µD3TN. The receiver
verifies, extracts, pushes to the Space registry and compares digests.

## Slide 6 — Image selection

The set comes from the Talos mirroring requirement: nine Kubernetes runtime
images plus the Talos installer. This distinction explains why someone may say
“nine cluster images” while the configured transfer matrix contains ten total.

## Slide 7 — Method

Each condition targets ten successful repetitions. The link between the two
µD3TN nodes is interrupted without killing the sender or receiver. Every run
records payload, duration, CPU, RSS, throughput, interruption/reconnection,
digests, success and error details.

## Slide 8 — Scenario choice

LEO, Lunar and Mars represent increasingly difficult communication conditions.
Bandwidth and RTT are independent: bandwidth describes capacity, whereas RTT
mainly reflects propagation distance. Hardware and link-budget assumptions can
make one profile’s bandwidth higher even when its RTT is longer.

## Slide 9 — Reference versus applied RTT

LEO applied 50 ms RTT and Lunar applied 2.6 s RTT. Mars retained 1800 s as a
reference but applied zero propagation delay. Therefore Mars results must be
called contact-only; they evaluate 5 Mbit/s bandwidth, disruption and recovery,
not physical Mars propagation.

## Slide 10 — Contact window

The contact pattern is controlled: submit the ADU, remove the TCPCL path for 10
seconds, 5 minutes or 1 hour, restore it, and observe delivery. This is not a
real satellite pass or DSN schedule. Actual timestamps are retained per run.

## Slide 11 — Coverage

This is a live snapshot. Some images have complete or nearly complete matrices,
whereas later images have not yet been reached. I show recorded attempts rather
than claiming completion for all ten images.

## Slide 12 — Performance results

Mean total duration rises with longer outages. Effective throughput falls
because it is payload divided by the complete wall-clock duration, including
waiting. The success plot includes every attempt; it does not hide failures.

## Slide 13 — Why results look this way

Longer duration does not mean corrupted transfer. It means the bundle waited
for contact or for operational recovery. Large error bars mainly reflect WSL
suspension and variable reconnection. A matching final digest proves that the
delivered OCI content is intact.

## Slide 14 — TCPCL

TCPCL works well when an IP path exists and the handshake completes within
timeouts. It becomes difficult across long outages or extreme RTT. A key race
was found: recalling SQLite before TCPCL was active could make µD3TN report an
inactive link. Waiting 15 seconds after contact configuration fixed that race.

## Slide 15 — Persistent DTN

Persistent store-and-forward separates application submission from contact
availability. The sender submits once; Earth retains the original bundle; after
contact restoration the storage agent recalls it. This is the important DTN
behavior demonstrated by successful post-reconnection runs.

## Slide 16 — Limitations and requirements

WSL suspension pauses Linux sleep timers, so “one hour” can become many hours
of wall time. Mars RTT was not applied. CPU/RSS values belong to the local host,
not flight hardware. The single large ADU does not demonstrate partial
application-level resume. These define the next experiments.

## Slide 17 — Conclusion

The system demonstrates eventual, digest-verified OCI delivery across
controlled disruptions. TCPCL provides transport when contact exists;
persistent BPv7/µD3TN storage bridges the periods when it does not. The correct
claim is reliable eventual delivery under recorded applied conditions—not
minimum latency or full physical Mars emulation.

## Slide 18 — Evidence

Point to the raw CSV, per-run logs, scenario configuration, scripts and graphs.
The repository evidence makes the presentation reproducible and allows every
success or failure statement to be traced to a real run.
