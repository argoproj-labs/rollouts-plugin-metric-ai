## rollouts-plugin-metric-ai

Standalone Argo Rollouts Metric Provider plugin written in Go. It:
- Collects stable/canary pod logs in the Rollout namespace
- Uses Gemini (Google Generative AI) to analyze logs and decide promote/fail
- On failure, uses GitHub API to open a PR with AI-proposed fixes (or a proposal file)

Configuration snippet in argo-rollouts-config ConfigMap:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: argo-rollouts-config
data:
  metricProviderPlugins: |-
    - name: "argoproj-labs/metric-ai"
      location: "file://./rollouts-plugin-metric-ai/bin/metric-ai"
```

Use in an AnalysisTemplate:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: AnalysisTemplate
metadata:
  name: canary-analysis
spec:
  metrics:
    - name: ai-analysis
      provider:
        plugin:
          argoproj-labs/metric-ai:
            model: gemini-2.0-flash-exp
            baseBranch: main
            githubUrl: https://github.com/carlossg/rollouts-demo
```

## Analysis Modes

The plugin supports two analysis modes:

### Default Mode (Direct AI Analysis)
Uses Gemini AI directly to analyze pod logs:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: AnalysisTemplate
metadata:
  name: canary-analysis-default
spec:
  metrics:
    - name: ai-analysis
      provider:
        plugin:
          argoproj-labs/metric-ai:
            analysisMode: default
            model: gemini-2.0-flash-exp
            stablePodLabel: app=rollouts-demo,revision=stable
            canaryPodLabel: app=rollouts-demo,revision=canary
            baseBranch: main
            githubUrl: https://github.com/carlossg/rollouts-demo
            extraPrompt: "Pay special attention to database connection errors and memory usage patterns."
```

### Agent Mode (Kubernetes Agent via A2A)
Delegates analysis to a Kubernetes Agent using the A2A protocol for enhanced analysis.

An example agent is available at [carlossg/kubernetes-agent](https://github.com/carlossg/kubernetes-agent)

```yaml
apiVersion: argoproj.io/v1alpha1
kind: AnalysisTemplate
metadata:
  name: canary-analysis-agent
spec:
  args:
    - name: namespace
    - name: canary-pod
  metrics:
    - name: ai-analysis
      provider:
        plugin:
          argoproj-labs/metric-ai:
            analysisMode: agent
            namespace: "{{args.namespace}}"
            podName: "{{args.canary-pod}}"
            # Fallback fields for default mode
            stablePodLabel: app=rollouts-demo,revision=stable
            canaryPodLabel: app=rollouts-demo,revision=canary
            model: gemini-2.0-flash-exp
            baseBranch: main
            githubUrl: https://github.com/carlossg/rollouts-demo
            extraPrompt: "Focus on error rates and response times. Consider this a critical production deployment."
```

### Agent Mode Prerequisites

For agent mode to work, you need:

1. **Kubernetes Agent deployed** in the cluster
2. **A2A protocol communication** enabled
3. **Environment variable** `K8S_AGENT_URL` (defaults to `http://kubernetes-agent.argo-rollouts.svc.cluster.local:8080`)

**Important:** When agent mode is explicitly configured, the analysis will **fail** if:
- `namespace` or `podName` arguments are not provided
- Kubernetes Agent is not available or health check fails
- A2A communication fails

The plugin will **not** fall back to default mode. This ensures you know when agent mode is not working as expected.

### Extra Prompt Feature

The `extraPrompt` parameter allows you to provide additional context to the AI analysis. This text is appended to the standard analysis prompt, giving you fine-grained control over what the AI should focus on.

**Use cases:**
- **Performance focus**: "Focus on response times and throughput metrics"
- **Error analysis**: "Pay special attention to error rates and exception patterns"
- **Business context**: "This is a critical payment processing service - prioritize stability"
- **Technical constraints**: "Consider memory usage patterns and database connection limits"

**Example:**
```yaml
extraPrompt: "This is a high-traffic e-commerce service. Focus on error rates, response times, and any database connection issues. Consider the business impact of any failures."
```

## Configuration Fields

### Plugin Configuration Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `model` | string | Yes | Gemini model to use (e.g., `gemini-2.0-flash-exp`) |
| `analysisMode` | string | No | Analysis mode: `default` or `agent` (default: `default`) |
| `stablePodLabel` | string | Yes* | Label selector for stable pods (*required for default mode) |
| `canaryPodLabel` | string | Yes* | Label selector for canary pods (*required for default mode) |
| `namespace` | string | Yes* | Namespace for agent mode (*required for agent mode) |
| `podName` | string | Yes* | Pod name for agent mode (*required for agent mode) |
| `baseBranch` | string | No | Git base branch for PR creation |
| `githubUrl` | string | No | GitHub repository URL for issue/PR creation |
| `extraPrompt` | string | No | Additional context text to append to the AI analysis prompt |

### Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `GOOGLE_API_KEY` | Yes | Google API key for Gemini AI |
| `GITHUB_TOKEN` | No | GitHub token for issue/PR creation |
| `AUTO_PR_ENABLED` | No | Enable automatic PR creation (`true`/`false`) |
| `K8S_AGENT_URL` | No | Kubernetes Agent URL (default: `http://kubernetes-agent.argo-rollouts.svc.cluster.local:8080`) |
| `LOG_LEVEL` | No | Log level (`panic`, `fatal`, `error`, `warn`, `info`, `debug`, `trace`). Default: `info` |

## Building

Build locally:

```bash
# Build Go binary
make build

# Build Docker image
make docker-build

# Build multi-platform Docker image and push to registry
make docker-buildx
```

## CI/CD

GitHub Actions automatically builds and publishes Docker images to GitHub Container Registry (ghcr.io) on pushes to main:
- Images are tagged with the commit SHA
- Multi-platform builds (amd64, arm64)
- Available at: `ghcr.io/carlossg/argo-rollouts/rollouts-plugin-metric-ai:<commit-sha>`

## Examples

See `examples/` directory for:
- Analysis template configuration
- Argo Rollouts ConfigMap setup

See `config/rollouts-examples/` for complete deployment examples including:
- Rollout with AI analysis
- Canary services and ingress
- Traffic generator for testing

## Debugging and Logging

The plugin supports configurable logging levels to help with debugging and monitoring. You can control the log level using the `LOG_LEVEL` environment variable.

### Available Log Levels

- `panic`: Only panic level messages
- `fatal`: Fatal and panic level messages  
- `error`: Error, fatal, and panic level messages
- `warn`: Warning, error, fatal, and panic level messages
- `info`: Info, warning, error, fatal, and panic level messages (default)
- `debug`: Debug, info, warning, error, fatal, and panic level messages
- `trace`: All log messages including trace level

### Setting Log Level

#### Via Environment Variable
```bash
export LOG_LEVEL=debug
```

#### Via Kubernetes Deployment
Update the Argo Rollouts deployment to include the environment variable:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: argo-rollouts
spec:
  template:
    spec:
      containers:
      - name: argo-rollouts
        env:
        - name: LOG_LEVEL
          value: "debug"
```

#### Via Kustomize (recommended)
The deployment configuration in `config/argo-rollouts/kustomization.yaml` already includes the `LOG_LEVEL` environment variable set to `debug` by default.

### Viewing Plugin Logs

To view the plugin logs with debug information:

```bash
# View all logs
kubectl logs -f -n argo-rollouts deployment/argo-rollouts

# Filter for plugin-specific logs
kubectl logs -n argo-rollouts deployment/argo-rollouts | grep -E "metric-ai|AI metric|plugin"

# View logs with timestamps
kubectl logs -n argo-rollouts deployment/argo-rollouts --timestamps=true
```

### Debug Information

When `LOG_LEVEL=debug` or `LOG_LEVEL=trace`, the plugin will log:
- Detailed configuration parsing
- Pod log fetching operations
- AI analysis requests and responses
- GitHub API interactions
- Rate limiting and retry attempts
- Performance metrics
- Agent mode communication (A2A protocol)
- Fallback behavior when agent mode fails

## Troubleshooting

### Agent Mode Issues

If agent mode is not working, check:

1. **Kubernetes Agent is deployed:**
   ```bash
   kubectl get pods -n argo-rollouts | grep kubernetes-agent
   ```

2. **Agent health check:**
   ```bash
   kubectl logs -n argo-rollouts deployment/argo-rollouts | grep "agent"
   ```

3. **A2A communication:**
   ```bash
   kubectl logs -n argo-rollouts deployment/argo-rollouts | grep "A2A\|agent"
   ```

4. **Environment variable:**
   ```bash
   kubectl get deployment argo-rollouts -n argo-rollouts -o yaml | grep K8S_AGENT_URL
   ```

### Common Issues

- **"Agent mode requires namespace and podName"**: Ensure both fields are provided in the AnalysisTemplate
- **"Kubernetes Agent health check failed"**: Check if the agent is running and accessible
- **"Failed to analyze with kubernetes-agent"**: Check agent logs and network connectivity
- **Analysis fails in agent mode**: The plugin will fail the analysis if agent mode is configured but the agent is unavailable. Check the prerequisites above.

# Testing

```bash
make test
```

This will create a Kind cluster and run the e2e tests.

You can also run only the e2e tests locally with:

```bash
make test-e2e
```
