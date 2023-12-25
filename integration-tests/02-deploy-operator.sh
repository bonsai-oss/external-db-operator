#!/bin/bash -x

kubectl apply -f manifests/rbac.yaml

sed -i "s|image: .*|image: ${CI_REGISTRY_IMAGE}:latest|" manifests/deployment.yaml
kubectl apply -f manifests/deployment.yaml

# Wait for the operator to be ready
kubectl wait --for=condition=Available deployment/external-db-operator --timeout=20s

kubectl get pods
kubectl logs -l app=external-db-operator

kubectl wait --for=condition=Available deployment/external-db-operator --timeout=20s || exit 1
