## rollouts-plugin-metric-ai

Standalone Argo Rollouts Metric Provider plugin written in Go. It:
- Collects stable/canary pod logs in the Rollout namespace
- Uses Gemini (Google Generative AI) to analyze logs and decide promote/fail
- Supports two analysis modes: **Default** (direct AI) and **Agent** (autonomous Kubernetes agent)
- On failure, creates GitHub issues with AI-generated analysis (both modes)

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
            analysisMode: default
            model: gemini-2.0-flash
            stableLabel: role=stable
            canaryLabel: role=canary
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
            model: gemini-2.0-flash
            stableLabel: role=stable
            canaryLabel: role=canary
            # Optional: Create GitHub issues on failures
            githubUrl: https://github.com/carlossg/rollouts-demo
            baseBranch: main
            extraPrompt: "Pay special attention to database connection errors and memory usage patterns."
```

### Agent Mode (Kubernetes Agent via A2A)
Delegates analysis to an autonomous Kubernetes Agent using the A2A protocol. The agent:
- **Autonomously fetches logs** using its own Kubernetes tools
- **Analyzes with structured output** (guaranteed JSON response)
- **Uses Gemini model** configured via `GEMINI_MODEL` environment variable

An example agent is available at [carlossg/kubernetes-agent](https://github.com/carlossg/kubernetes-agent)

```yaml
apiVersion: argoproj.io/v1alpha1
kind: AnalysisTemplate
metadata:
  name: canary-analysis-agent
spec:
  metrics:
    - name: ai-analysis
      provider:
        plugin:
          argoproj-labs/metric-ai:
            # Agent mode configuration
            analysisMode: agent
            # Required: Agent URL (no default, must be explicitly configured)
            agentUrl: http://kubernetes-agent.argo-rollouts.svc.cluster.local:8080
            # Required: Pod selectors for agent to fetch logs
            stableLabel: role=stable
            canaryLabel: role=canary
            # Optional: Create GitHub issues on failures (works in agent mode too)
            # githubUrl: https://github.com/carlossg/rollouts-demo
            # baseBranch: main
```

**Key differences from default mode:**
- ✅ **No model config needed** - Agent uses its own model
- ✅ **Structured outputs** - Agent returns guaranteed JSON format
- ✅ **Agent fetches logs** - Uses Kubernetes tools autonomously

### Agent Mode Prerequisites

For agent mode to work, you need:

1. **Kubernetes Agent deployed** in the cluster
2. **A2A protocol communication** enabled
3. **Agent URL** configured in the AnalysisTemplate (required, no default)

**Important:** When agent mode is configured, the analysis will **fail** if:
- `agentUrl` is not provided in the configuration
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
| `analysisMode` | string | No | Analysis mode: `default` or `agent` (default: `default`) |
| `model` | string | Yes* | Gemini model to use (*required for default mode only) |
| `stableLabel` | string | Yes | Label selector for stable pods (e.g., `role=stable`) |
| `canaryLabel` | string | Yes | Label selector for canary pods (e.g., `role=canary`) |
| `agentUrl` | string | Yes* | Agent URL (*required for agent mode, no default) |
| `githubUrl` | string | No | GitHub repository URL for issue creation (works in both modes) |
| `baseBranch` | string | No | Git base branch for issue creation |
| `extraPrompt` | string | No | Additional context text for AI analysis (default mode only) |

**Notes:**
- **Default mode**: Requires `model`, `stableLabel`, `canaryLabel`
- **Agent mode**: Requires `agentUrl`, `stableLabel`, `canaryLabel`
- **GitHub integration**: Optional in both modes, creates issues on failure
- **No args needed**: Agent mode auto-detects namespace from AnalysisRun

### Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `GOOGLE_API_KEY` | Yes | Google API key for Gemini AI (required for default mode) |
| `GITHUB_TOKEN` | No | GitHub token for issue creation (optional, used in both modes) |
| `LOG_LEVEL` | No | Log level (`panic`, `fatal`, `error`, `warn`, `info`, `debug`, `trace`). Default: `info` |

**Agent Mode Notes:**
- Agent has its own `GEMINI_MODEL` environment variable
- Agent requires `GOOGLE_API_KEY` and optionally `GITHUB_TOKEN` (for PR creation)
- Plugin only needs `GITHUB_TOKEN` if creating issues from the plugin side

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

1. **Agent URL is configured:**
   ```bash
   # Check AnalysisTemplate configuration
   kubectl get analysistemplate -n <namespace> <template-name> -o yaml | grep agentUrl
   ```

2. **Kubernetes Agent is deployed and running:**
   ```bash
   kubectl get pods -n argo-rollouts | grep kubernetes-agent
   kubectl logs -n argo-rollouts deployment/kubernetes-agent
   ```

3. **Agent health check:**
   ```bash
   # From within cluster
   curl http://kubernetes-agent.argo-rollouts.svc.cluster.local:8080/a2a/health
   ```

4. **Plugin logs for agent communication:**
   ```bash
   kubectl logs -n argo-rollouts deployment/argo-rollouts | grep -E "agent|A2A|Attempting to create"
   ```

### Common Issues

- **"agent mode requires agentUrl to be configured"**: Add `agentUrl` field to your AnalysisTemplate
- **"Kubernetes Agent health check failed"**: Verify agent is running and accessible at the configured URL
- **"Failed to analyze with kubernetes-agent"**: Check agent logs and network connectivity
- **GitHub issue creation skipped**: Check logs for "Skipping GitHub issue creation (githubUrl not configured)"
- **Namespace auto-detection**: Agent automatically uses the namespace from the AnalysisRun, no manual config needed

# Testing

```bash
make test
```

This will create a Kind cluster and run the e2e tests.

You can also run only the e2e tests locally with:

```bash
make test-e2e
```
