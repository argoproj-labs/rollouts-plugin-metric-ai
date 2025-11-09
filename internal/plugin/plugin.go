package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/rpc"
	"os"
	"path/filepath"
	"strings"

	v1alpha1 "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"github.com/argoproj/argo-rollouts/utils/plugin/types"
	pluginTypes "github.com/argoproj/argo-rollouts/utils/plugin/types"
	goPlugin "github.com/hashicorp/go-plugin"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	// force vendoring
	"github.com/argoproj/argo-rollouts/rollout/steps/plugin"
	analysisutil "github.com/argoproj/argo-rollouts/utils/analysis"
)

// TODO remove
func test() {
	var x *plugin.StepPlugin
	fmt.Println("x", x)
	// _ = c.TestControllers()
	_ = analysisutil.CurrentAnalysisRuns{}
}

const ProviderType = "MetricAI"

// Configuration loaded at startup
var (
	googleAPIKey       string
	googleCloudProject string
	githubToken        string
)

// loadConfigFromFiles reads configuration from mounted secret files
func loadConfigFromFiles() error {
	secretsDir := "/etc/secrets"

	// Read Google API Key
	apiKeyFile := filepath.Join(secretsDir, "google_api_key")
	if data, err := os.ReadFile(apiKeyFile); err != nil {
		return fmt.Errorf("failed to read Google API key from %s: %v", apiKeyFile, err)
	} else {
		googleAPIKey = strings.TrimSpace(string(data))
		if googleAPIKey == "" {
			return fmt.Errorf("google API key is empty in %s", apiKeyFile)
		}
	}

	// Read Google Cloud Project (optional)
	projectFile := filepath.Join(secretsDir, "google_cloud_project")
	if data, err := os.ReadFile(projectFile); err != nil {
		log.Warnf("Google Cloud Project not found in %s: %v", projectFile, err)
	} else {
		googleCloudProject = strings.TrimSpace(string(data))
	}

	// Read GitHub Token
	tokenFile := filepath.Join(secretsDir, "github_token")
	if data, err := os.ReadFile(tokenFile); err != nil {
		return fmt.Errorf("failed to read GitHub token from %s: %v", tokenFile, err)
	} else {
		githubToken = strings.TrimSpace(string(data))
		if githubToken == "" {
			return fmt.Errorf("github token is empty in %s", tokenFile)
		}
	}

	log.Info("Successfully loaded configuration from mounted files")
	return nil
}

// validateConfig validates that all required configuration is present
func validateConfig() error {
	if googleAPIKey == "" {
		return fmt.Errorf("google API key is required but not configured")
	}
	if githubToken == "" {
		return fmt.Errorf("github token is required but not configured")
	}
	return nil
}

// RpcPlugin implements the metric provider RPC interface
type RpcPlugin struct {
	LogCtx log.Entry
}

type aiConfig struct {
	// optional explicit model
	Model string `json:"model,omitempty"`
	// optional: namespace label selectors for stable/canary pods
	StableLabel string `json:"stableLabel,omitempty"`
	CanaryLabel string `json:"canaryLabel,omitempty"`
	// GitHub base branch
	BaseBranch string `json:"baseBranch,omitempty"`
	// GitHub repository URL
	GitHubURL string `json:"githubUrl,omitempty"`
	// Analysis mode: "default" or "agent"
	AnalysisMode string `json:"analysisMode,omitempty"`
	// Agent URL for agent mode (optional, defaults to kubernetes-agent service)
	AgentURL string `json:"agentUrl,omitempty"`
	// Extra prompt text to append to the AI analysis
	ExtraPrompt string `json:"extraPrompt,omitempty"`
}

func (g *RpcPlugin) InitPlugin() types.RpcError {
	log.Info("Initializing AI metric plugin")

	// Initialize configuration at startup
	if err := loadConfigFromFiles(); err != nil {
		log.WithError(err).Fatal("Failed to load configuration")
	}

	if err := validateConfig(); err != nil {
		log.WithError(err).Fatal("Configuration validation failed")
	}

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
	modelName := cfg.Model
	if modelName == "" {
		modelName = "gemini-2.0-flash"
	}

	log.WithFields(log.Fields{
		"stableSelector": stableSelector,
		"canarySelector": canarySelector,
		"model":          modelName,
	}).Info("Fetching pod logs for analysis")

	// Get Kubernetes client
	kubeClient, err := acquireKubeClient()
	if err != nil {
		log.WithError(err).Error("Failed to acquire Kubernetes client")
		return markMeasurementError(newMeasurement, err)
	}

	// Fetch logs
	ns := analysisRun.Namespace
	stableLogs, err := readFirstPodLogs(context.Background(), kubeClient, ns, stableSelector)
	if err != nil {
		log.WithError(err).Error("Failed to fetch stable pod logs")
		return markMeasurementError(newMeasurement, err)
	}

	canaryLogs, err := readFirstPodLogs(context.Background(), kubeClient, ns, canarySelector)
	if err != nil {
		if errors.IsNotFound(err) {
			log.WithError(err).Warn("Canary pods not found, marking as successful")
			newMeasurement.Value = "1"
			newMeasurement.Phase = v1alpha1.AnalysisPhaseSuccessful
			finishedTime := metav1.Now()
			newMeasurement.FinishedAt = &finishedTime
			return newMeasurement
		}
		log.WithError(err).Error("Failed to fetch canary pod logs")
		return markMeasurementError(newMeasurement, err)
	}

	log.WithFields(log.Fields{
		"stableLogsLength": len(stableLogs),
		"canaryLogsLength": len(canaryLogs),
	}).Info("Successfully fetched pod logs")

	logsContext := "--- STABLE LOGS ---\n" + stableLogs + "\n\n--- CANARY LOGS ---\n" + canaryLogs

	// Get analysis mode (default or agent)
	analysisMode := cfg.AnalysisMode
	if analysisMode == "" {
		analysisMode = AnalysisModeDefault
	}

	// For agent mode, use the namespace from the AnalysisRun (already in 'ns' variable)
	// Agent doesn't need podName - it fetches logs using label selectors

	// Analyze with AI (mode-aware)
	log.WithFields(log.Fields{
		"model": modelName,
		"mode":  analysisMode,
	}).Info("Starting AI analysis")
	analysisJSON, result, aiErr := analyzeWithMode(analysisMode, modelName, logsContext, ns, stableSelector, canarySelector, cfg.AgentURL, cfg.ExtraPrompt)
	if aiErr != nil {
		log.WithError(aiErr).Error("AI analysis failed")
		return markMeasurementError(newMeasurement, aiErr)
	}

	log.WithFields(log.Fields{
		"promote":        result.Promote,
		"confidence":     result.Confidence,
		"analysisLength": len(result.Text),
	}).Info("AI analysis completed")

	// Store analysis in metadata
	if newMeasurement.Metadata == nil {
		newMeasurement.Metadata = make(map[string]string)
	}
	newMeasurement.Metadata["analysis"] = result.Text
	newMeasurement.Metadata["analysisJSON"] = analysisJSON
	newMeasurement.Metadata["confidence"] = fmt.Sprintf("%d", result.Confidence)

	if result.Promote {
		// Success: canary is good
		// Use confidence as a decimal value (0.0 to 1.0)
		newMeasurement.Value = fmt.Sprintf("%.2f", float64(result.Confidence)/100.0)
		newMeasurement.Phase = v1alpha1.AnalysisPhaseSuccessful
		log.Info("Canary promotion recommended by AI analysis")
	} else {
		// Failure: canary has issues
		newMeasurement.Value = "0"
		newMeasurement.Phase = v1alpha1.AnalysisPhaseFailed
		log.Info("Canary promotion not recommended")

		// Create GitHub issue on failure (both default and agent modes)
		if cfg.GitHubURL != "" {
			log.WithField("githubUrl", cfg.GitHubURL).Info("Attempting to create GitHub issue for canary failure")
			if issueErr := createCanaryFailureIssue(logsContext, result.Text, cfg.BaseBranch, cfg.GitHubURL, modelName); issueErr != nil {
				log.WithError(issueErr).Warn("Failed to create GitHub issue")
			} else {
				log.Info("Successfully created GitHub issue for canary failure")
			}
		} else {
			log.Info("Skipping GitHub issue creation (githubUrl not configured)")
		}
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
	// Gemini analysis is synchronous, so just return the measurement
	return measurement
}

// Terminate stops an in-progress measurement
func (p *RpcPlugin) Terminate(analysisRun *v1alpha1.AnalysisRun, metric v1alpha1.Metric, measurement v1alpha1.Measurement) v1alpha1.Measurement {
	log.WithFields(log.Fields{
		"analysisRun": analysisRun.Name,
		"metric":      metric.Name,
	}).Info("Terminating Gemini analysis measurement")
	return measurement
}

// GarbageCollect cleans up old measurements
func (p *RpcPlugin) GarbageCollect(analysisRun *v1alpha1.AnalysisRun, metric v1alpha1.Metric, limit int) pluginTypes.RpcError {
	log.WithFields(log.Fields{
		"analysisRun": analysisRun.Name,
		"metric":      metric.Name,
		"limit":       limit,
	}).Debug("GarbageCollect called (no-op for Gemini plugin)")
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
			if cfg.Model != "" {
				metadata["model"] = cfg.Model
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
// Kubernetes helpers
// ------------------------------

var getKubeClient = func() (*kubernetes.Clientset, error) {
	// Try in-cluster first
	if cfg, err := rest.InClusterConfig(); err == nil {
		return kubernetes.NewForConfig(cfg)
	}
	// Fallback to KUBECONFIG
	kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{},
	)
	restCfg, err := kubeconfig.ClientConfig()
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(restCfg)
}

var fetchFirstPodLogs = func(ctx context.Context, client *kubernetes.Clientset, namespace, labelSelector string) (string, error) {
	log := log.WithFields(log.Fields{
		"namespace":     namespace,
		"labelSelector": labelSelector,
	})
	pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		log.Error("Failed to list pods", err)
		return "", fmt.Errorf("failed to list pods for selector %s in namespace %s: %w", labelSelector, namespace, err)
	}
	if len(pods.Items) == 0 {
		log.Error("No pods found for selector")
		return "", errors.NewNotFound(schema.GroupResource{Group: "", Resource: "pods"}, labelSelector)
	}
	pod := pods.Items[0]
	podLogOpts := &corev1.PodLogOptions{}
	req := client.CoreV1().Pods(namespace).GetLogs(pod.Name, podLogOpts)
	bytes, err := req.DoRaw(ctx)
	if err != nil {
		log.WithField("podName", pod.Name).Error("Failed to fetch logs for pod", err)
		return "", fmt.Errorf("failed to fetch logs for pod %s in namespace %s: %w", pod.Name, namespace, err)
	}
	return string(bytes), nil
}

// indirection to allow test override without touching exported names
var acquireKubeClient = getKubeClient
var readFirstPodLogs = fetchFirstPodLogs

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
