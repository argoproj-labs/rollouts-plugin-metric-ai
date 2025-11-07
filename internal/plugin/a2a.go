package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
)

// A2AClient handles communication with the Kubernetes Agent via A2A protocol
type A2AClient struct {
	baseURL    string
	httpClient *http.Client
}

// A2ARequest represents a request to the Kubernetes Agent
type A2ARequest struct {
	UserID  string                 `json:"userId"`
	Prompt  string                 `json:"prompt"`
	Context map[string]interface{} `json:"context"`
}

// A2AResponse represents the response from the Kubernetes Agent
type A2AResponse struct {
	Analysis    string `json:"analysis"`
	RootCause   string `json:"rootCause"`
	Remediation string `json:"remediation"`
	PRLink      string `json:"prLink,omitempty"`
	Promote     bool   `json:"promote"`
	Confidence  int    `json:"confidence"`
}

// NewA2AClient creates a new A2A client
func NewA2AClient(baseURL string) *A2AClient {
	return &A2AClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute, // Agent analysis may take time
		},
	}
}

// AnalyzeWithAgent sends analysis request to Kubernetes Agent
func (c *A2AClient) AnalyzeWithAgent(namespace, podName, stableLogs, canaryLogs string) (*A2AResponse, error) {
	log.WithFields(log.Fields{
		"namespace": namespace,
		"podName":   podName,
	}).Info("Sending analysis request to Kubernetes Agent")

	req := A2ARequest{
		UserID: "argo-rollouts",
		Prompt: fmt.Sprintf(
			"Analyze canary deployment issue. Namespace: %s, Pod: %s. Compare stable vs canary behavior and determine if canary should be promoted.",
			namespace, podName,
		),
		Context: map[string]interface{}{
			"namespace":  namespace,
			"podName":    podName,
			"stableLogs": stableLogs,
			"canaryLogs": canaryLogs,
		},
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	resp, err := c.httpClient.Post(
		c.baseURL+"/a2a/analyze",
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

	var result A2AResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	log.WithFields(log.Fields{
		"promote":    result.Promote,
		"confidence": result.Confidence,
		"hasPR":      result.PRLink != "",
	}).Info("Received analysis from Kubernetes Agent")

	return &result, nil
}

// HealthCheck checks if the Kubernetes Agent is available
// Returns nil if the agent responds (even with 404), as long as it's reachable
func (c *A2AClient) HealthCheck() error {
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

// splitLogs splits the combined logs context into stable and canary logs
func splitLogs(logsContext string) (string, string) {
	// The logsContext format: "--- STABLE LOGS ---\n...\n--- CANARY LOGS ---\n..."
	const stableMarker = "--- STABLE LOGS ---"
	const canaryMarker = "--- CANARY LOGS ---"

	stableIdx := findIndex(logsContext, stableMarker)
	canaryIdx := findIndex(logsContext, canaryMarker)

	if stableIdx == -1 || canaryIdx == -1 {
		// Fallback: if markers not found, treat all as stable logs
		return logsContext, ""
	}

	stableLogs := logsContext[stableIdx+len(stableMarker) : canaryIdx]
	canaryLogs := logsContext[canaryIdx+len(canaryMarker):]

	return stableLogs, canaryLogs
}

func findIndex(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
