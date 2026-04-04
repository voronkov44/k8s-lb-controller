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
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/f1lzz/k8s-lb-controller/test/utils"
)

const (
	namespace        = "k8s-lb-controller-system"
	serviceNamespace = "k8s-lb-controller-e2e"
	metricsService   = "k8s-lb-controller-controller-manager-metrics-service"
)

var _ = Describe("Manager", Ordered, func() {
	var controllerPodName string

	BeforeAll(func() {
		By("creating manager namespace")
		cmd := exec.Command("kubectl", "create", "ns", namespace)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create manager namespace")

		By("labeling the manager namespace with restricted pod security")
		cmd = exec.Command("kubectl", "label", "--overwrite", "ns", namespace,
			"pod-security.kubernetes.io/enforce=restricted")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to label manager namespace")

		By("creating service test namespace")
		cmd = exec.Command("kubectl", "create", "ns", serviceNamespace)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create service test namespace")

		By("deploying the controller-manager")
		cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", managerImage))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager")
	})

	AfterAll(func() {
		By("cleaning up the curl pod for metrics")
		cmd := exec.Command("kubectl", "delete", "pod", "curl-metrics", "-n", namespace, "--ignore-not-found")
		_, _ = utils.Run(cmd)

		By("undeploying the controller-manager")
		cmd = exec.Command("make", "undeploy")
		_, _ = utils.Run(cmd)

		By("removing service test namespace")
		cmd = exec.Command("kubectl", "delete", "ns", serviceNamespace, "--ignore-not-found")
		_, _ = utils.Run(cmd)

		By("removing manager namespace")
		cmd = exec.Command("kubectl", "delete", "ns", namespace, "--ignore-not-found")
		_, _ = utils.Run(cmd)
	})

	AfterEach(func() {
		specReport := CurrentSpecReport()
		if !specReport.Failed() {
			return
		}

		By("fetching controller manager pod logs")
		logs, err := controllerLogs(controllerPodName)
		if err == nil {
			_, _ = fmt.Fprintf(GinkgoWriter, "Controller logs:\n%s\n", logs)
		}

		By("fetching Kubernetes events")
		cmd := exec.Command("kubectl", "get", "events", "-n", namespace, "--sort-by=.lastTimestamp")
		eventsOutput, err := utils.Run(cmd)
		if err == nil {
			_, _ = fmt.Fprintf(GinkgoWriter, "Kubernetes events:\n%s\n", eventsOutput)
		}
	})

	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

	Context("Manager", func() {
		It("should run successfully", func() {
			Eventually(func(g Gomega) {
				podName, err := activeControllerPod()
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(podName).To(ContainSubstring("controller-manager"))

				controllerPodName = podName

				cmd := exec.Command("kubectl", "get", "pod", controllerPodName, "-n", namespace,
					"-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"))
			}).Should(Succeed())
		})

		It("should expose metrics", func() {
			By("ensuring the metrics service exists")
			cmd := exec.Command("kubectl", "get", "service", metricsService, "-n", namespace)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Metrics service should exist")

			By("creating a curl pod to access the metrics endpoint")
			cmd = exec.Command("kubectl", "run", "curl-metrics", "--restart=Never",
				"--namespace", namespace,
				"--image=curlimages/curl:latest",
				"--overrides",
				fmt.Sprintf(`{
					"spec": {
						"containers": [{
							"name": "curl",
							"image": "curlimages/curl:latest",
							"command": ["/bin/sh", "-c"],
							"args": ["curl -sS http://%s.%s.svc.cluster.local:8080/metrics"],
							"securityContext": {
								"readOnlyRootFilesystem": true,
								"allowPrivilegeEscalation": false,
								"capabilities": {
									"drop": ["ALL"]
								},
								"runAsNonRoot": true,
								"runAsUser": 1000,
								"seccompProfile": {
									"type": "RuntimeDefault"
								}
							}
						}]
					}
				}`, metricsService, namespace))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create curl-metrics pod")

			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pod", "curl-metrics", "-n", namespace,
					"-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Succeeded"))
			}, 5*time.Minute, time.Second).Should(Succeed())

			Eventually(func(g Gomega) {
				output, err := metricsOutput()
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("go_gc_duration_seconds"))
			}, 2*time.Minute, time.Second).Should(Succeed())
		})

		It("should reconcile only matching LoadBalancer Services", func() {
			const manifest = `
apiVersion: v1
kind: Service
metadata:
  name: demo-matching
  namespace: k8s-lb-controller-e2e
spec:
  type: LoadBalancer
  loadBalancerClass: iedge.local/service-lb
  selector:
    app: demo
  ports:
    - port: 80
      targetPort: 8080
---
apiVersion: v1
kind: Service
metadata:
  name: demo-ignored
  namespace: k8s-lb-controller-e2e
spec:
  type: LoadBalancer
  loadBalancerClass: diploma.local/other
  selector:
    app: demo
  ports:
    - port: 81
      targetPort: 8081
`

			By("applying Service manifests")
			path, err := writeTempManifest("service-e2e", manifest)
			Expect(err).NotTo(HaveOccurred(), "Failed to write test manifest")
			defer os.Remove(path)

			cmd := exec.Command("kubectl", "apply", "-f", path)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to apply Service manifest")

			Eventually(func(g Gomega) {
				logs, err := controllerLogs(controllerPodName)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(logs).To(ContainSubstring("service matched controller selection"))
				g.Expect(logs).To(ContainSubstring("demo-matching"))
				g.Expect(logs).To(ContainSubstring("iedge.local/service-lb"))
			}, 2*time.Minute, time.Second).Should(Succeed())

			Consistently(func(g Gomega) {
				logs, err := controllerLogs(controllerPodName)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(strings.Contains(logs, "demo-ignored") &&
					strings.Contains(logs, "service matched controller selection")).To(BeFalse())
			}, 15*time.Second, time.Second).Should(Succeed())
		})
	})
})

func activeControllerPod() (string, error) {
	cmd := exec.Command("kubectl", "get", "pods",
		"-l", "control-plane=controller-manager",
		"-o", "go-template={{ range .items }}{{ if not .metadata.deletionTimestamp }}{{ .metadata.name }}{{ \"\\n\" }}{{ end }}{{ end }}",
		"-n", namespace,
	)

	output, err := utils.Run(cmd)
	if err != nil {
		return "", err
	}

	podNames := utils.GetNonEmptyLines(output)
	if len(podNames) != 1 {
		return "", fmt.Errorf("expected one controller pod, got %d", len(podNames))
	}

	return podNames[0], nil
}

func controllerLogs(podName string) (string, error) {
	cmd := exec.Command("kubectl", "logs", podName, "-n", namespace)
	return utils.Run(cmd)
}

func metricsOutput() (string, error) {
	cmd := exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
	return utils.Run(cmd)
}

func writeTempManifest(prefix, content string) (string, error) {
	path := filepath.Join(os.TempDir(), fmt.Sprintf("%s.yaml", prefix))
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)), 0o600); err != nil {
		return "", err
	}

	return path, nil
}
