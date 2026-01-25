package plugin

// AIAnalysisResult represents the result of AI analysis
type AIAnalysisResult struct {
	Text       string `json:"text"`
	Promote    bool   `json:"promote"`
	Confidence int    `json:"confidence"`

	// Multi-model fields
	ModelResults    []ModelAnalysisResult `json:"modelResults,omitempty"`
	VotingRationale string                `json:"votingRationale,omitempty"`
}

// ModelAnalysisResult represents a single model's analysis (mirrors A2A response)
type ModelAnalysisResult struct {
	ModelName       string `json:"modelName"`
	Analysis        string `json:"analysis"`
	RootCause       string `json:"rootCause"`
	Remediation     string `json:"remediation"`
	Promote         bool   `json:"promote"`
	Confidence      int    `json:"confidence"`
	ExecutionTimeMs int64  `json:"executionTimeMs"`
	Error           string `json:"error,omitempty"`
}

// AIAnalysisParams represents parameters for AI analysis
// This is kept for backwards compatibility but is no longer used
// since all analysis is delegated to the A2A agent
type AIAnalysisParams struct {
	ModelName   string
	LogsContext string
	ExtraPrompt string
}
