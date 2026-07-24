# Agent Instructions — rollouts-plugin-metric-ai

Go-based Argo Rollouts metric provider plugin. It collects stable and canary pod logs then delegates all AI analysis to the `kubernetes-agent` via a single `POST /a2a/analyze` call. There is no LLM inside the plugin. This is an experimental project — simplicity over backwards compatibility.

Do not create summary or migration documents. Update code and documentation in place.

## Stack

Go 1.24+, Argo Rollouts v1.9.0 metric provider interface, HashiCorp go-plugin (RPC), `k8s.io/client-go`.

## Project Layout

```
main.go                          # Plugin bootstrap — RPC handshake, serves RpcMetricProviderPlugin
internal/plugin/
  plugin.go                      # RpcPlugin — implements Run(), Resume(), Terminate(), GarbageCollect()
  a2a.go                         # A2AClient — POST /a2a/analyze, HealthCheck()
  ai_mode.go                     # analyzeWithKubernetesAgent() — wires plugin.go to a2a.go
  ai.go                          # AIAnalysisResult, ModelAnalysisResult, AIAnalysisParams
  utils.go                       # isTruthy(), truncate()
config/argo-rollouts/            # Kustomize overlay for deploying Argo Rollouts with plugin
                                 # Note: the plugin itself carries no credentials — all AI keys
                                 # live in the kubernetes-agent pod, not in the controller
config/rollouts-examples/        # Example Rollout and AnalysisTemplate resources
examples/                        # Minimal snippets for plugin registration
```

## Building and Running

```bash
make build              # Compile to bin/manager
make test               # Unit tests with coverage
make lint               # golangci-lint
make fmt                # gofmt
make docker-build       # Build image locally
make docker-buildx      # Build and push multi-platform (linux/amd64,linux/arm64)
```

## Local Development Cycle (Kind)

```bash
make docker-build
kind load docker-image csanchez/rollouts-plugin-metric-ai:latest --name rollouts-plugin-metric-ai-test-e2e
kubectl rollout restart deployment/argo-rollouts -n argo-rollouts
kubectl rollout status deployment/argo-rollouts -n argo-rollouts
kubectl logs -f deployment/argo-rollouts -n argo-rollouts
```

## Cluster Deployment (Production / OpenShift)

The plugin is deployed as part of the Argo Rollouts controller image via GitOps (`progressive-delivery/` repo). Do not patch cluster resources directly — update the manifests and push.

```bash
# After pushing a new image, update config/argo-rollouts/kustomization.yaml with the new tag,
# commit, and push. Argo CD will sync.

# Verify the plugin binary is present in the controller
kubectl exec -n argo-rollouts deployment/argo-rollouts -- ls -la /home/argo-rollouts/rollouts-plugin-metric-ai
```

## Plugin Interface

`Run()` in [`internal/plugin/plugin.go`](internal/plugin/plugin.go) is the main entry point called by Argo Rollouts during an analysis step. It:

1. Parses the `AnalysisRun` metric config (`agentUrl`, `stableLabel`, `canaryLabel`, `extraPrompt`, `githubUrl`, `baseBranch`).
2. Fetches pod logs for stable and canary pods using the in-cluster Kubernetes client.
3. Calls `analyzeWithKubernetesAgent()`, which POSTs to the agent's `/a2a/analyze` endpoint.
4. Returns a `Measurement` with `Phase: Successful` (promote) or `Phase: Failed` (rollback).

## AnalysisTemplate Configuration

```yaml
apiVersion: argoproj.io/v1alpha1
kind: AnalysisTemplate
metadata:
  name: ai-analysis-agent
spec:
  metrics:
    - name: ai-check
      provider:
        plugin:
          argoproj-labs/metric-ai:
            agentUrl: http://kubernetes-agent.openshift-gitops.svc.cluster.local:8080
            stableLabel: role=stable
            canaryLabel: role=canary
            githubUrl: https://github.com/org/repo   # Optional — enables async PR/issue creation
            baseBranch: main
            extraPrompt: "Focus on error rate and latency."
```

## Debugging

```bash
# Follow controller logs (plugin output is interleaved here)
kubectl logs -f deployment/argo-rollouts -n argo-rollouts

# Filter for plugin activity
kubectl logs -n argo-rollouts deployment/argo-rollouts | grep -E "metric-ai|plugin|analysis"

# Check analysis run details
kubectl get analysisrun -n quarkus-demo
kubectl describe analysisrun <name> -n quarkus-demo
```

The plugin carries no credentials. All AI keys (`OPENAI_API_KEY`, `GOOGLE_API_KEY`, `github_token`, etc.) live exclusively in the `kubernetes-agent` pod — either injected from the `kubernetes-agent` Kubernetes Secret or read from Vault at startup. The `githubUrl` field in the `AnalysisTemplate` is a plain repository URL forwarded to the agent; no token is needed in the controller.

## Common Issues

| Symptom | Check |
|---|---|
| Plugin binary not found | Verify image tag in `config/argo-rollouts/kustomization.yaml` and that the controller restarted |
| Analysis always fails | `kubectl logs` on controller; verify `agentUrl` resolves and agent is healthy |
| 429 errors | Rate limit on the agent/LLM side; check agent logs |
| Architecture mismatch on Kind | Rebuild with `make docker-build PLATFORMS=linux/arm64` or `linux/amd64` to match cluster |

## Documentation Standards

- Update `README.md` and `ARCHITECTURE.md` when behaviour changes.
- Professional tone, plain English, no AI-sounding phrasing.
- No "Made with Bob" comments.

## Resources

- [Argo Rollouts Plugin Docs](https://argo-rollouts.readthedocs.io/en/stable/features/plugins/)
- [HashiCorp go-plugin](https://github.com/hashicorp/go-plugin)
- [Kind](https://kind.sigs.k8s.io/)
