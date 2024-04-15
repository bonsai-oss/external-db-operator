#!/bin/bash

if [ -z "${CI}" ]; then
  echo "This script is only intended to be run in CI"
  exit 1
fi

# Setup K3s
curl -sfL https://get.k3s.io | sh -
curl -sSL https://get.docker.com | sh

CONTAINER_IMAGE="${CI_REGISTRY_IMAGE}:$(git rev-parse --short HEAD)"
docker build -t "${CONTAINER_IMAGE}" . || exit 1

# Wait for K3s to be ready with kubectl; retry up to 60 seconds
while :; do
  kubectl wait --for=condition=Ready node --all --timeout=60s && break
  sleep 10
done

# Export the image to the local k3s installation
docker save "${CONTAINER_IMAGE}" | k3s ctr images import -
sed -i "s|image: .*|image: ${CONTAINER_IMAGE}|g" ./manifests/deployment.yaml

echo "export OPERATOR_IMAGE=${CONTAINER_IMAGE}" >> .env