/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package utils

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	. "github.com/onsi/ginkgo/v2" // nolint:revive,staticcheck
)

const (
	ArgoRolloutsNamespace         = "argo-rollouts"
	argoRolloutsManagerKustomize  = "config/rolloutmanager"
	argoRolloutsManagerSystemNS   = "argo-rollouts-manager-system"
	argoRolloutsManagerDeployment = "argo-rollouts-manager-controller-manager"
	pluginBinaryPath              = "bin/rollouts-plugin-metric-ai"
	pluginServerName              = "plugin-server"
	pluginServerImage             = "python:3.13-alpine"
)

func warnError(err error) {
	_, _ = fmt.Fprintf(GinkgoWriter, "warning: %v\n", err)
}

// Run executes the provided command within this context
func Run(cmd *exec.Cmd) (string, error) {
	dir, _ := GetProjectDir()
	cmd.Dir = dir

	if err := os.Chdir(cmd.Dir); err != nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "chdir dir: %q\n", err)
	}

	if len(cmd.Env) == 0 {
		cmd.Env = os.Environ()
	}
	cmd.Env = append(cmd.Env, "GO111MODULE=on")
	command := strings.Join(cmd.Args, " ")
	_, _ = fmt.Fprintf(GinkgoWriter, "running: %q\n", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("%q failed with error %q: %w", command, string(output), err)
	}

	return string(output), nil
}

// InstallPrometheusOperator installs the prometheus Operator to be used to export the enabled metrics.
// func InstallPrometheusOperator() error {
// 	url := fmt.Sprintf(prometheusOperatorURL, prometheusOperatorVersion)
// 	cmd := exec.Command("kubectl", "create", "-f", url)
// 	_, err := Run(cmd)
// 	return err
// }

// UninstallPrometheusOperator uninstalls the prometheus
// func UninstallPrometheusOperator() {
// 	url := fmt.Sprintf(prometheusOperatorURL, prometheusOperatorVersion)
// 	cmd := exec.Command("kubectl", "delete", "-f", url)
// 	if _, err := Run(cmd); err != nil {
// 		warnError(err)
// 	}
// }

// // IsPrometheusCRDsInstalled checks if any Prometheus CRDs are installed
// // by verifying the existence of key CRDs related to Prometheus.
// func IsPrometheusCRDsInstalled() bool {
// 	// List of common Prometheus CRDs
// 	prometheusCRDs := []string{
// 		"prometheuses.monitoring.coreos.com",
// 		"prometheusrules.monitoring.coreos.com",
// 		"prometheusagents.monitoring.coreos.com",
// 	}

// 	cmd := exec.Command("kubectl", "get", "crds", "-o", "custom-columns=NAME:.metadata.name")
// 	output, err := Run(cmd)
// 	if err != nil {
// 		return false
// 	}
// 	crdList := GetNonEmptyLines(output)
// 	for _, crd := range prometheusCRDs {
// 		for _, line := range crdList {
// 			if strings.Contains(line, crd) {
// 				return true
// 			}
// 		}
// 	}

// 	return false
// }

func UninstallArgoRollouts() {
	for _, args := range [][]string{
		{"kubectl", "delete", "rolloutmanager", "argo-rollouts", "-n", ArgoRolloutsNamespace, "--ignore-not-found"},
		{"kubectl", "delete", "pod", pluginServerName, "-n", ArgoRolloutsNamespace, "--ignore-not-found"},
		{"kubectl", "delete", "service", pluginServerName, "-n", ArgoRolloutsNamespace, "--ignore-not-found"},
		{"kubectl", "delete", "-k", argoRolloutsManagerKustomize, "--ignore-not-found"},
	} {
		if _, err := Run(exec.Command(args[0], args[1:]...)); err != nil {
			warnError(err)
		}
	}
}

func targetNodeArch() string {
	cmd := exec.Command("kubectl", "get", "nodes", "-o", "jsonpath={.items[0].status.nodeInfo.architecture}")
	if out, err := Run(cmd); err == nil {
		if arch := strings.TrimSpace(out); arch != "" {
			return arch
		}
	}
	return runtime.GOARCH
}

func InstallArgoRollouts() error {
	// create namespace (idempotent)
	cmd := exec.Command("kubectl", "create", "namespace", ArgoRolloutsNamespace)
	if _, err := Run(cmd); err != nil {
		if !strings.Contains(err.Error(), "AlreadyExists") {
			return err
		}
	}

	// install operator via local kustomization with pinned release image tag
	cmd = exec.Command("kubectl", "apply", "-k", argoRolloutsManagerKustomize)
	if _, err := Run(cmd); err != nil {
		return err
	}

	// wait for operator deployment (installs into argo-rollouts-manager-system namespace)
	cmd = exec.Command("kubectl", "wait", fmt.Sprintf("deployment/%s", argoRolloutsManagerDeployment),
		"--for", "condition=Available",
		"--namespace", argoRolloutsManagerSystemNS,
		"--timeout", "3m",
	)
	if _, err := Run(cmd); err != nil {
		return err
	}

	// build plugin binary for Linux container environment matching cluster node architecture
	cmd = exec.Command("go", "build", "-o", pluginBinaryPath, ".")
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+targetNodeArch(), "CGO_ENABLED=0")
	if _, err := Run(cmd); err != nil {
		return fmt.Errorf("building plugin binary: %w", err)
	}

	// start non-root HTTP server pod to serve the binary
	podManifest := fmt.Sprintf(`apiVersion: v1
kind: Pod
metadata:
  name: %s
  namespace: %s
  labels:
    run: %s
spec:
  securityContext:
    runAsNonRoot: true
    runAsUser: 65534
    runAsGroup: 65534
  containers:
    - name: %s
      image: %s
      command: ["sh", "-c", "mkdir -p /data && exec python3 -m http.server 8080 --directory /data"]
      ports:
        - containerPort: 8080
      securityContext:
        allowPrivilegeEscalation: false
        capabilities:
          drop: ["ALL"]
      volumeMounts:
        - name: data
          mountPath: /data
  volumes:
    - name: data
      emptyDir: {}
`, pluginServerName, ArgoRolloutsNamespace, pluginServerName, pluginServerName, pluginServerImage)
	cmd = exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(podManifest)
	if _, err := Run(cmd); err != nil {
		return err
	}

	// wait for plugin server pod to be ready
	cmd = exec.Command("kubectl", "wait", fmt.Sprintf("pod/%s", pluginServerName),
		"--for", "condition=Ready",
		"--namespace", ArgoRolloutsNamespace,
		"--timeout", "3m",
	)
	if _, err := Run(cmd); err != nil {
		return err
	}

	// copy binary into the pod
	cmd = exec.Command("kubectl", "cp",
		pluginBinaryPath,
		fmt.Sprintf("%s/%s:/data/rollouts-plugin-metric-ai", ArgoRolloutsNamespace, pluginServerName),
	)
	if _, err := Run(cmd); err != nil {
		return err
	}

	// expose pod as a ClusterIP service
	cmd = exec.Command("kubectl", "expose", "pod", pluginServerName,
		"--port=80", "--target-port=8080",
		"--namespace", ArgoRolloutsNamespace,
	)
	if _, err := Run(cmd); err != nil {
		if !strings.Contains(err.Error(), "AlreadyExists") {
			return err
		}
	}

	// apply RolloutManager CR
	cmd = exec.Command("kubectl", "apply", "-f", "config/rolloutmanager/rolloutmanager.yaml")
	if _, err := Run(cmd); err != nil {
		return err
	}

	// wait for RolloutManager CR to report Available phase
	cmd = exec.Command("kubectl", "wait", "rolloutmanager/argo-rollouts",
		"--for=jsonpath={.status.phase}=Available",
		"--namespace", ArgoRolloutsNamespace,
		"--timeout", "3m",
	)
	if _, err := Run(cmd); err != nil {
		return err
	}

	// wait for argo-rollouts deployment provisioned by the operator
	cmd = exec.Command("kubectl", "wait", "deployment/argo-rollouts",
		"--for", "condition=Available",
		"--namespace", ArgoRolloutsNamespace,
		"--timeout", "5m",
	)
	_, err := Run(cmd)
	return err
}

func IsArgoRolloutsCRDsInstalled() bool {
	// All of these must be present for the suite to skip installation.
	requiredCRDs := []string{
		"rollouts.argoproj.io",
		"analysisruns.argoproj.io",
	}
	for _, crd := range requiredCRDs {
		cmd := exec.Command("kubectl", "get", "crd", crd)
		if _, err := Run(cmd); err != nil {
			return false
		}
	}
	return true
}

// GetNonEmptyLines converts given command output string into individual objects
// according to line breakers, and ignores the empty elements in it.
func GetNonEmptyLines(output string) []string {
	var res []string
	elements := strings.Split(output, "\n")
	for _, element := range elements {
		if element != "" {
			res = append(res, element)
		}
	}

	return res
}

// GetProjectDir will return the directory where the project is
func GetProjectDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return wd, fmt.Errorf("failed to get current working directory: %w", err)
	}
	wd = strings.ReplaceAll(wd, "/test/e2e", "")
	return wd, nil
}

// UncommentCode searches for target in the file and remove the comment prefix
// of the target content. The target content may span multiple lines.
func UncommentCode(filename, target, prefix string) error {
	// false positive
	// nolint:gosec
	content, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read file %q: %w", filename, err)
	}
	strContent := string(content)

	idx := strings.Index(strContent, target)
	if idx < 0 {
		return fmt.Errorf("unable to find the code %q to be uncomment", target)
	}

	out := new(bytes.Buffer)
	_, err = out.Write(content[:idx])
	if err != nil {
		return fmt.Errorf("failed to write to output: %w", err)
	}

	scanner := bufio.NewScanner(bytes.NewBufferString(target))
	if !scanner.Scan() {
		return nil
	}
	for {
		if _, err = out.WriteString(strings.TrimPrefix(scanner.Text(), prefix)); err != nil {
			return fmt.Errorf("failed to write to output: %w", err)
		}
		// Avoid writing a newline in case the previous line was the last in target.
		if !scanner.Scan() {
			break
		}
		if _, err = out.WriteString("\n"); err != nil {
			return fmt.Errorf("failed to write to output: %w", err)
		}
	}

	if _, err = out.Write(content[idx+len(target):]); err != nil {
		return fmt.Errorf("failed to write to output: %w", err)
	}

	// false positive
	// nolint:gosec
	if err = os.WriteFile(filename, out.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write file %q: %w", filename, err)
	}

	return nil
}
