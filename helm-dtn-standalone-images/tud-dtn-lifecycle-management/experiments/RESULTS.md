# Ten-image standalone OCI transfer evaluation

## Scope

This evaluation reuses Helm-DTN's existing route:

```text
source registry → Go sender → OCI layout/TAR → AAP2 → Earth µD3TN
→ BPv7/TCPCLv3 → Space µD3TN → Go receiver → Space registry
→ source/destination digest comparison
```

The experiment scripts do not replace the existing sender, receiver, OCI
packaging, AAP2, µD3TN, registry-push, or digest-verification implementation.
They configure runs, introduce interruption, collect measurements, calculate
statistics, and generate graphs.

## Completed matrix

- 10 configured Talos/Kubernetes images
- Normal: 10 runs per image, 100 total
- Temporary interruption: 10 runs per image, 100 total
- Successful transfers: 200/200
- Source/destination digest matches: 200/200
- Detailed final logs: 200, one for every run

The interruption blocks only Earth-to-Space TCPCLv3 port 4556 for 10 seconds.
It does not kill the sender, receiver, or either µD3TN node. Because the trigger
starts from detection of source-image pulling, the outage can overlap download
or OCI preparation and is not guaranteed to occur during active TCPCL payload
transmission, especially for large images.

## Folder guide

```text
experiments/
├── README.md                  Reproduction commands
├── RESULTS.md                 Result and evidence guide
├── config/images.yaml         Exact 10-image list
├── scripts/                   Run, interruption, analysis, graph scripts
├── network-tools/             iptables helper container
├── logs/                      100 normal + 100 interruption logs
├── results/
│   ├── evaluation_results.csv
│   ├── cluster10_evaluation_results.csv
│   ├── evaluation_summary.csv
│   ├── graphs/                Five SVG result graphs
│   └── archive/               Earlier baseline CSVs
└── archive/                   Temporary and exploratory evidence
```

## Primary evidence

- `results/cluster10_evaluation_results.csv`: one row per final run
- `results/evaluation_summary.csv`: grouped performance statistics
- `logs/<image>_normal_runNN.log`: normal-run evidence
- `logs/<image>_interruption_runNN.log`: interruption-run evidence
- `results/graphs/`: normal, interruption, comparison, and trend graphs

## Limitations

- Local Docker environment, not a physical Earth-to-space link
- Local Registry V2 endpoint representing Space-side Harbor
- Latency is post-reconnection Docker ping, not satellite/outage latency
- Throughput is payload divided by end-to-end duration, not raw TCPCL capacity
- Sender CPU uses GNU `time`; receiver CPU/RSS is sampled every 200 ms
- Process memory excludes µD3TN, registry, Docker, kernel and filesystem cache
- Repeated runs may benefit from registry and operating-system caches
