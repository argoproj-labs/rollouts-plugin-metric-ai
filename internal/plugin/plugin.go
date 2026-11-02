package plugin

import (
	"encoding/json"
	"fmt"
	"net/rpc"

	v1alpha1 "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"github.com/argoproj/argo-rollouts/utils/plugin/types"
	pluginTypes "github.com/argoproj/argo-rollouts/utils/plugin/types"
	goPlugin "github.com/hashicorp/go-plugin"
	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const ProviderType = "MetricAI"

// RpcPlugin implements the metric provider RPC interface
type RpcPlugin struct {
	LogCtx log.Entry
}

// extractRolloutNameFromOwnerRefs attempts to extract the rollout name from AnalysisRun's owner references
func extractRolloutNameFromOwnerRefs(analysisRun *v1alpha1.AnalysisRun) string {
	if analysisRun == nil || analysisRun.OwnerReferences == nil {
		return ""
	}

	// Look for a Rollout owner reference
	for _, owner := range analysisRun.OwnerReferences {
		if owner.Kind == "Rollout" {
			return owner.Name
		}
	}

	return ""
}

type aiConfig struct {
	// optional: namespace label selectors for stable/canary pods
	StableLabel string `json:"stableLabel,omitempty"`
	CanaryLabel string `json:"canaryLabel,omitempty"`
	// Agent URL for the analysis agent (required)
	AgentURL string `json:"agentUrl"`
	// Extra prompt text to append to the AI analysis
	ExtraPrompt string `json:"extraPrompt,omitempty"`
	// GitHub repository URL for creating issues/PRs
	GitHubURL string `json:"githubUrl,omitempty"`
	// Base branch for GitHub PRs
	BaseBranch string `json:"baseBranch,omitempty"`
	// Pause the rollout for human review when the agent promotes with
	// confidence below the threshold (optional, default off)
	PauseOnUncertainty   bool `json:"pauseOnUncertainty,omitempty"`
	UncertaintyThreshold int  `json:"uncertaintyThreshold,omitempty"`
}

func (g *RpcPlugin) InitPlugin() types.RpcError {
	log.Info("Initializing AI metric plugin")
	log.Info("AI metric plugin initialized successfully")
	return types.RpcError{}
}

// Run starts a new measurement
func (p *RpcPlugin) Run(analysisRun *v1alpha1.AnalysisRun, metric v1alpha1.Metric) v1alpha1.Measurement {
	startTime := metav1.Now()
	newMeasurement := v1alpha1.Measurement{
		StartedAt: &startTime,
	}

	log.WithFields(log.Fields{
		"analysisRun": analysisRun.Name,
		"namespace":   analysisRun.Namespace,
		"metric":      metric.Name,
	}).Info("Running AI metric analysis")

	// Parse plugin configuration
	var cfg aiConfig
	if pluginCfg, ok := metric.Provider.Plugin["argoproj-labs/metric-ai"]; ok {
		if err := json.Unmarshal(pluginCfg, &cfg); err != nil {
			log.WithError(err).Error("Failed to parse plugin configuration")
			return markMeasurementError(newMeasurement, err)
		}
	}

	// Set defaults
	stableSelector := cfg.StableLabel
	if stableSelector == "" {
		stableSelector = "role=stable"
	}
	canarySelector := cfg.CanaryLabel
	if canarySelector == "" {
		canarySelector = "role=canary"
	}

	// Validate that agentURL is configured
	if cfg.AgentURL == "" {
		log.Error("AgentURL is required but not configured in the AnalysisTemplate")
		return markMeasurementError(newMeasurement, fmt.Errorf("agentUrl is required in plugin configuration"))
	}

	log.WithFields(log.Fields{
		"stableSelector": stableSelector,
		"canarySelector": canarySelector,
		"agentURL":       cfg.AgentURL,
	}).Info("Delegating analysis to the agent")

	ns := analysisRun.Namespace

	// Extract rollout name from analysisRun (use analysisRun name as rollout identifier)
	// The analysisRun name typically includes the rollout name or is unique per rollout
	rolloutName := analysisRun.Name
	if ownerName := extractRolloutNameFromOwnerRefs(analysisRun); ownerName != "" {
		rolloutName = ownerName
	}

	// Analyze with the agent (all analysis is delegated to the agent)
	log.WithFields(log.Fields{
		"agentURL":    cfg.AgentURL,
		"rolloutName": rolloutName,
		"githubUrl":   cfg.GitHubURL,
		"baseBranch":  cfg.BaseBranch,
	}).Info("Starting agent analysis")
	analysisJSON, result, aiErr := analyzeWithAgent(ns, rolloutName, stableSelector, canarySelector, cfg.AgentURL, cfg.ExtraPrompt, cfg.GitHubURL, cfg.BaseBranch, cfg.PauseOnUncertainty, cfg.UncertaintyThreshold)
	if aiErr != nil {
		log.WithError(aiErr).Error("Agent analysis failed")
		return markMeasurementError(newMeasurement, aiErr)
	}

	log.WithFields(log.Fields{
		"promote":        result.Promote,
		"confidence":     result.Confidence,
		"analysisLength": len(result.Text),
	}).Info("Agent analysis completed")

	// Store analysis in metadata
	if newMeasurement.Metadata == nil {
		newMeasurement.Metadata = make(map[string]string)
	}
	newMeasurement.Metadata["analysis"] = result.Text
	newMeasurement.Metadata["analysisJSON"] = analysisJSON
	newMeasurement.Metadata["confidence"] = fmt.Sprintf("%d", result.Confidence)

	if result.PausedRollout != "" {
		newMeasurement.Metadata["pausedRollout"] = result.PausedRollout
		log.WithField("pausedRollout", result.PausedRollout).Info("Rollout paused for human review by the uncertainty gate")
	}

	if result.Promote {
		// Success: canary is good
		// Use confidence as a decimal value (0.0 to 1.0)
		newMeasurement.Value = fmt.Sprintf("%.2f", float64(result.Confidence)/100.0)
		newMeasurement.Phase = v1alpha1.AnalysisPhaseSuccessful
		log.Info("Canary promotion recommended by agent analysis")
	} else {
		// Failure: canary has issues
		newMeasurement.Value = "0"
		newMeasurement.Phase = v1alpha1.AnalysisPhaseFailed
		log.Info("Canary promotion not recommended - kubernetes-agent can create GitHub issues/PRs via its tools")
	}

	finishedTime := metav1.Now()
	newMeasurement.FinishedAt = &finishedTime
	return newMeasurement
}

// markMeasurementError marks a measurement as errored
func markMeasurementError(m v1alpha1.Measurement, err error) v1alpha1.Measurement {
	m.Phase = v1alpha1.AnalysisPhaseError
	m.Message = err.Error()
	finishedTime := metav1.Now()
	m.FinishedAt = &finishedTime
	return m
}

// Resume checks if an external measurement is finished
func (p *RpcPlugin) Resume(analysisRun *v1alpha1.AnalysisRun, metric v1alpha1.Metric, measurement v1alpha1.Measurement) v1alpha1.Measurement {
	// Agent analysis is synchronous, so just return the measurement
	return measurement
}

// Terminate stops an in-progress measurement
func (p *RpcPlugin) Terminate(analysisRun *v1alpha1.AnalysisRun, metric v1alpha1.Metric, measurement v1alpha1.Measurement) v1alpha1.Measurement {
	log.WithFields(log.Fields{
		"analysisRun": analysisRun.Name,
		"metric":      metric.Name,
	}).Info("Terminating agent analysis measurement")
	return measurement
}

// GarbageCollect cleans up old measurements
func (p *RpcPlugin) GarbageCollect(analysisRun *v1alpha1.AnalysisRun, metric v1alpha1.Metric, limit int) pluginTypes.RpcError {
	log.WithFields(log.Fields{
		"analysisRun": analysisRun.Name,
		"metric":      metric.Name,
		"limit":       limit,
	}).Debug("GarbageCollect called (no-op for this plugin)")
	return pluginTypes.RpcError{}
}

// Type returns the provider type
func (p *RpcPlugin) Type() string {
	return ProviderType
}

// GetMetadata returns metadata about the measurement
func (p *RpcPlugin) GetMetadata(metric v1alpha1.Metric) map[string]string {
	metadata := make(map[string]string)
	metadata["provider"] = ProviderType

	var cfg aiConfig
	if pluginCfg, ok := metric.Provider.Plugin["argoproj-labs/metric-ai"]; ok {
		if err := json.Unmarshal(pluginCfg, &cfg); err == nil {
			if cfg.AgentURL != "" {
				metadata["agentUrl"] = cfg.AgentURL
			}
			if cfg.StableLabel != "" {
				metadata["stableLabel"] = cfg.StableLabel
			}
			if cfg.CanaryLabel != "" {
				metadata["canaryLabel"] = cfg.CanaryLabel
			}
		}
	}

	return metadata
}

// ------------------------------
// RPC Plugin wrapper
// ------------------------------

// RpcMetricPlugin is the implementation of goPlugin.Plugin for serving the metric provider
type RpcMetricPlugin struct {
	Impl pluginTypes.RpcMetricProvider
}

func (p *RpcMetricPlugin) Server(*goPlugin.MuxBroker) (interface{}, error) {
	return &RpcMetricServer{Impl: p.Impl}, nil
}

func (RpcMetricPlugin) Client(b *goPlugin.MuxBroker, c *rpc.Client) (interface{}, error) {
	return &RpcMetricClient{client: c}, nil
}

// RpcMetricServer is the RPC server implementation
type RpcMetricServer struct {
	Impl pluginTypes.RpcMetricProvider
}

func (s *RpcMetricServer) Run(args interface{}, resp *v1alpha1.Measurement) error {
	runArgs, ok := args.(*RunArgs)
	if !ok {
		return fmt.Errorf("invalid args %T", args)
	}
	*resp = s.Impl.Run(runArgs.AnalysisRun, runArgs.Metric)
	return nil
}

func (s *RpcMetricServer) Resume(args interface{}, resp *v1alpha1.Measurement) error {
	resumeArgs, ok := args.(*ResumeArgs)
	if !ok {
		return fmt.Errorf("invalid args %T", args)
	}
	*resp = s.Impl.Resume(resumeArgs.AnalysisRun, resumeArgs.Metric, resumeArgs.Measurement)
	return nil
}

func (s *RpcMetricServer) Terminate(args interface{}, resp *v1alpha1.Measurement) error {
	terminateArgs, ok := args.(*TerminateArgs)
	if !ok {
		return fmt.Errorf("invalid args %T", args)
	}
	*resp = s.Impl.Terminate(terminateArgs.AnalysisRun, terminateArgs.Metric, terminateArgs.Measurement)
	return nil
}

func (s *RpcMetricServer) GarbageCollect(args interface{}, resp *pluginTypes.RpcError) error {
	gcArgs, ok := args.(*GCArgs)
	if !ok {
		return fmt.Errorf("invalid args %T", args)
	}
	*resp = s.Impl.GarbageCollect(gcArgs.AnalysisRun, gcArgs.Metric, gcArgs.Limit)
	return nil
}

func (s *RpcMetricServer) Type(args interface{}, resp *string) error {
	*resp = s.Impl.Type()
	return nil
}

func (s *RpcMetricServer) GetMetadata(args interface{}, resp *map[string]string) error {
	metadataArgs, ok := args.(*MetadataArgs)
	if !ok {
		return fmt.Errorf("invalid args %T", args)
	}
	*resp = s.Impl.GetMetadata(metadataArgs.Metric)
	return nil
}

// RpcMetricClient is the RPC client implementation
type RpcMetricClient struct {
	client *rpc.Client
}

// RPC Args types
type RunArgs struct {
	AnalysisRun *v1alpha1.AnalysisRun
	Metric      v1alpha1.Metric
}

type ResumeArgs struct {
	AnalysisRun *v1alpha1.AnalysisRun
	Metric      v1alpha1.Metric
	Measurement v1alpha1.Measurement
}

type TerminateArgs struct {
	AnalysisRun *v1alpha1.AnalysisRun
	Metric      v1alpha1.Metric
	Measurement v1alpha1.Measurement
}

type GCArgs struct {
	AnalysisRun *v1alpha1.AnalysisRun
	Metric      v1alpha1.Metric
	Limit       int
}

type MetadataArgs struct {
	Metric v1alpha1.Metric
}
