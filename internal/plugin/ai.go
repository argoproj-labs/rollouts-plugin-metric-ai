package plugin

// AIAnalysisResult represents the result of AI analysis
type AIAnalysisResult struct {
	Text       string `json:"text"`
	Promote    bool   `json:"promote"`
	Confidence int    `json:"confidence"`
	// Rollout paused for human review by the uncertainty gate, if any
	PausedRollout string `json:"pausedRollout,omitempty"`
}
