# Interim Space-Network Image-Transfer Results

**Snapshot:** 7 September 2026 (72 of 90 planned runs recorded)

## What was evaluated

This experiment evaluates the existing standalone image-transfer implementation. It does not install Talos or create a Kubernetes cluster, and it does not replace the existing sender or receiver.

The tested route is:

```text
Earth source registry
  -> existing Go image sender
  -> OCI image layout and TAR payload
  -> one AAP2 ADU
  -> Earth micro-D3TN
  -> BPv7 over TCPCL
  -> Space micro-D3TN
  -> existing Go receiver
  -> SHA-256 verification
  -> destination Harbor registry
  -> source/destination digest comparison
```

The representative test image is `ghcr.io/siderolabs/flannel:v0.27.4`. Its OCI/AAP2 payload is 34,145,679 bytes (34.146 MB). The current experiment is the confirmed 90-run representative-image matrix, not the unconfirmed 900-run matrix across all ten Talos/Kubernetes images.

## Network profiles

| Scenario | Configured RTT | One-way delay | Bandwidth | Packet loss |
|---|---:|---:|---:|---:|
| LEO | 50 ms | 25 ms | 6 Mbit/s | 0% |
| Lunar | 2.6 s | 1.3 s | 20 Mbit/s | 0% |
| Mars | 1,800 s (30 min) | 900 s | 5 Mbit/s | 0% |

### How the referenced IMC paper was used

The supplied `imc2026-cycle2-paper200.pdf`, *Towards Characterizing Solar System Networking: Analyzing the DSN Now Data Set*, is used as the latency guide, not as the source of every network input:

- Its observed Lunar examples are approximately 2.4–2.7 seconds RTT. The experiment fixes one reproducible value in that range: **2.6 seconds RTT**.
- Its observed Mars examples are approximately 29–40 minutes RTT. The experiment fixes a reproducible representative value: **30 minutes (1,800 seconds) RTT**.
- It does not define a LEO profile, because the analysed DSN data concerns deep-space contacts. The **50 ms LEO RTT** is therefore sourced separately from a Starlink measurement study.
- The paper does not provide sufficient DSN uplink-rate data to select all experiment bandwidths. The Lunar, Mars, and LEO bandwidth values are therefore sourced separately and are clearly labelled in `network_scenarios.yaml`.
- The exact interruption durations **10 seconds, 5 minutes, and 1 hour** are controlled experimental choices spanning short, medium, and long outages. They must not be claimed as exact recommendations from the paper.

This separation is important: the experiment follows the paper's network-characterisation approach and uses its Lunar/Mars latency observations, while filling unsupported parameters with cited external sources. The parameters are fixed across repetitions so that runs remain comparable.

For a separate higher-bandwidth LEO sensitivity test, `network_scenarios.yaml`
also defines **15 Mbit/s uplink**. This is the midpoint of the 10–20 Mbit/s
median Starlink return-link goodput reported by Bülo et al., *Crowdsourced
Starlink Performance Measurements from starlinkstatus.space*, IEEE ICC 2024,
DOI `10.1109/ICC51166.2024.10622394`. It must be labelled separately and must
not be mixed with the 6 Mbit/s baseline results.

Traffic shaping is applied only between the Earth and Space micro-D3TN nodes. Registry download traffic is not intentionally limited. Each scenario uses network interruptions of 10 seconds, 300 seconds (5 minutes), and 3,600 seconds (1 hour), with ten intended repetitions per condition.

Latency validation measured 50.6 ms for the 50 ms LEO profile and 2,600 ms for the 2.6 s Lunar profile. A complete Mars RTT could not be measured through the present TCPCL connection because TCP connection establishment times out first.

## Interim results

| Scenario | Interruption | Runs | Successful | Failed | Interpretation |
|---|---:|---:|---:|---:|---|
| LEO | 10 s | 10 | 10 | 0 | Delivery recovered after the short outage. |
| LEO | 5 min | 10 | 10 | 0 | Delivery recovered after reconnection. |
| LEO | 1 hour | 10 | 0 | 10 | Receiver completion timeout after the long outage. |
| Lunar | 10 s | 10 | 10 | 0 | Delivery recovered despite the configured 2.6 s RTT. |
| Lunar | 5 min | 10 | 10 | 0 | Delivery recovered after reconnection. |
| Lunar | 1 hour | 10 | 0 | 10 | Receiver completion timeout after the long outage. |
| Mars | 10 s | 10 | 0 | 10 | TCPCL connection timed out before bundle transfer began. |
| Mars | 5 min | 2 | 0 | 2 | Interim; the remaining matrix is still running. |
| Mars | 1 hour | 0 | 0 | 0 | Not started at this snapshot. |

Successful runs also completed source/destination digest verification. Failed runs have no destination digest because delivery and Harbor push did not complete; this must not be described as a digest mismatch or corrupted image.

## Why Mars fails

TCPCL is the **TCP Convergence Layer** used to carry BPv7 bundles between the two micro-D3TN nodes. TCP must first finish its connection handshake. With 900 seconds of delay in each direction, the configured Mars RTT is 1,800 seconds. The operating system's TCP connection attempt times out after roughly two minutes, long before one realistic Mars RTT can complete. Therefore, no BPv7 image delivery starts.

This is a limitation of applying the full end-to-end Mars propagation delay directly to a conventional TCPCL connection. It is not a failure of OCI packaging, the AAP2 sender, SHA-256 verification, or Harbor.

A reduced-latency Mars feasibility experiment could show that the remaining route works when TCP can connect, but it must be kept separate and must not replace the realistic 1,800-second-RTT results.

## Meaning of the one-hour failures

All completed one-hour LEO and Lunar runs ended with `receiver_completion_timeout`. The 10-second and 5-minute results show that the present DTN route can recover from shorter temporary outages. The one-hour results show that the current implementation, timeout configuration, contact handling, or retained TCPCL state is not sufficient for this longer outage. Additional protocol-log analysis is needed before attributing the failure to one specific component.

## Measurements saved for every run

- image name and source/destination references
- payload size
- start time, finish time, and wall-clock duration
- sender peak CPU and memory
- receiver sampled CPU and memory and their peaks
- configured bandwidth, RTT, and packet loss
- effective payload throughput
- interruption start, requested duration, and reconnection time
- completion after reconnection
- source and destination digest and digest match
- success/failure and error message
- one detailed log file per run

## Important limitations

1. The matrix uses one representative image. Whether the same 90 conditions must be repeated for all ten Talos/Kubernetes images, producing 900 runs, still requires confirmation.
2. Packet loss is currently 0%; delay, bandwidth, and complete link interruption are emulated.
3. Bandwidth and latency are configured link conditions. Effective application throughput is calculated from payload size divided by total duration, so it includes outage and protocol waiting time.
4. Sender CPU is a run-level peak. Receiver CPU and memory are sampled over time. A sender CPU time series was not collected, so the two CPU measurements must not be presented as identical sampling methods.
5. Laptop sleep/hibernation paused WSL during some long experiments. Those wall-clock-duration outliers include host suspension and are not directly comparable to continuously awake runs. They remain in the raw data and have not been replaced with fake values.
6. A failed transfer has no destination digest. This means verification was not reached, not that a transferred image was corrupted.
7. Mars results characterize the current TCPCL setup under literal end-to-end Mars RTT. They do not prove that BPv7/DTN in general cannot operate over Mars links.
8. The 90-run matrix was still running at this snapshot; summary charts must be labelled **interim** until all conditions finish.

## Graphs to show

The preferred paper-style figures are available in both PNG and SVG under
`results/graphs/paper_style/`:

- `fig01_network_profiles`: RTT, one-way delay, and bandwidth with units
- `fig02_success_rate_matrix`: direct LEO/Lunar/Mars comparison for all common interruption durations
- `fig03_success_duration_distribution`: individual successful runs plus mean and standard deviation
- `fig04_failure_time_distribution`: failure timing kept separate from successful transfer duration
- `fig05_success_throughput_distribution`: successful effective throughput in Mbit/s
- `fig06_sender_receiver_cpu`: sender and receiver CPU clearly distinguished
- `fig07_sender_receiver_memory`: sender and receiver memory clearly distinguished

`N/A` means no applicable completed result and is never plotted as zero. Each
distribution figure shows the individual repetitions and identifies the mean
and one standard deviation, following a research-paper presentation rather
than showing only aggregate bars.

- [Average transfer duration](results/graphs/scenario_duration_comparison.svg)
- [Effective throughput](results/graphs/scenario_throughput_comparison.svg)
- [Success count](results/graphs/scenario_success_comparison.svg)
- [Peak sender CPU](results/graphs/scenario_sender_cpu_comparison.svg)
- [Peak receiver memory](results/graphs/scenario_receiver_memory_comparison.svg)
- [LEO 10-second detailed timeline](results/graphs/timeline_flannel_leo_interrupt10s_run10.svg)
- [LEO 5-minute detailed timeline](results/graphs/timeline_flannel_leo_interrupt300s_run10.svg)
- [Lunar 10-second detailed timeline](results/graphs/timeline_flannel_lunar_interrupt10s_run10.svg)
- [Lunar 5-minute detailed timeline](results/graphs/timeline_flannel_lunar_interrupt300s_run10.svg)

In each detailed timeline, the x-axis is seconds after transfer start and the red shaded region is the interruption. The bandwidth trace drops to zero during the outage. The receiver CPU and resident-memory traces show sampled process behaviour. Configured RTT and bandwidth must not be described as continuously measured application performance.

## Short explanation for the meeting

> I evaluated the existing OCI image-transfer route over BPv7/TCPCL using controlled LEO, Lunar, and Mars link profiles. I applied 10-second, 5-minute, and 1-hour interruptions only between the Earth and Space micro-D3TN nodes. Short and five-minute LEO and Lunar interruptions recovered successfully and preserved the image digest. One-hour interruptions reached the receiver timeout. Under a literal 30-minute Mars RTT, TCPCL could not establish TCP before the connection timeout, so bundle transfer never began. These failures are preserved as experimental results, and the graphs and raw logs clearly distinguish configured link conditions, measured resource use, successful digest verification, and incomplete delivery.
