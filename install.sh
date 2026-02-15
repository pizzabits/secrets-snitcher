#!/bin/bash
# secrets-snitcher installer
# Usage: curl -sL https://raw.githubusercontent.com/pizzabits/secrets-snitcher/main/install.sh | bash
set -e

echo ""
echo "  secrets-snitcher installer"
echo "  eBPF-powered Kubernetes secret access monitor"
echo ""

# Create namespace + RBAC
echo "[1/3] Creating namespace and RBAC..."
kubectl apply -f https://raw.githubusercontent.com/pizzabits/secrets-snitcher/main/k8s/rbac.yaml

# Deploy pod (ConfigMap + Pod + Service — no Docker build needed)
echo "[2/3] Deploying probe..."
kubectl apply -f https://raw.githubusercontent.com/pizzabits/secrets-snitcher/main/k8s/pod-inline.yaml

# Wait for ready
echo "[3/3] Waiting for pod to be ready (~30s for BCC install)..."
kubectl -n secrets-snitcher wait --for=condition=Ready pod/secrets-snitcher --timeout=120s

echo ""
echo "Done! secrets-snitcher is running."
echo ""
echo "Next steps:"
echo "  kubectl -n secrets-snitcher port-forward svc/secrets-snitcher 9100:9100 &"
echo "  curl http://localhost:9100/api/v1/secret-access | jq"
echo ""
