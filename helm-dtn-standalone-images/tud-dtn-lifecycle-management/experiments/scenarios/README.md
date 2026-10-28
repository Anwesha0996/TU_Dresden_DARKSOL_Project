# LEO, Lunar, and Mars network scenarios

These experiments reuse the existing standalone OCI image-transfer route. They
shape only traffic between the Earth and Space µD3TN peers. Pulling images from
`registry.k8s.io` or `ghcr.io`, host-side AAP2, and the destination registry are
not intentionally rate-limited.

## Fixed profiles

| Scenario | Configured RTT | Per-direction delay | Rate | Basis |
|---|---:|---:|---:|---|
| LEO | 50 ms | 25 ms | 6 Mbit/s | Starlink stationary RTT and TCP upload measurement |
| Lunar | 2.6 s | 1.3 s | 20 Mbit/s | DSN paper RTT and NASA LLCD uplink |
| Mars | 30 min | 15 min | 5 Mbit/s | DSN paper RTT and conservative JPL optical-link profile |

The Mars rate is a documented emulation choice based on a high-rate optical
profile, applied symmetrically for tractable OCI experiments. It is not a
measured Mars command uplink; real command uplinks can be only kbit/s.

Packet loss is initially fixed at 0% so the first comparison varies only
propagation delay, bandwidth, and interruption. Packet loss can be a separate
follow-up factor after the base matrix is validated.

## Interruption durations

The same durations are applied to every scenario:

- short: 10 seconds
- medium: 5 minutes (300 seconds)
- long: 1 hour (3600 seconds)

The trigger waits for the sender's `sending ... as one AAP2 ADU` event, meaning
the OCI payload has been created and AAP2 submission is starting. This is
better than the historical pull-based trigger, but it still does not prove
that TCPCL has transmitted a particular number of payload bytes before the
firewall rule takes effect.

## Safe validation order

```bash
# One image, one scenario, shortest interruption, one repetition
bash experiments/scripts/run_scenario_single.sh pause leo 10 1

# Only after validating the log and CSV, run all nine conditions once
bash experiments/scripts/run_scenario_matrix.sh pause 1

# Then run ten repetitions for the selected image
bash experiments/scripts/run_scenario_matrix.sh pause 10
```

Do not start a 10-image matrix until the professor confirms whether the scope
is 90 runs for one representative image or 900 runs for all ten images.

Results are written to `scenarios/results/scenario_results.csv`; detailed logs
are grouped under `scenarios/logs/<scenario>/`.

## Sources

- Supplied paper: *Towards Characterizing Solar System Networking: Analyzing
  the DSN Now Data Set*, Paper #200.
- Ma et al.: *Network Characteristics of LEO Satellite Constellations: A
  Starlink-Based Measurement from End Users*.
- NASA: Lunar Laser Communications Demonstration (LLCD).
- JPL: *Deep-Space Optical Communications Downlink Budget from Mars*.

