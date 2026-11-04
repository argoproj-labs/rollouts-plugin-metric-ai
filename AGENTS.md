# Agent Instructions — rollouts-plugin-metric-ai

Go-based Argo Rollouts metric provider plugin. It delegates all AI analysis to an external analysis agent over HTTP — a single `POST /analyze` call — and turns the response into a promote or rollback measurement. The plugin sends only the pod selectors; the agent fetches logs and metrics itself with its own tools. `kubernetes-agent` is the reference implementation of this HTTP contract, and any service implementing the same contract can serve as the analysis agent. There is no LLM inside the plugin. This is an experimental project — simplicity over backwards compatibility.

The reference agent runs a tiered pipeline behind that single call: parallel data agents gather evidence, a cheap LLM (open weights, e.g. Qwen) concludes, a decision model (Jev via TypeSafe) grades whether the conclusion is supported by the evidence, and a clearly disputed conclusion is re-analyzed once with a stronger LLM (open weights, e.g. DeepSeek) before an optional human-approval pause gate decides. On a rollback the agent asynchronously delegates to `remediation-agent`, a sandboxed coding agent that verifies fixes with tests and opens GitHub PRs (code bugs) or issues (operational problems). The full stack deploys via [kdubois/progressive-delivery](https://github.com/kdubois/progressive-delivery).

Do not create summary or migration documents. Update code and documentation in place.

## Stack

Go 1.24+, Argo Rollouts v1.9.0 metric provider interface, HashiCorp go-plugin (RPC).

## Project Layout

```
main.go                          # Plugin bootstrap — RPC handshake, serves RpcMetricProviderPlugin
internal/plugin/
  plugin.go                      # RpcPlugin — implements Run(), Resume(), Terminate(), GarbageCollect()
  agent.go                       # AgentClient — POST /analyze, HealthCheck()
  ai_mode.go                     # analyzeWithKubernetesAgent() — wires plugin.go to agent.go
  ai.go                          # AIAnalysisResult
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

The plugin ships as a binary inside the Argo Rollouts controller image. To deploy a new version:

```bash
# Build and push the image
make docker-buildx

# Update the image tag in config/argo-rollouts/kustomization.yaml, commit, and push

# Restart the controller so it picks up the new binary
kubectl rollout restart deployment/argo-rollouts -n argo-rollouts
kubectl rollout status deployment/argo-rollouts -n argo-rollouts

# Verify the plugin binary is present in the controller
kubectl exec -n argo-rollouts deployment/argo-rollouts -- ls -la /home/argo-rollouts/rollouts-plugin-metric-ai
```

## Plugin Interface

`Run()` in [`internal/plugin/plugin.go`](internal/plugin/plugin.go) is the main entry point called by Argo Rollouts during an analysis step. It:

1. Parses the `AnalysisRun` metric config (`agentUrl`, `stableLabel`, `canaryLabel`, `extraPrompt`, `githubUrl`, `baseBranch`, `pauseOnUncertainty`, `uncertaintyThreshold`).
2. Calls `analyzeWithKubernetesAgent()`, which POSTs to the agent's `/analyze` endpoint. The agent fetches pod logs itself; the plugin passes only the pod selectors.
3. Returns a `Measurement` with `Phase: Successful` (promote) or `Phase: Failed` (rollback). A promote that the agent's grader disputes beyond its margin and that trips the uncertainty gate returns `pausedRollout` in the measurement metadata instead — the rollout holds for human review until `oc argo rollouts promote`.

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

The plugin carries no credentials. All AI keys (analysis, grader, escalation and
remediation keys) live exclusively in the agent services — injected from their
Kubernetes Secrets or synced from Vault by the Vault Secrets Operator. The
`githubUrl` field in the `AnalysisTemplate` is a plain repository URL forwarded
to the agent; no token is needed in the controller.

## Common Issues

| Symptom | Check |
|---|---|
| Plugin binary not found | Verify image tag in `config/argo-rollouts/kustomization.yaml` and that the controller restarted |
| Analysis always fails | `kubectl logs` on controller; verify `agentUrl` resolves and agent is healthy |
| 429 errors | Rate limit on the agent/LLM side; check agent logs |
| Architecture mismatch on Kind | Rebuild with `make docker-build PLATFORMS=linux/arm64` or `linux/amd64` to match cluster |

## Commits

Every commit needs a `Signed-off-by` line — the DCO app rejects PRs without it. Always commit with `git commit -s`.

## Versioning

Tags track the Argo Rollouts version the plugin is built against, which is pinned in `go.mod` — Argo Rollouts v1.9.0 gets plugin releases v1.9.x. Bump the patch for each plugin release within that Argo Rollouts line, and bump the minor when `go.mod` moves to a new Argo Rollouts minor. The early `v0.x` tags predate this rule.

## Documentation Standards

- Update `README.md` and `ARCHITECTURE.md` when behaviour changes.
- Document features without naming release versions or adding unreleased markers; both go stale once a tag is cut. The README documents `main`, and the release workflow generates release notes that list what each version contains. The install example URL in `README.md` is the one exception and is updated with each release.
- Professional tone, plain English, no AI-sounding phrasing.
- No "Made with Bob" comments.

## Resources

- [Argo Rollouts Plugin Docs](https://argo-rollouts.readthedocs.io/en/stable/features/plugins/)
- [HashiCorp go-plugin](https://github.com/hashicorp/go-plugin)
- [Kind](https://kind.sigs.k8s.io/)
