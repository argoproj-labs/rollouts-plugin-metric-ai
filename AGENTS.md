# Agent Instructions for Argo Rollouts AI Metric Plugin

This document provides step-by-step instructions for building, deploying, and testing the Argo Rollouts AI Metric Plugin.

## Prerequisites

- Docker installed and running
- Kind (Kubernetes in Docker) cluster running
- kubectl configured to access the Kind cluster
- Go 1.24+ installed
- GNU Make installed

## Available Make Targets

View all available Make targets:

```bash
# Show all available targets with descriptions
make help
```

Common targets:
- `make build` - Build the Go binary
- `make test` - Run unit tests
- `make docker-build` - Build Docker image
- `make docker-push` - Push Docker image to registry
- `make docker-buildx` - Build and push multi-platform image
- `make fmt` - Format Go code
- `make vet` - Run Go vet
- `make lint` - Run linter

## 1. Build the Plugin Image

Build the Docker image for the plugin using Make:

```bash
# Build image with default settings (uses IMG from Makefile: csanchez/rollouts-plugin-metric-ai:latest)
make docker-build

# Build for multiple platforms (ARM64, AMD64, etc.) - pushes to registry
make docker-buildx

# Build for specific platforms only
make docker-buildx PLATFORMS=linux/arm64,linux/amd64
```

**Note:** 
- The `docker-buildx` target will push to a registry. For local Kind usage, use `docker-build` instead.
- Default image name is defined in Makefile: `csanchez/rollouts-plugin-metric-ai:latest`

## 2. Setup Kind Cluster

Create a Kind cluster for testing (if it doesn't exist):

```bash
# Create Kind cluster with default name
kind create cluster --name rollouts-plugin-metric-ai-test-e2e
```

## 3. Load Image into Kind Cluster

Load the built image into your Kind cluster:

```bash
# Load the image into Kind using the default cluster name
kind load docker-image csanchez/rollouts-plugin-metric-ai:latest --name rollouts-plugin-metric-ai-test-e2e

# Verify the image was loaded
docker exec -it rollouts-plugin-metric-ai-test-e2e-control-plane crictl images | grep rollouts-plugin-metric-ai
```

**Note:** The default Kind cluster name from Makefile is `rollouts-plugin-metric-ai-test-e2e`

## 5. Deploy Argo Rollouts with Plugin

Deploy Argo Rollouts with the AI metric plugin using Kustomize:

```bash
# Deploy Argo Rollouts with plugin configuration
kubectl apply -n argo-rollouts -k config/argo-rollouts

# Wait for the deployment to be ready
kubectl rollout status deployment/argo-rollouts -n argo-rollouts --timeout=5m

# Verify the controller is running
kubectl get pods -n argo-rollouts
```

## 6. Update Plugin After Code Changes

After making changes and rebuilding, restart the controller:

```bash
# Restart the Argo Rollouts controller
kubectl rollout restart deployment/argo-rollouts -n argo-rollouts

# Wait for the rollout to complete
kubectl rollout status deployment/argo-rollouts -n argo-rollouts --timeout=5m

# Verify the controller is running
kubectl get pods -n argo-rollouts
```

## 7. Get Argo Rollouts Controller Logs

View the logs to debug and monitor the plugin:

```bash
# Follow logs in real-time
kubectl logs -f -n argo-rollouts deployment/argo-rollouts

# Or get recent logs
kubectl logs -n argo-rollouts deployment/argo-rollouts --tail=100

# Filter for plugin-specific logs
kubectl logs -n argo-rollouts deployment/argo-rollouts | grep -i "metric-ai\|plugin\|quota"

# View logs with timestamps
kubectl logs -n argo-rollouts deployment/argo-rollouts --timestamps=true
```

## 8. Deploy Test Application

Deploy a sample application for canary testing:

```bash
# Apply the canary demo resources using Kustomize
kubectl apply -k config/rollouts-examples/

# This deploys:
# - Canary service (stable and preview)
# - Rollout with AI analysis
# - Analysis template
# - Traffic generator (hits /color endpoint every second)
```

### Monitor Traffic Generator

The traffic generator continuously hits both stable and canary services:

```bash
# View traffic generator logs
kubectl logs -f deployment/traffic-generator -n rollouts-test-system

# Example output:
# [2025-10-01 12:00:00] STABLE: blue (HTTP 200)
# [2025-10-01 12:00:00] CANARY: yellow (HTTP 200)
```

## 9. Trigger a Canary Rollout

### Option A: Update the Rollout Image

Trigger a canary deployment by updating the image:

```bash
# Update the rollout to a new image version
kubectl argo rollouts set image canary-demo \
  canary-demo=argoproj/rollouts-demo:yellow \
  -n rollouts-test-system

# Or manually edit the rollout
kubectl edit rollout canary-demo -n rollouts-test-system
```

### Option B: Use kubectl patch

```bash
# Patch the rollout with a new image
kubectl patch rollout canary-demo -n rollouts-test-system --type='json' \
  -p='[{"op": "replace", "path": "/spec/template/spec/containers/0/image", "value":"argoproj/rollouts-demo:yellow"}]'
```

### Option C: Restart the Rollout

```bash
# Restart an existing rollout
kubectl argo rollouts restart canary-demo -n rollouts-test-system
```

## 10. Monitor the Rollout

Watch the rollout progress:

```bash
# Watch the rollout status
kubectl argo rollouts get rollout canary-demo -n rollouts-test-system --watch

# View the rollout status
kubectl argo rollouts status canary-demo -n rollouts-test-system

# List all rollouts
kubectl argo rollouts list rollouts -n rollouts-test-system

# View the analysis run
kubectl get analysisrun -n rollouts-test-system

# Describe the analysis run
kubectl describe analysisrun -n rollouts-test-system
```

## 11. Check Plugin Execution

Monitor the plugin execution in the controller logs:

```bash
# Watch for plugin activity
kubectl logs -f -n argo-rollouts deployment/argo-rollouts | grep -E "metric-ai|AI metric|quota|analysis"

# Check for errors
kubectl logs -n argo-rollouts deployment/argo-rollouts | grep -i error

# Check quota status
kubectl logs -n argo-rollouts deployment/argo-rollouts | grep -i quota
```

## 12. View Analysis Results

Check the analysis results:

```bash
# Get the latest analysis run
kubectl get analysisrun -n rollouts-test-system -o wide

# Get detailed analysis results
ANALYSIS_RUN=$(kubectl get analysisrun -n rollouts-test-system -o jsonpath='{.items[0].metadata.name}')
kubectl get analysisrun $ANALYSIS_RUN -n rollouts-test-system -o yaml

# View the measurement results
kubectl get analysisrun $ANALYSIS_RUN -n rollouts-test-system -o jsonpath='{.status.metricResults}'
```

## 13. Debugging Common Issues

### Plugin Not Loading

```bash
# Check if the plugin file exists in the controller
kubectl exec -n argo-rollouts deployment/argo-rollouts -- ls -la /home/argo-rollouts/rollouts-plugin-metric-ai

# Check plugin registration
kubectl logs -n argo-rollouts deployment/argo-rollouts | grep "plugin"
```

### API Key Issues

```bash
# Verify secret exists
kubectl get secret argo-rollouts -n argo-rollouts -o yaml

# Check if secret is mounted
kubectl describe pod -n argo-rollouts deployment/argo-rollouts | grep -A 5 "Mounts:"
```

### Rate Limiting (429 Errors)

```bash
# Check for rate limit errors
kubectl logs -n argo-rollouts deployment/argo-rollouts | grep "429\|rate limit\|quota exceeded"

# View quota status
kubectl logs -n argo-rollouts deployment/argo-rollouts | grep "quota status"
```

## 14. Quick Development Cycle

For rapid development and testing:

```bash
#!/bin/bash
# build-and-deploy.sh

# Set Kind cluster name (from Makefile default)
KIND_CLUSTER="rollouts-plugin-metric-ai-test-e2e"

# Build the image using Make
make docker-build

# Load into Kind
kind load docker-image csanchez/rollouts-plugin-metric-ai:latest --name $KIND_CLUSTER

# Restart controller
kubectl rollout restart deployment argo-rollouts -n argo-rollouts
kubectl rollout status deployment argo-rollouts -n argo-rollouts

# Watch logs
kubectl logs -f -n argo-rollouts deployment/argo-rollouts
```

Make the script executable:

```bash
chmod +x build-and-deploy.sh
./build-and-deploy.sh
```

**Alternative: One-liner using Make**

```bash
make docker-build && \
  kind load docker-image csanchez/rollouts-plugin-metric-ai:latest --name rollouts-plugin-metric-ai-test-e2e && \
  kubectl rollout restart deployment argo-rollouts -n argo-rollouts
```

## 15. Useful Aliases

Add these to your shell profile for convenience:

```bash
# Argo Rollouts aliases
alias k='kubectl'
alias kgr='kubectl argo rollouts get rollout'
alias kwr='kubectl argo rollouts get rollout --watch'
alias klr='kubectl argo rollouts list rollouts'
alias kar='kubectl get analysisrun'

# Log aliases
alias rl='kubectl logs -f -n argo-rollouts deployment/argo-rollouts'
alias rle='kubectl logs -n argo-rollouts deployment/argo-rollouts | grep -i error'

# Build and deploy alias using Make
alias bdp='make docker-build && kind load docker-image csanchez/rollouts-plugin-metric-ai:latest --name rollouts-plugin-metric-ai-test-e2e && kubectl rollout restart deployment argo-rollouts -n argo-rollouts'
```

## 16. Testing Checklist

- [ ] Build image successfully
- [ ] Load image into Kind
- [ ] Controller restarts without errors
- [ ] Plugin file is accessible in controller
- [ ] Secrets are properly mounted
- [ ] Rollout triggers successfully
- [ ] Analysis run is created
- [ ] Plugin executes and logs appear
- [ ] Quota tracking is working
- [ ] No 429 errors or proper retry on rate limits
- [ ] Analysis results are recorded
- [ ] Rollout promotes or aborts based on analysis

## 17. Architecture Check

Verify your cluster and image architecture match:

```bash
# Check Kind node architecture
kubectl get nodes -o jsonpath='{.items[0].status.nodeInfo.architecture}'

# Check your Docker build architecture
docker inspect csanchez/rollouts-plugin-metric-ai:latest | grep Architecture

# They should match (both arm64 or both amd64)
```

**Tip:** If architectures don't match, rebuild with the correct platform:

```bash
# For ARM64 (M1/M2 Macs)
make docker-build PLATFORMS=linux/arm64

# For AMD64 (Intel)
make docker-build PLATFORMS=linux/amd64
```

## Resources

- [Argo Rollouts Documentation](https://argo-rollouts.readthedocs.io/)
- [Argo Rollouts Plugins](https://argo-rollouts.readthedocs.io/en/stable/features/plugins/)
- [Google Gemini API Docs](https://ai.google.dev/gemini-api/docs)
- [Kind Documentation](https://kind.sigs.k8s.io/)
