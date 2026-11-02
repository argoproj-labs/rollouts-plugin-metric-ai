package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
)

// AgentClient handles the HTTP calls to the analysis agent
type AgentClient struct {
	baseURL    string
	httpClient *http.Client
}

// AnalysisRequest is the request body sent to the agent's /analyze endpoint
type AnalysisRequest struct {
	UserID  string                 `json:"userId"`
	Prompt  string                 `json:"prompt"`
	Context map[string]interface{} `json:"context"`
}

// AnalysisResponse is the response returned by the agent's /analyze endpoint
type AnalysisResponse struct {
	Analysis    string `json:"analysis"`
	RootCause   string `json:"rootCause"`
	Remediation string `json:"remediation"`
	Promote     bool   `json:"promote"`
	Confidence  int    `json:"confidence"`
	// Rollout paused for human review by the uncertainty gate, if any
	PausedRollout string `json:"pausedRollout,omitempty"`
}

// NewAgentClient creates a new agent HTTP client
func NewAgentClient(baseURL string) *AgentClient {
	return &AgentClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute, // Agent analysis may take time
		},
	}
}

// AnalyzeWithAgent POSTs an analysis request to the agent's /analyze endpoint
// In agent mode, send pod selectors instead of logs - let the agent fetch logs using its tools
func (c *AgentClient) AnalyzeWithAgent(namespace, rolloutName, stableSelector, canarySelector, extraPrompt, githubUrl, baseBranch string, pauseOnUncertainty bool, uncertaintyThreshold int) (*AnalysisResponse, error) {
	log.WithFields(log.Fields{
		"namespace":      namespace,
		"rolloutName":    rolloutName,
		"stableSelector": stableSelector,
		"canarySelector": canarySelector,
		"extraPrompt":    extraPrompt != "",
		"githubUrl":      githubUrl,
		"baseBranch":     baseBranch,
	}).Info("Sending analysis request to Kubernetes Agent")

	// Build the base prompt
	prompt := fmt.Sprintf(
		"Analyze canary deployment for rollout '%s'. Namespace: %s. Use your tools to fetch and compare logs from stable pods (selector: %s) vs canary pods (selector: %s). Determine if canary should be promoted.",
		rolloutName, namespace, stableSelector, canarySelector,
	)

	// Append extra prompt if provided
	if extraPrompt != "" {
		prompt += fmt.Sprintf("\n\nAdditional context: %s", extraPrompt)
	}

	req := AnalysisRequest{
		UserID: "argo-rollouts",
		Prompt: prompt,
		Context: map[string]interface{}{
			"namespace":      namespace,
			"rolloutName":    rolloutName,
			"stableSelector": stableSelector,
			"canarySelector": canarySelector,
		},
	}

	// Include extraPrompt in context for potential use by the agent
	if extraPrompt != "" {
		req.Context["extraPrompt"] = extraPrompt
	}

	// Include GitHub configuration for remediation
	if githubUrl != "" {
		req.Context["repoUrl"] = githubUrl
	}
	if baseBranch != "" {
		req.Context["baseBranch"] = baseBranch
	}

	// Include the uncertainty pause gate when enabled
	if pauseOnUncertainty {
		req.Context["pauseOnUncertainty"] = true
		// 0 is the "not set" sentinel (omitempty on the config struct), so it is
		// omitted and the agent applies its own default threshold
		if uncertaintyThreshold > 0 {
			req.Context["uncertaintyThreshold"] = uncertaintyThreshold
		}
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	resp, err := c.httpClient.Post(
		c.baseURL+"/analyze",
		"application/json",
		bytes.NewBuffer(body),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("agent returned status %d", resp.StatusCode)
	}

	// Read the response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	// Log the full JSON response
	log.WithField("response", string(bodyBytes)).Info("Response from Kubernetes Agent")

	var result AnalysisResponse
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	log.WithFields(log.Fields{
		"promote":     result.Promote,
		"confidence":  result.Confidence,
		"analysis":    result.Analysis,
		"rootCause":   result.RootCause,
		"remediation": result.Remediation,
	}).Info("Received analysis from Kubernetes Agent")

	return &result, nil
}

// HealthCheck checks if the analysis agent is available
// Returns nil if the agent responds (even with 404), as long as it's reachable
func (c *AgentClient) HealthCheck() error {
	resp, err := c.httpClient.Get(c.baseURL + "/")
	if err != nil {
		return fmt.Errorf("health check failed: %v", err)
	}
	defer resp.Body.Close()

	// Accept any response from the agent (even 404) as it means the service is reachable
	// A 404 just means the health endpoint doesn't exist, but the agent is running
	log.WithField("statusCode", resp.StatusCode).Debug("Kubernetes Agent responded to health check")
	return nil
}
