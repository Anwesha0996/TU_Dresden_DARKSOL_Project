# Demo walkthrough: Rust-native AAP2 image channel

Step-by-step instructions to run the full corrected architecture
(TASKS-PART-2.md) end-to-end on your own machine and show it to your
supervisor: a container image and a Helm chart both cross a simulated
DTN link through **two independent AAP2 agents** (one Go, one Rust), land
on a real Kubernetes cluster, and a pod comes up pulling from Harbor —
including a run where the image transfer survives a deliberately injected
network outage.

Total time: roughly 20-30 minutes for the one-time setup, then a few
minutes per demo run after that.

## What you're demonstrating

```text
helm dtn install <chart>          (single entrypoint, unchanged for the user)
   |
   +-- detected image -> aap2-image-sender -> AAP2 -> µD3TN -> AAP2 -> aap2-image-receiver -> Harbor
   |
   `-- chart only (no image bytes) -> existing Go AAP2 agent -> µD3TN -> existing Go receiver -> `helm install`
                                                                                  |
                                                                                  v
                                                                     pod pulls image from Harbor
```

Two independent AAP2 agents share the same pair of µD3TN nodes. The image
never travels inside the chart bundle. Nothing blocks chart install on the
image having arrived — Kubernetes' own `ImagePullBackOff` retry covers the
gap, which you can show live in Part 6.

---

**On Linux (e.g. SUSE 15.6) instead of macOS?** Only Part 0 below is
macOS/Homebrew-specific. Use [DEMO-linux.md](DEMO-linux.md) for Part 0, then
come back here and continue at Part 1 — everything from there on is
identical.

## 0. Prerequisites (one-time install)

```sh
brew install --cask docker-desktop     # if not already installed; then open it once
brew install go rustup-init protobuf kind kubectl helm skopeo
rustup-init -y                          # if you installed rustup fresh
```

Versions this was built and verified against (yours don't need to match
exactly, but if something's off, check here first):

| Tool | Version used |
| --- | --- |
| Docker | 29.6.2 / Compose v5.3.1 |
| Go | 1.26.5 |
| Rust/Cargo | 1.97.1 |
| kind | 0.32.0 |
| kubectl | 1.36.1 |
| helm | 4.2.3 (Helm 3.x line) |
| skopeo | 1.24.0 |

Make sure Docker Desktop is actually running before continuing (`docker
info` should succeed).

---

## 1. Start the local DTN + Harbor harness

This brings up two µD3TN nodes (`earth-node`, `space-node`) and a real
Harbor registry, all in Docker, simulating the earth/space link.

```sh
cd deploy/local-test

# First time only, if harbor/docker-compose.yml doesn't exist yet:
./harbor/fetch-and-prepare.sh

docker compose up -d --build
```

Wait about a minute, then check everything is healthy:

```sh
docker compose ps
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8091/api/v2.0/systeminfo   # expect 200
```

The port above (`8091`) is whatever `http.port` is set to in
`harbor/harbor.yml` — this repo currently has it set to `8091` because the
originally-documented `8081` (and `8082`) were already bound by unrelated
containers on the machine this was last verified on. If you get a `404`/
connection-refused here, check that port against your own `harbor.yml`
rather than assuming the stack didn't start — `docker compose ps` (above)
is the actual health check; the curl just confirms the port. If you change
the port, re-run `./harbor/fetch-and-prepare.sh` to regenerate the compose
file from it.

Configure the earth->space contact (µD3TN contacts are runtime-only, not
persisted — re-run this any time `earth-node` restarts):

```sh
./scripts/configure-contact.sh
```

---

## 2. Build everything

From the repo root:

```sh
cd ../..   # back to repo root if you're still in deploy/local-test

# Go CLI (the `helm dtn` binary)
go build -o helm-dtn .

# Rust workspace (aap2-image-sender / aap2-image-receiver), native binaries
cd rust
cargo build --workspace
cd ..

# Container image with both Rust binaries + skopeo, for running the
# space-side receiver in a container
docker build -t helm-dtn-rust-image-transfer:demo rust/

# Container image for the Go chart receiver (the "ALME")
docker build -f deploy/demo-k8s/Dockerfile.receiver -t helm-dtn-receiver:demo .
```

---

## 3. Stand up a demo Kubernetes cluster

A `kind` (Kubernetes-in-Docker) cluster stands in for the real remote
cluster. It's configured to trust Harbor as a plain-HTTP registry reachable
by its container-network name (`nginx:8080`), and joined to Harbor's Docker
network so pods can actually pull from it.

```sh
kind create cluster --config deploy/demo-k8s/kind-config.yaml
docker network connect local-test_harbor helm-dtn-phase-f-control-plane
kubectl wait --for=condition=Ready node/helm-dtn-phase-f-control-plane --timeout=90s
```

---

## 4. Start the two space-side receivers

**Rust image receiver** — registers its own AAP2 agent, pushes received
images into Harbor:

```sh
docker run -d --name aap2-image-receiver \
  --network dtn-net \
  -e HARBOR_CREDS=admin:Harbor12345 \
  helm-dtn-rust-image-transfer:demo \
  aap2-image-receiver --tcp space-node:4242 --registry nginx:8080/library \
  --insecure-registry --agent-id image-transfer

docker network connect local-test_harbor aap2-image-receiver

docker logs aap2-image-receiver   # expect "registered with µD3TN, waiting for transfers"
```

**Go chart receiver** — registers `install`/`upgrade`/`uninstall` agents,
runs `helm install`/`upgrade` against the kind cluster when a chart arrives:

```sh
kind get kubeconfig --internal --name helm-dtn-phase-f > /tmp/kubeconfig-internal.yaml

docker run -d --name helm-dtn-receiver \
  --network dtn-net \
  -e DTN_SERVICE_ADRESS=space-node \
  -e DTN_SERVICE_PORT=4242 \
  -e KUBECONFIG=/kubeconfig/config \
  -v /tmp/kubeconfig-internal.yaml:/kubeconfig/config:ro \
  helm-dtn-receiver:demo

docker network connect kind helm-dtn-receiver

docker logs helm-dtn-receiver   # expect three "config:{is_subscriber:true ...}" lines
```

At this point you have: Harbor + 2 µD3TN nodes + a Kubernetes cluster + 2
independent receivers, all running and waiting — this is the "remote/space
cluster" side, fully set up.

---

## 5. Run the demo: `helm dtn install`

This is the actual command your supervisor should see. It's the **same
single entrypoint** as before this work — nothing about the user-facing
command changed:

```sh
PATH="$(pwd)/rust/target/debug:$PATH" ./helm-dtn install examples/demo-chart/ \
  --destination-eid "dtn://space.dtn/" \
  --registry "nginx:8080/library" \
  --tcp 127.0.0.1:14242
```

Narrate what's happening in the log output as it scrolls:
1. `helmdd` detects the one image reference in the chart (`nginx:alpine`).
2. `aap2-image-sender` pulls it with `skopeo`, computes a checksum, and
   sends it over its **own** AAP2 connection (separate from the chart).
3. `helmdp` strips any image data from the chart bundle (there was none to
   begin with here — images never enter the chart archive) and sends the
   **chart only** over the existing/unmodified Go AAP2 path.

---

## 6. Verify the result

```sh
kubectl get pods -o wide
```

You should see a `demo-chart-release-...` pod `Running` within a few
seconds. If you catch it right after the install command, you may
legitimately see one `ImagePullBackOff` first — that's expected and is
itself worth showing your supervisor:

```sh
kubectl get events --sort-by='.lastTimestamp' | tail -10
```

This shows the chart and image arriving independently (possibly in either
order) and Kubernetes' own retry recovering automatically — no custom
coordination code was written for this, by design.

Independently confirm the image really is in Harbor (not just assumed):

```sh
skopeo inspect --creds admin:Harbor12345 --tls-verify=false \
  docker://127.0.0.1:8091/library/nginx:alpine
```

The `Digest` here should match the digest in `aap2-image-receiver`'s log:

```sh
docker logs aap2-image-receiver | grep "transfer complete"
```

---

## 7. Bonus (recommended): show it surviving a network outage

This is the most convincing part of the demo — it proves the "disruption
tolerant" part of DTN, not just the happy path.

```sh
# Clean up the previous release first so the reinstall isn't blocked by
# stale Helm state:
kubectl delete deployment,service demo-chart-release --ignore-not-found
kubectl delete secret -l owner=helm --ignore-not-found

# Cut the link between earth-node and space-node completely:
(cd deploy/local-test && ./scripts/netem.sh outage space-node)

# Kick off the install while the link is down:
PATH="$(pwd)/rust/target/debug:$PATH" ./helm-dtn install examples/demo-chart/ \
  --destination-eid "dtn://space.dtn/" \
  --registry "nginx:8080/library" \
  --tcp 127.0.0.1:14242
```

The command still completes and prints success — because a send only needs
to hand the bundle to your **local** µD3TN node, which queues it for
delivery (store-and-forward). Point this out: `helm dtn install` returning
doesn't mean delivery happened yet.

```sh
# Confirm nothing has arrived yet:
docker logs aap2-image-receiver --tail 3
docker logs helm-dtn-receiver --tail 3
kubectl get pods   # should show no new pod
```

Wait ~15 seconds with the link still down, then restore it:

```sh
(cd deploy/local-test && ./scripts/netem.sh clear space-node)
```

Within a few seconds, watch it recover with no manual retry:

```sh
docker logs aap2-image-receiver --tail 5
docker logs helm-dtn-receiver --tail 6
kubectl get pods -o wide
```

The pod should come up `Running`, and the image digest in Harbor should
match — the transfer that was queued during the outage was delivered
automatically the moment the link came back.

---

## Cleanup

```sh
docker rm -f aap2-image-receiver helm-dtn-receiver
kind delete cluster --name helm-dtn-phase-f
cd deploy/local-test && docker compose down       # add -v to also wipe Harbor's data
```

---

## If something goes wrong

- **`aap2-image-receiver`/`helm-dtn-receiver` won't register / "Unauthorized"**:
  a µD3TN AAP2 agent ID from a previous, uncleanly-killed run may still be
  registered. `docker compose restart earth-node space-node` in
  `deploy/local-test` and re-run `./scripts/configure-contact.sh`.
- **Image pull fails inside the pod**: confirm the kind node is actually on
  the Harbor network: `docker network connect local-test_harbor
  helm-dtn-phase-f-control-plane` (safe to re-run, no-op if already
  connected).
- **`helm dtn install` hangs**: check `docker compose ps` in
  `deploy/local-test` — all of `earth-node`, `space-node`, and the Harbor
  services should be `Up`/`healthy`.
- **`docker compose up` fails with "port is already allocated"**, or the
  Part 1 health-check curl 404s/connection-refuses: another container on
  this machine already owns that host port. Change `http.port` in
  `harbor/harbor.yml` to a free port, re-run `./harbor/fetch-and-prepare.sh`
  to regenerate the compose file, then use that port in every command below
  that references it (the curl check, and the `skopeo inspect` in Part 6).
- **Harder to spot: `docker compose up` fails with "permission denied"**
  reading a file under `harbor/common/config/.../env`: Harbor's `prepare`
  step runs inside a container as root, so on a rootful-Docker Linux host
  the generated config files end up owned by `root` with mode `640` —
  unreadable by your own user. Fix with
  `docker run --rm -v "$(pwd)/harbor/common:/common" alpine chmod -R a+rX /common`
  (from `deploy/local-test`) after every `fetch-and-prepare.sh` run. Not
  observed on macOS/Docker Desktop.
- **Full reference**: `deploy/demo-k8s/README.md` has the exact commands
  from the original verification run, plus two pre-existing (unrelated)
  bugs found while building this — worth mentioning if asked "did you find
  anything else along the way?"
