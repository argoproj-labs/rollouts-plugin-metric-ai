package plugin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// getSecretValue retrieves a value from a Kubernetes secret
func getSecretValue(namespace, key string) (string, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		// Fallback to kubeconfig for local development
		homeDir, _ := os.UserHomeDir()
		kubeconfig := filepath.Join(homeDir, ".kube", "config")
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return "", fmt.Errorf("failed to get kubeconfig: %v", err)
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return "", fmt.Errorf("failed to create kubernetes client: %v", err)
	}

	secret, err := clientset.CoreV1().Secrets(namespace).Get(context.Background(), "argo-rollouts", metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return "", fmt.Errorf("secret 'argo-rollouts' not found in namespace '%s'", namespace)
		}
		return "", fmt.Errorf("failed to get secret: %v", err)
	}

	switch key {
	case "github_token":
		githubToken := string(secret.Data["github_token"])
		if githubToken == "" {
			return "", fmt.Errorf("github token not loaded at startup")
		}
		return githubToken, nil
	default:
		return "", fmt.Errorf("unknown secret key: %s", key)
	}
}
