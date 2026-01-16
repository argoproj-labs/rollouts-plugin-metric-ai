## rollouts-plugin-metric-ai

Standalone Argo Rollouts Metric Provider plugin written in Go. It:
- Collects stable/canary pod logs in the Rollout namespace
- **Delegates all AI analysis to an A2A (Agent-to-Agent) agent** - no direct LLM calls
- The agent autonomously fetches logs and performs structured analysis
- On failure, creates GitHub issues with AI-generated analysis

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
            # Required: A2A agent URL
            agentUrl: http://kubernetes-agent.argo-rollouts.svc.cluster.local:8080
            stableLabel: role=stable
            canaryLabel: role=canary
            # Optional: Create GitHub issues on failure
            githubUrl: https://github.com/carlossg/rollouts-demo
            baseBranch: main
```

## How It Works

The plugin **exclusively** uses the A2A (Agent-to-Agent) protocol to delegate all AI analysis to an autonomous Kubernetes Agent. The agent:
- **Autonomously fetches logs** using its own Kubernetes tools
- **Analyzes with structured output** (guaranteed JSON response)
- **Uses its own LLM** (e.g., Gemini) configured via the agent's environment variables
- **No direct LLM calls from the plugin** - all AI functionality is delegated to the agent

An example agent is available at [carlossg/kubernetes-agent](https://github.com/carlossg/kubernetes-agent)

### Configuration Example

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
            # Required: Agent URL (must be explicitly configured)
            agentUrl: http://kubernetes-agent.argo-rollouts.svc.cluster.local:8080
            # Required: Pod selectors for agent to fetch logs
            stableLabel: role=stable
            canaryLabel: role=canary
            # Optional: Create GitHub issues on failures
            githubUrl: https://github.com/carlossg/rollouts-demo
            baseBranch: main
            # Optional: Additional context for AI analysis
            extraPrompt: "Ignore color changes. Consider LoadBalancerNegNotReady a temporary condition."
```

**Key features:**
- ✅ **No LLM configuration needed** - Agent uses its own model
- ✅ **Structured outputs** - Agent returns guaranteed JSON format
- ✅ **Agent fetches logs** - Uses Kubernetes tools autonomously
- ✅ **No API keys in plugin** - Only GitHub token needed for issue creation

### Prerequisites

For the plugin to work, you need:

1. **Kubernetes Agent deployed** in the cluster (see [carlossg/kubernetes-agent](https://github.com/carlossg/kubernetes-agent))
2. **A2A protocol communication** enabled
3. **Agent URL** configured in the AnalysisTemplate (required)

**Important:** The analysis will **fail** if:
- `agentUrl` is not provided in the configuration
- Kubernetes Agent is not available or health check fails
- A2A communication fails

There is no fallback mode - all analysis requires the agent to be available.

### Extra Prompt Feature

The `extraPrompt` parameter allows you to provide additional context to the AI analysis. This text is passed to the Kubernetes Agent and included in the agent's analysis prompt, giving you fine-grained control over what the AI should focus on.

**Use cases:**
- **Performance focus**: "Focus on response times and throughput metrics"
- **Error analysis**: "Pay special attention to error rates and exception patterns"
- **Business context**: "This is a critical payment processing service - prioritize stability"
- **Technical constraints**: "Consider memory usage patterns and database connection limits"
- **Ignoring expected changes**: "Ignore color changes in the output"

**Example:**
```yaml
extraPrompt: "This is a high-traffic e-commerce service. Focus on error rates, response times, and any database connection issues. Consider the business impact of any failures."
```

## Configuration Fields

### Plugin Configuration Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `agentUrl` | string | **Yes** | A2A agent URL (e.g., `http://kubernetes-agent:8080`) |
| `stableLabel` | string | Yes | Label selector for stable pods (e.g., `role=stable`) |
| `canaryLabel` | string | Yes | Label selector for canary pods (e.g., `role=canary`) |
| `githubUrl` | string | No | GitHub repository URL for issue creation |
| `baseBranch` | string | No | Git base branch for issue creation |
| `extraPrompt` | string | No | Additional context text for AI analysis |

**Notes:**
- **Required**: `agentUrl`, `stableLabel`, `canaryLabel`
- **GitHub integration**: Optional, creates issues on failure from plugin side
- **Namespace detection**: Auto-detects namespace from AnalysisRun, no manual config needed
- **Agent configuration**: The agent itself configures which LLM to use (e.g., Gemini)

### Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `GITHUB_TOKEN` | No | GitHub token for issue creation (optional) |
| `LOG_LEVEL` | No | Log level (`panic`, `fatal`, `error`, `warn`, `info`, `debug`, `trace`). Default: `info` |

**Notes:**
- **No API keys needed**: The plugin does not make direct LLM calls
- **GitHub token**: Only required if you want the plugin to create GitHub issues on failure
- **Agent configuration**: The agent has its own environment variables:
  - `GEMINI_MODEL` or similar for LLM configuration
  - `GOOGLE_API_KEY` or similar for LLM API access
  - `GITHUB_TOKEN` for agent-side PR creation (separate from plugin's GitHub integration)

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
- A2A agent communication (requests and responses)
- Agent health checks
- GitHub API interactions
- Performance metrics

## Troubleshooting

### Agent Connection Issues

If the plugin cannot connect to the agent, check:

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

- **"agentUrl is required in plugin configuration"**: Add `agentUrl` field to your AnalysisTemplate
- **"Kubernetes Agent health check failed"**: Verify agent is running and accessible at the configured URL
- **"A2A agent analysis failed"**: Check agent logs and network connectivity
- **GitHub issue creation skipped**: Check logs for "Skipping GitHub issue creation (githubUrl not configured)"
- **Namespace auto-detection**: The agent automatically uses the namespace from the AnalysisRun, no manual config needed

# Testing

```bash
make test
```

This will create a Kind cluster and run the e2e tests.

You can also run only the e2e tests locally with:

```bash
make test-e2e
```
