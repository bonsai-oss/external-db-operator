#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail
set -o xtrace

# This script is used to run integration tests locally using minikube.

command -v minikube >/dev/null 2>&1 || { echo >&2 "minikube is required but not installed.  Aborting."; exit 1; }
command -v docker >/dev/null 2>&1 || { echo >&2 "docker is required but not installed.  Aborting."; exit 1; }
command -v kubectl >/dev/null 2>&1 || { echo >&2 "kubectl is required but not installed.  Aborting."; exit 1; }

kubectl get nodes | grep minikube || { echo >&2 "minikube is not running / used.  Aborting."; exit 1; }

eval $(minikube docker-env)

export OPERATOR_IMAGE="external-dns-operator:$(git rev-parse --short HEAD)"
docker buildx build -t "${OPERATOR_IMAGE}" --squash .

# clean go test cache
go clean -testcache
go test --tags integration -v ./integration-tests/...
