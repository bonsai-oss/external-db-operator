#!/bin/bash

# Setup K3s
curl -sfL https://get.k3s.io | sh -
curl -sSL https://get.docker.com | sh

docker build -t "${CI_REGISTRY_IMAGE}:latest" .

# Wait for K3s to be ready with kubectl; retry up to 60 seconds
while :; do
  kubectl wait --for=condition=Ready node --all --timeout=60s && break
  sleep 10
done

# Export the image to the local k3s installation
docker save "${CI_REGISTRY_IMAGE}:latest" | k3s ctr images import -

# Start databases
kubectl apply -f manifests/databases/*.yaml
