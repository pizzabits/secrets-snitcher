#!/bin/bash
# Demo recording script for secrets-snitcher
#
# Prerequisites:
#   - kubectl configured to your cluster
#   - python3 available
#   - jq installed
#
# Usage:
#   1. Start the mock API in another terminal:  python3 demo/mock-api.py
#   2. Start recording (screen capture or asciinema)
#   3. Run this script:  bash demo/record.sh
#
# The script pauses between steps so you can see each command clearly.

set -e

DELAY=2

type_and_run() {
    echo ""
    echo -e "\033[1;32m\$\033[0m $1"
    sleep 1
    eval "$1"
    sleep "$DELAY"
}

echo "========================================="
echo "  secrets-snitcher demo"
echo "========================================="
echo ""
sleep 1

# Step 1: Show real cluster
type_and_run "kubectl get nodes"

# Step 2: Deploy secrets-snitcher
type_and_run "kubectl apply -f k8s/"

# Step 3: Wait for pods
sleep 2
type_and_run "kubectl get pods -n secrets-snitcher"

# Step 4: Check baseline — nothing suspicious
type_and_run "curl -s localhost:9100/api/v1/secret-access | jq"

# Step 5: Deploy the malicious pod
echo ""
echo -e "\033[1;33m--- Now let's deploy something suspicious... ---\033[0m"
sleep 2
type_and_run "kubectl apply -f demo/malicious-pod.yaml"

# Step 6: Toggle mock to return caught data
curl -s localhost:9100/toggle > /dev/null 2>&1

# Step 7: Wait and check again
echo ""
echo -e "\033[1;33m--- Waiting 5 seconds... ---\033[0m"
sleep 5
type_and_run "curl -s localhost:9100/api/v1/secret-access | jq"

# Step 8: Punchline — what is it actually doing?
echo ""
echo -e "\033[1;31m--- 4,872 reads/sec on the service account token! Let's check what this pod is doing... ---\033[0m"
sleep 2
type_and_run "kubectl logs totally-legit-app --tail=3 2>/dev/null || echo '[worker] processing batch job...
[worker] processing batch job...
[worker] processing batch job...'"

echo ""
echo -e "\033[1;31m--- Looks innocent? Let's check the actual command... ---\033[0m"
sleep 2
echo ""
echo -e "\033[1;32m\$\033[0m kubectl exec totally-legit-app -- cat /proc/1/cmdline | tr '\\\\0' ' '"
echo "/bin/sh -c while true; do cat /var/run/secrets/kubernetes.io/serviceaccount/token > /dev/null; done"
sleep 3

echo ""
echo "========================================="
echo "  Caught."
echo "========================================="
echo ""
