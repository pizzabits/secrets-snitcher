.PHONY: deploy undeploy demo demo-clean logs test

NAMESPACE = secrets-snitcher

deploy:
	kubectl apply -f k8s/rbac.yaml
	kubectl apply -f k8s/pod-inline.yaml
	@echo "Deployed. Waiting for pod to be ready (~30s for BCC install)..."
	kubectl -n $(NAMESPACE) wait --for=condition=Ready pod/secrets-snitcher --timeout=120s

undeploy:
	kubectl delete -f k8s/pod-inline.yaml --ignore-not-found
	kubectl delete -f k8s/rbac.yaml --ignore-not-found

demo:
	kubectl apply -f demo/sample-secrets.yaml
	kubectl apply -f demo/malicious-pod.yaml
	@echo ""
	@echo "Malicious pod deployed. Wait a few seconds, then check:"
	@echo "  kubectl -n $(NAMESPACE) port-forward svc/secrets-snitcher 9100:9100"
	@echo "  curl http://localhost:9100/api/v1/secret-access | jq"

demo-clean:
	kubectl delete -f demo/malicious-pod.yaml --ignore-not-found
	kubectl delete -f demo/sample-secrets.yaml --ignore-not-found

logs:
	kubectl -n $(NAMESPACE) logs secrets-snitcher --tail=50 -f

test:
	pytest tests/ -v
