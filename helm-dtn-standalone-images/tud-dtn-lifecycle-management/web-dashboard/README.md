# DTN experiment results dashboard

This dependency-free PHP dashboard reads the existing experiment CSV without
modifying the transfer application.

Run locally:

```bash
RESULTS_CSV=../experiments/store-forward/results/persistent_store_forward_results.csv \
php -S 127.0.0.1:8080 -t public
```

Run with Docker:

```bash
docker build -t dtn-results-dashboard web-dashboard
docker run --rm -p 8080:80 \
  -v "$PWD/experiments/store-forward/results/persistent_store_forward_results.csv:/data/persistent_store_forward_results.csv:ro" \
  dtn-results-dashboard
```

Open <http://127.0.0.1:8080>. The dashboard is read-only and escapes all CSV
values before rendering them.
