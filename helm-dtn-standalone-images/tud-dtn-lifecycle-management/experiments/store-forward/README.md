# One-hour store-and-forward validation

This is separate from the baseline 90-run matrix. It tests whether the original
ADU, submitted once, is retained at Earth µD3TN until a future contact becomes
available after a one-hour TCPCL path outage.

The test does not resend the ADU after reconnection. A successful destination
digest match therefore demonstrates delivery of the originally queued bundle.

Run only when no other experiment owns `earth-node` and `space-node`:

```bash
experiments/scripts/run_store_forward_1h.sh flannel leo 1
```

Results are written to `experiments/store-forward/results/`, with one detailed
log under `experiments/store-forward/logs/` and raw work files under
`experiments/store-forward/work/`.

This µD3TN v0.15.0 harness uses in-process bundle retention; it does not yet
prove persistence across an Earth-node restart. Do not restart the node during
the outage. Persistent disk-backed custody/storage would require a separate
µD3TN storage configuration and must not be claimed by this test.
