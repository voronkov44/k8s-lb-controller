//go:build e2e
// +build e2e

/*
Copyright 2026.

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
	"os/exec"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/voronkov44/k8s-lb-controller/test/utils"
)

var (
	// managerImage is the manager image to be built and loaded for testing.
	managerImage   = envOrDefault("E2E_MANAGER_IMAGE", "example.com/k8s-lb-controller:e2e")
	dataplaneImage = envOrDefault("E2E_DATAPLANE_IMAGE", "example.com/k8s-lb-controller-dataplane:e2e")
)

const (
	deployModeLocal     = "local"
	deployModeDataplane = "dataplane"
)

// TestE2E runs the e2e test suite to validate the controller in an isolated Kind cluster.
func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	_, _ = fmt.Fprintf(GinkgoWriter, "Starting k8s-lb-controller e2e test suite\n")
	RunSpecs(t, "e2e suite")
}

var _ = BeforeSuite(func() {
	By("validating the e2e deployment mode")
	ExpectWithOffset(1, validateDeployMode(e2eDeployMode())).To(Succeed())

	By("building the manager image")
	cmd := exec.Command("make", "docker-build", fmt.Sprintf("IMG=%s", managerImage))
	_, err := utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build the manager image")

	By("loading the manager image on Kind")
	err = utils.LoadImageToKindClusterWithName(managerImage)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load the manager image into Kind")

	if isDataplaneMode() {
		By("building the dataplane image")
		cmd = exec.Command("make", "docker-build-dataplane", fmt.Sprintf("DATAPLANE_IMG=%s", dataplaneImage))
		_, err = utils.Run(cmd)
		ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build the dataplane image")

		By("loading the dataplane image on Kind")
		err = utils.LoadImageToKindClusterWithName(dataplaneImage)
		ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load the dataplane image into Kind")
	}
})

func e2eDeployMode() string {
	return strings.TrimSpace(os.Getenv("E2E_DEPLOY_MODE"))
}

func isDataplaneMode() bool {
	return normalizedDeployMode() == deployModeDataplane
}

func normalizedDeployMode() string {
	mode := e2eDeployMode()
	if mode == "" {
		return deployModeLocal
	}

	return mode
}

func validateDeployMode(mode string) error {
	switch mode {
	case "", deployModeLocal, deployModeDataplane:
		return nil
	default:
		return fmt.Errorf("unsupported E2E_DEPLOY_MODE %q", mode)
	}
}

func envOrDefault(name, fallback string) string {
	if value, ok := os.LookupEnv(name); ok && strings.TrimSpace(value) != "" {
		return value
	}

	return fallback
}
