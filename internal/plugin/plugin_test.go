package plugin

import (
	"context"
	"encoding/json"
	"testing"

	v1alpha1 "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"k8s.io/client-go/kubernetes"
)

func TestRun_ParsesConfigAndReturnsResult(t *testing.T) {
	p := &RpcPlugin{}
	analysisRun := &v1alpha1.AnalysisRun{}
	analysisRun.Name = "test-analysis"
	analysisRun.Namespace = "default"

	cfg := aiConfig{AgentURL: "http://localhost:8080"}
	b, _ := json.Marshal(cfg)

	metric := v1alpha1.Metric{
		Name: "ai-test",
		Provider: v1alpha1.MetricProvider{
			Plugin: map[string]json.RawMessage{
				"argoproj-labs/metric-ai": b,
			},
		},
	}

	// Override agent analysis call to avoid external dependency
	old := analyzeWithAgent
	analyzeWithAgent = func(namespace, rolloutName, stableSelector, canarySelector, agentURL, extraPrompt string) (string, AIAnalysisResult, error) {
		return `{"text":"ok","promote":true,"confidence":100}`, AIAnalysisResult{Text: "ok", Promote: true, Confidence: 100}, nil
	}
	t.Cleanup(func() { analyzeWithAgent = old })

	// Stub kube access
	oldKC := acquireKubeClient
	acquireKubeClient = func() (*kubernetes.Clientset, error) { return nil, nil }
	t.Cleanup(func() { acquireKubeClient = oldKC })

	oldLogs := readFirstPodLogs
	readFirstPodLogs = func(ctx context.Context, _ *kubernetes.Clientset, _ string, _ string) (string, error) {
		return "dummy", nil
	}
	t.Cleanup(func() { readFirstPodLogs = oldLogs })

	measurement := p.Run(analysisRun, metric)
	if measurement.Phase != v1alpha1.AnalysisPhaseSuccessful {
		t.Fatalf("expected successful, got %s with message: %s", measurement.Phase, measurement.Message)
	}
	if measurement.Value != "1.00" {
		t.Fatalf("expected value '1.00' (confidence 100%%), got '%s'", measurement.Value)
	}
	// Verify confidence is stored in metadata
	if measurement.Metadata["confidence"] != "100" {
		t.Fatalf("expected confidence '100', got '%s'", measurement.Metadata["confidence"])
	}
}

func TestRun_FailureCreatesIssue(t *testing.T) {
	p := &RpcPlugin{}
	analysisRun := &v1alpha1.AnalysisRun{}
	analysisRun.Name = "test-analysis"
	analysisRun.Namespace = "default"

	cfg := aiConfig{
		AgentURL:  "http://localhost:8080",
		GitHubURL: "https://github.com/owner/repo",
	}
	b, _ := json.Marshal(cfg)

	metric := v1alpha1.Metric{
		Name: "ai-test",
		Provider: v1alpha1.MetricProvider{
			Plugin: map[string]json.RawMessage{
				"argoproj-labs/metric-ai": b,
			},
		},
	}

	// Override agent analysis call to return failure
	old := analyzeWithAgent
	analyzeWithAgent = func(namespace, rolloutName, stableSelector, canarySelector, agentURL, extraPrompt string) (string, AIAnalysisResult, error) {
		return `{"text":"canary is bad","promote":false,"confidence":90}`, AIAnalysisResult{Text: "canary is bad", Promote: false, Confidence: 90}, nil
	}
	t.Cleanup(func() { analyzeWithAgent = old })

	// Stub kube access
	oldKC := acquireKubeClient
	acquireKubeClient = func() (*kubernetes.Clientset, error) { return nil, nil }
	t.Cleanup(func() { acquireKubeClient = oldKC })

	oldLogs := readFirstPodLogs
	readFirstPodLogs = func(ctx context.Context, _ *kubernetes.Clientset, _ string, _ string) (string, error) {
		return "dummy", nil
	}
	t.Cleanup(func() { readFirstPodLogs = oldLogs })

	measurement := p.Run(analysisRun, metric)
	if measurement.Phase != v1alpha1.AnalysisPhaseFailed {
		t.Fatalf("expected failed, got %s", measurement.Phase)
	}
	if measurement.Value != "0" {
		t.Fatalf("expected value '0', got '%s'", measurement.Value)
	}
}

func TestGetMetadata(t *testing.T) {
	p := &RpcPlugin{}

	cfg := aiConfig{
		AgentURL:    "http://localhost:8080",
		StableLabel: "app=stable",
		CanaryLabel: "app=canary",
	}
	b, _ := json.Marshal(cfg)

	metric := v1alpha1.Metric{
		Name: "ai-test",
		Provider: v1alpha1.MetricProvider{
			Plugin: map[string]json.RawMessage{
				"argoproj-labs/metric-ai": b,
			},
		},
	}

	metadata := p.GetMetadata(metric)

	if metadata["provider"] != ProviderType {
		t.Fatalf("expected provider %s, got %s", ProviderType, metadata["provider"])
	}
	if metadata["agentUrl"] != "http://localhost:8080" {
		t.Fatalf("expected agentUrl http://localhost:8080, got %s", metadata["agentUrl"])
	}
}

func TestType(t *testing.T) {
	p := &RpcPlugin{}
	if p.Type() != ProviderType {
		t.Fatalf("expected type %s, got %s", ProviderType, p.Type())
	}
}
