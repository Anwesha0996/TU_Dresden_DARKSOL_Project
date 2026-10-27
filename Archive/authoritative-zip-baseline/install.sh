#!/usr/bin/env sh

PLUGIN_NAME="helm-dtn"
PLUGIN_SOURCE="https://github.com/darksol-cloud/helm-dtn"
PLUGIN_VERSION=$(grep "^version:" "${HELM_PLUGIN_DIR}/plugin.yaml" | sed -e 's/version:[[:space:]]*//' -e 's/"//g' -e "s/'//g")
OS=""
ARCH=""
DOWNLOAD_URL=""
PLUGIN_EXTRACT_PATH="$(mktemp -d -t ".${PLUGIN_NAME}-install_XXXXXXX")"

setup_and_checks() {
    echo "Performing initial checks..."
    echo "  Checking OS and architecture..."
    OS=$(uname -s)
    ARCH=$(uname -m)
    case "$ARCH" in
        x86_64)            ARCH="amd64" ;;
        aarch64|arm64)     ARCH="arm64" ;;
        i386|i686)         ARCH="386"   ;;
    esac
    echo "\tOS: $OS"
    echo "\tARCH: $ARCH"
    echo "  Constructing dowload URL..."
    DOWNLOAD_URL="${PLUGIN_SOURCE}/releases/download/v${PLUGIN_VERSION}/${PLUGIN_NAME}_${PLUGIN_VERSION}_${OS}_${ARCH}.tar.gz"
    echo "  Downloading from $DOWNLOAD_URL"

    echo "  Checking availability of curl..."
    if ! command -v curl >/dev/null 2>&1
    then
        echo -e '\e[1A\e[K  Checking availability of curl  🗴'
        echo "\tcurl is required for getting the plugin binary!"
        exit 1
    fi
    echo -e '\e[1A\e[G\e[K  Checking availability of curl  ✓'
    echo "Checks completed..."


}

download_binary(){
    echo "Downloading and extracting plugin archive..."
    if curl -fsSL "${DOWNLOAD_URL}" | tar xz -C "${PLUGIN_EXTRACT_PATH}"; then
    echo "Successfully downloaded and extracted the plugin archive!"
  else
    echo "Failed to download or extract the plugin archive!"
    exit 1
  fi
}

install_binary(){
    PLUGIN_BIN_EXTRACT_PATH="$PLUGIN_EXTRACT_PATH/helm-dtn"
    mkdir -p "$HELM_PLUGIN_DIR/bin"
    cp "$PLUGIN_BIN_EXTRACT_PATH" "$HELM_PLUGIN_DIR/bin"
}

trap "rm -rf $PLUGIN_EXTRACT_PATH" EXIT
setup_and_checks
download_binary
install_binary

echo "Successfully installed helm-dtn!"
echo "Please refer to https://github.com/darksol-cloud/helm-dtn for further information on how to set up the remote cluster and use the plugin."
