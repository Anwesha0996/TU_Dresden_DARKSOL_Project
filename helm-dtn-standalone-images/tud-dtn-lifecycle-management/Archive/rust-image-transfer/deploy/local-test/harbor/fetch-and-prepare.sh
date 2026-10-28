#!/usr/bin/env bash
# Fetches the pinned Harbor online installer and runs its `prepare` step,
# which generates docker-compose.yml + common/config/* for this harbor.yml.
#
# This does NOT start Harbor - it only stages the installer files (prepare,
# install.sh, common.sh, LICENSE) and generates the Compose file + configs
# that ../docker-compose.yml then `include`s. Startup happens via
# `docker compose up` in the parent directory (see ../README.md).
#
# Safe to re-run: re-fetches and re-generates deterministically from the
# pinned version and the checked-in harbor.yml.
set -euo pipefail

HARBOR_VERSION="v2.15.2"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

if [[ ! -x ./prepare ]]; then
    echo "==> Fetching Harbor online installer ${HARBOR_VERSION}..."
    curl -sSL -o /tmp/harbor-installer.tgz \
        "https://github.com/goharbor/harbor/releases/download/${HARBOR_VERSION}/harbor-online-installer-${HARBOR_VERSION}.tgz"
    tar -xzf /tmp/harbor-installer.tgz -C /tmp
    cp /tmp/harbor/prepare ./prepare
    cp /tmp/harbor/common.sh ./common.sh
    cp /tmp/harbor/install.sh ./install.sh
    cp /tmp/harbor/LICENSE ./LICENSE
    chmod +x ./prepare ./install.sh
    rm -rf /tmp/harbor /tmp/harbor-installer.tgz
    echo "${HARBOR_VERSION}" > .installer-version
else
    echo "==> Installer already staged ($(cat .installer-version 2>/dev/null || echo unknown version)), skipping fetch."
fi

mkdir -p data log

echo "==> Running ./prepare against harbor.yml (generates docker-compose.yml + common/config)..."
./prepare

echo "==> Done. Generated: $SCRIPT_DIR/docker-compose.yml"
