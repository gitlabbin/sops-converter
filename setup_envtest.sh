#!/bin/bash

ENVTEST_ASSETS_DIR=$(pwd)/test/assets

API_FILE=$(pwd)/test/assets/bin/kube-apiserver
KUBECTL_FILE=$(pwd)/test/assets/bin/kubectl
ETCD_FILE=$(pwd)/test/assets/bin/etcd

fetch_envtest_assets() {
  if [[ ! -f "$API_FILE" && ! -f "$KUBECTL_FILE" && ! -f "$ETCD_FILE" ]]; then
    echo "kube builder tool files not exist."
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m | sed 's/x86_64/amd64/')
    mkdir -p $ENVTEST_ASSETS_DIR
    curl -fsL "https://storage.googleapis.com/kubebuilder-tools/kubebuilder-tools-1.16.4-${OS}-${ARCH}.tar.gz" | tar zx --strip-components=1 -C $ENVTEST_ASSETS_DIR
  fi
}

setup_envtest_env() {
  export TEST_ASSET_KUBE_APISERVER=$ENVTEST_ASSETS_DIR/bin/kube-apiserver
  export TEST_ASSET_ETCD=$ENVTEST_ASSETS_DIR/bin/etcd
  export TEST_ASSET_KUBECTL=$ENVTEST_ASSETS_DIR/bin/kubectl
}
