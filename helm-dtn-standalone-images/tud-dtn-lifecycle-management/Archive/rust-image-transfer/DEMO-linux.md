# Demo walkthrough: SUSE / Linux prerequisites

Companion to [DEMO.md](DEMO.md), for running the same demo on Linux (written
against openSUSE Leap / SLES 15.6, `zypper`-based). **Only Part 0
(Prerequisites) differs from DEMO.md** — every command from Part 1 onward
(`docker compose`, `docker build`, `kind`, `kubectl`, `skopeo`,
`./helm-dtn install ...`, `netem.sh`) is plain Docker/Linux CLI and works
identically once the tools below are on `PATH`. Use this file for Part 0,
then continue directly with [DEMO.md](DEMO.md) starting at Part 1.

**Caveat:** unlike DEMO.md — which was executed command-by-command on macOS
this session to confirm it works exactly as written — this Part 0 has not
been run end-to-end on an actual SUSE box. The package names and install
methods below are accurate as of writing, but if a `zypper` package has
moved or been renamed, that's the first thing to check.

## 0. Prerequisites (one-time install, SUSE/Linux)

### Docker

SLES's Docker packages live in the **Containers Module**, a free add-on you
register once:

```sh
sudo SUSEConnect -p sle-module-containers/15.6/x86_64   # SLES; skip on openSUSE Leap, it's in the default repos
sudo zypper install docker docker-compose
sudo systemctl enable --now docker
sudo usermod -aG docker "$USER"                          # log out/in (or `newgrp docker`) to pick this up
```

Verify with `docker info` and `docker compose version` before continuing.

**Gotcha:** SUSE's firewall (`firewalld`, or the older `SuSEfirewall2`) can
block Docker's bridge networking by default. If containers can't reach each
other later in the demo (e.g. `aap2-image-receiver` can't resolve
`space-node`), check the firewall first — it's not a bug in the demo:

```sh
sudo firewall-cmd --state                                # is it even running?
sudo firewall-cmd --zone=trusted --add-interface=docker0 --permanent
sudo firewall-cmd --reload
```

### Go, Rust, protobuf, skopeo

```sh
sudo zypper install go1.26 protobuf-devel skopeo         # adjust the go package name/version to what's current -
                                                           # `zypper search go` if `go1.26` doesn't exist yet
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y
source "$HOME/.cargo/env"
```

skopeo is commonly packaged directly on SUSE (SUSE co-maintains a lot of the
containers/OCI tooling) — `zypper install skopeo` should just work. If
`protobuf-devel` doesn't provide a working `protoc`, check
`zypper search protoc` or fetch a release binary directly from
[protocolbuffers/protobuf](https://github.com/protocolbuffers/protobuf/releases).

### kind, kubectl, helm

None of these are reliably in SUSE's default repos — install the official
prebuilt Linux binaries directly (the same method Kubernetes/kind/Helm
recommend for every distro, zypper or not):

```sh
# kind
curl -Lo /tmp/kind https://kind.sigs.k8s.io/dl/v0.32.0/kind-linux-amd64
sudo install -m 0755 /tmp/kind /usr/local/bin/kind

# kubectl
curl -Lo /tmp/kubectl "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
sudo install -m 0755 /tmp/kubectl /usr/local/bin/kubectl

# helm
curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
```

(Use `kind-linux-arm64` / `bin/linux/arm64/kubectl` instead if you're on an
ARM SLES box.)

### Verify everything

```sh
docker --version && docker compose version
go version
cargo --version
kind version
kubectl version --client
helm version
skopeo --version
```

All seven should print a version with no errors before moving on.

---

Continue with [DEMO.md](DEMO.md) **Part 1: Start the local DTN + Harbor
harness** — everything from there is identical on Linux. Two small notes
that apply once you get there, both harmless, not bugs:
- `aap2-image-sender`'s `--override-os linux --override-arch <arch>` flag
  becomes redundant on a Linux host (skopeo already defaults to `linux`)
  but doesn't need to be removed — leave DEMO.md's commands exactly as
  written.
- `target_arch` defaults to `amd64`, matching a typical SLES x86_64 host, so
  no flag changes are needed there either.
