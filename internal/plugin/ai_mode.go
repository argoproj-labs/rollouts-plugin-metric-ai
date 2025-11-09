package plugin

import (
	"encoding/json"
	"os"

	log "github.com/sirupsen/logrus"
)

// Analysis mode constants
const (
	AnalysisModeDefault = "default" // Current implementation
	AnalysisModeAgent   = "agent"   // Delegate to kubernetes-agent
)

// analyzeWithMode analyzes logs using the specified mode
func analyzeWithMode(mode, modelName, logsContext, namespace, stableSelector, canarySelector, extraPrompt string) (string, AIAnalysisResult, error) {
	log.WithFields(log.Fields{
		"mode":      mode,
		"namespace": namespace,
	}).Info("Analyzing with mode")

	switch mode {
	case AnalysisModeAgent:
		return analyzeWithKubernetesAgent(namespace, stableSelector, canarySelector)
	default:
		params := AIAnalysisParams{
			ModelName:   modelName,
			LogsContext: logsContext,
			ExtraPrompt: extraPrompt,
		}
		return analyzeLogsWithAI(params)
	}
}

// analyzeWithKubernetesAgent delegates analysis to the Kubernetes Agent via A2A
func analyzeWithKubernetesAgent(namespace, stableSelector, canarySelector string) (string, AIAnalysisResult, error) {
	agentURL := os.Getenv("K8S_AGENT_URL")
	if agentURL == "" {
		agentURL = "http://kubernetes-agent.argo-rollouts.svc.cluster.local:8080"
	}

	log.WithField("agentURL", agentURL).Info("Using Kubernetes Agent for analysis")

	client := NewA2AClient(agentURL)

	// Health check first
	if err := client.HealthCheck(); err != nil {
		log.WithError(err).Error("Kubernetes Agent health check failed")
		return "", AIAnalysisResult{}, err
	}

	// In agent mode, don't send logs - let the agent fetch them using its tools
	log.WithFields(log.Fields{
		"stableSelector": stableSelector,
		"canarySelector": canarySelector,
	}).Info("Agent mode: letting agent fetch logs using its own tools")

	// Send request to agent with pod selectors (no logs)
	resp, err := client.AnalyzeWithAgent(namespace, stableSelector, canarySelector)
	if err != nil {
		log.WithError(err).Error("Failed to analyze with kubernetes-agent")
		return "", AIAnalysisResult{}, err
	}

	// Build result object
	result := AIAnalysisResult{
		Text:       resp.Analysis,
		Promote:    resp.Promote,
		Confidence: resp.Confidence,
	}

	// Build JSON response for Argo Rollouts
	jsonResp := map[string]interface{}{
		"text":        resp.Analysis,
		"promote":     resp.Promote,
		"confidence":  resp.Confidence,
		"rootCause":   resp.RootCause,
		"remediation": resp.Remediation,
	}
	if resp.PRLink != "" {
		jsonResp["prLink"] = resp.PRLink
		log.WithField("prLink", resp.PRLink).Info("Agent created a PR with fix")
	}

	rawJSON, err := json.Marshal(jsonResp)
	if err != nil {
		return "", AIAnalysisResult{}, err
	}

	log.WithFields(log.Fields{
		"promote":    resp.Promote,
		"confidence": resp.Confidence,
	}).Info("Analysis completed via Kubernetes Agent")

	return string(rawJSON), result, nil
}
