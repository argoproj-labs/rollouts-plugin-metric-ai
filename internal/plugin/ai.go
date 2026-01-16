package plugin

// AIAnalysisResult represents the result of AI analysis
type AIAnalysisResult struct {
	Text       string `json:"text"`
	Promote    bool   `json:"promote"`
	Confidence int    `json:"confidence"`
}

// AIAnalysisParams represents parameters for AI analysis
// This is kept for backwards compatibility but is no longer used
// since all analysis is delegated to the A2A agent
type AIAnalysisParams struct {
	ModelName   string
	LogsContext string
	ExtraPrompt string
}
