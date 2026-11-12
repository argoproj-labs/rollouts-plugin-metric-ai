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

package e2e

import (
	"fmt"
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/argoproj-labs/rollouts-plugin-metric-ai/test/utils"
)

var (
	// Set ARGO_ROLLOUTS_INSTALL_SKIP=true to skip operator installation when
	// Argo Rollouts is already present on the cluster.
	skipArgoRolloutsInstall = os.Getenv("ARGO_ROLLOUTS_INSTALL_SKIP") == "true"
	// isArgoRolloutsAlreadyInstalled is set when CRDs are found before BeforeSuite runs.
	isArgoRolloutsAlreadyInstalled = false
)

// TestE2E runs the end-to-end (e2e) test suite for the project. These tests execute in an isolated,
// temporary environment to validate project changes with the purposed to be used in CI jobs.
// The default setup requires Kind, builds the plugin binary locally, serves it via an in-cluster
// HTTP pod, and installs Argo Rollouts via the argo-rollouts-manager operator.
func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	_, _ = fmt.Fprintf(GinkgoWriter, "Starting kubebuilder-example integration test suite\n")
	RunSpecs(t, "e2e suite")
}

var _ = BeforeSuite(func() {
	if !skipArgoRolloutsInstall {
		By("checking if Argo Rollouts is installed already")
		isArgoRolloutsAlreadyInstalled = utils.IsArgoRolloutsCRDsInstalled()
		_, _ = fmt.Fprintf(GinkgoWriter, "Installing Argo Rollouts via RolloutManager operator...\n")
		Expect(utils.InstallArgoRollouts()).To(Succeed(), "Failed to install Argo Rollouts")
	}
})

var _ = AfterSuite(func() {
	// if !skipArgoRolloutsInstall && !isArgoRolloutsAlreadyInstalled {
	// 	_, _ = fmt.Fprintf(GinkgoWriter, "Uninstalling Argo Rollouts...\n")
	// 	utils.UninstallArgoRollouts()
	// }
})
