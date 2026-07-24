# Integration Tests

## Overview

The project includes integration tests that make real API calls to Google Gemini AI. These tests are **automatically skipped** during normal test runs to avoid consuming API quota and requiring credentials.

## Running Integration Tests

### Prerequisites

1. **API Key**: You need a valid API key for any OpenAI-compatible endpoint
2. **Set Environment Variable**:
   ```bash
   export ANALYSIS_API_KEY="your-api-key-here"
   ```

### Run All Integration Tests

```bash
go test ./internal/plugin/... -run TestAnalyzeLogsWithAI_Integration -v
```

### Run Specific Integration Test

```bash
# Test main functionality
go test ./internal/plugin/... -run TestAnalyzeLogsWithAI_Integration$ -v

# Test error handling
go test ./internal/plugin/... -run TestAnalyzeLogsWithAI_Integration_ErrorHandling -v
```

### Skip Integration Tests (Default)

Integration tests are automatically skipped when running:

```bash
# Regular test run (skips integration tests)
go test ./internal/plugin/... -v

# Explicit short mode (skips integration tests)
go test ./internal/plugin/... -v -short
```

## What the Integration Tests Cover

### `TestAnalyzeLogsWithAI_Integration`

- Makes a real call to Google Gemini API
- Analyzes sample stable vs canary logs
- Validates JSON response structure
- Verifies promote decision logic
- Checks confidence score range (0-100)
- Validates quota tracking

### `TestAnalyzeLogsWithAI_Integration_ErrorHandling`

#### Sub-test: `invalid_model`
- Tests error handling with an invalid model name
- Verifies proper error propagation

#### Sub-test: `empty_logs`
- Tests behavior with empty log input
- Validates default behavior per the AI prompt

## Sample Output

```
=== RUN   TestAnalyzeLogsWithAI_Integration
    ai_test.go:568: Calling real Google Gemini API...
    ai_test.go:576: Raw JSON response: {"text":"Both versions appear healthy...","promote":true,"confidence":85}
    ai_test.go:577: Promote decision: true
    ai_test.go:578: Analysis text: Both versions appear healthy...
    ai_test.go:594: Parsed - Text: Both versions appear healthy..., Promote: true, Confidence: 85
    ai_test.go:617: Quota status after call: used=1, limit=200, remaining=199
--- PASS: TestAnalyzeLogsWithAI_Integration (2.3s)
```

## Notes

- **API Quota**: These tests consume your Google Gemini API quota
- **Network Required**: Tests will fail without internet connectivity
- **Rate Limits**: Multiple runs may trigger rate limiting (429 errors)
- **Cost**: Gemini API has free tier limits; check your usage
