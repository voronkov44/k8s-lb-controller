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
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
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
	namespace         = "k8s-lb-controller-system"
	serviceNamespace  = "k8s-lb-controller-e2e"
	metricsService    = "k8s-lb-controller-controller-manager-metrics-service"
	defaultExternalIP = "203.0.113.10"
	serviceFinalizer  = "iedge.local/service-lb-finalizer"
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
		By("undeploying the controller-manager")
		cmd := exec.Command("make", "undeploy")
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

			By("forwarding the metrics service locally")
			portForward, err := startMetricsPortForward()
			Expect(err).NotTo(HaveOccurred(), "Failed to start metrics port-forward")
			defer func() {
				Expect(portForward.Stop()).To(Succeed())
			}()

			Eventually(func(g Gomega) {
				output, err := metricsOutput(portForward.LocalPort)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("go_gc_duration_seconds"))
				g.Expect(output).To(ContainSubstring("k8s_lb_controller_service_reconcile_total"))
				g.Expect(output).To(ContainSubstring("k8s_lb_controller_service_reconcile_errors_total"))
				g.Expect(output).To(ContainSubstring("k8s_lb_controller_service_reconcile_duration_seconds"))
			}, 2*time.Minute, time.Second).Should(Succeed())
		})

		It("should assign an external IP and sync provider state for matching LoadBalancer Services", func() {
			const manifest = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo
  namespace: k8s-lb-controller-e2e
spec:
  replicas: 1
  selector:
    matchLabels:
      app: demo
  template:
    metadata:
      labels:
        app: demo
    spec:
      containers:
        - name: nginx
          image: nginx:stable
          ports:
            - containerPort: 80
---
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
      targetPort: 80
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
      targetPort: 80
`

			By("applying Service manifests")
			path, err := writeTempManifest("service-e2e", manifest)
			Expect(err).NotTo(HaveOccurred(), "Failed to write test manifest")
			defer os.Remove(path)

			cmd := exec.Command("kubectl", "apply", "-f", path)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to apply Service manifest")

			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "deployment", "demo", "-n", serviceNamespace,
					"-o", "jsonpath={.status.readyReplicas}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("1"))
			}, 2*time.Minute, time.Second).Should(Succeed())

			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "service", "demo-matching", "-n", serviceNamespace,
					"-o", "jsonpath={.status.loadBalancer.ingress[0].ip}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal(defaultExternalIP))
			}, 2*time.Minute, time.Second).Should(Succeed())

			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "service", "demo-matching", "-n", serviceNamespace,
					"-o", "jsonpath={.metadata.finalizers}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring(serviceFinalizer))
			}, 2*time.Minute, time.Second).Should(Succeed())

			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "endpointslice", "-n", serviceNamespace,
					"-l", "kubernetes.io/service-name=demo-matching",
					"-o", "jsonpath={.items[*].metadata.name}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(strings.TrimSpace(output)).NotTo(BeEmpty())
			}, 2*time.Minute, time.Second).Should(Succeed())

			Consistently(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "service", "demo-ignored", "-n", serviceNamespace,
					"-o", "jsonpath={.metadata.finalizers}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(BeEmpty())
			}, 15*time.Second, time.Second).Should(Succeed())

			Consistently(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "service", "demo-ignored", "-n", serviceNamespace,
					"-o", "jsonpath={.status.loadBalancer.ingress[0].ip}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(BeEmpty())
			}, 15*time.Second, time.Second).Should(Succeed())

			Eventually(func(g Gomega) {
				logs, err := controllerLogs(controllerPodName)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(logs).To(ContainSubstring("service matched controller selection"))
				g.Expect(logs).To(ContainSubstring("demo-matching"))
				g.Expect(logs).To(ContainSubstring("iedge.local/service-lb"))
				g.Expect(logs).To(ContainSubstring("assigned external IP"))
				g.Expect(logs).To(ContainSubstring(defaultExternalIP))
				g.Expect(logs).To(ContainSubstring("added service finalizer"))
				g.Expect(logs).To(ContainSubstring("ensured provider state"))
				g.Expect(logs).To(ContainSubstring("\"backendCount\":1"))
			}, 2*time.Minute, time.Second).Should(Succeed())

			By("verifying activity-dependent custom metrics after reconcile work has happened")
			portForward, err := startMetricsPortForward()
			Expect(err).NotTo(HaveOccurred(), "Failed to start metrics port-forward")
			defer func() {
				Expect(portForward.Stop()).To(Succeed())
			}()

			Eventually(func(g Gomega) {
				output, err := metricsOutput(portForward.LocalPort)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("k8s_lb_controller_ip_allocations_total"))
				g.Expect(output).To(ContainSubstring("k8s_lb_controller_provider_operations_total"))
				g.Expect(output).To(ContainSubstring("k8s_lb_controller_provider_managed_services"))
			}, 2*time.Minute, time.Second).Should(Succeed())

			Consistently(func(g Gomega) {
				logs, err := controllerLogs(controllerPodName)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(strings.Contains(logs, "demo-ignored") &&
					strings.Contains(logs, "service matched controller selection")).To(BeFalse())
			}, 15*time.Second, time.Second).Should(Succeed())

			By("scaling the demo deployment to trigger EndpointSlice reconciliation")
			cmd = exec.Command("kubectl", "scale", "deployment", "demo", "-n", serviceNamespace, "--replicas=2")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to scale demo Deployment")

			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "deployment", "demo", "-n", serviceNamespace,
					"-o", "jsonpath={.status.readyReplicas}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("2"))
			}, 2*time.Minute, time.Second).Should(Succeed())

			Eventually(func(g Gomega) {
				logs, err := controllerLogs(controllerPodName)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(logs).To(ContainSubstring("\"backendCount\":2"))
			}, 2*time.Minute, time.Second).Should(Succeed())

			By("deleting the managed Service")
			cmd = exec.Command("kubectl", "delete", "service", "demo-matching", "-n", serviceNamespace, "--wait=false")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to delete demo-matching Service")

			By("waiting for the Service to disappear instead of hanging in Terminating")
			cmd = exec.Command("kubectl", "wait", "--for=delete", "service/demo-matching",
				"-n", serviceNamespace, "--timeout=120s")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Managed Service should be deleted cleanly")

			Eventually(func(g Gomega) {
				logs, err := controllerLogs(controllerPodName)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(logs).To(ContainSubstring("cleaned up provider state"))
			}, 2*time.Minute, time.Second).Should(Succeed())
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

type portForwardSession struct {
	LocalPort int

	cancel context.CancelFunc
	cmd    *exec.Cmd
	stdout bytes.Buffer
	stderr bytes.Buffer
	done   chan error
}

func startMetricsPortForward() (*portForwardSession, error) {
	localPort, err := freeLocalPort()
	if err != nil {
		return nil, fmt.Errorf("allocate local port: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(
		ctx,
		"kubectl",
		"port-forward",
		"-n", namespace,
		"--address", "127.0.0.1",
		"service/"+metricsService,
		fmt.Sprintf("%d:8080", localPort),
	)

	session := &portForwardSession{
		LocalPort: localPort,
		cancel:    cancel,
		cmd:       cmd,
		done:      make(chan error, 1),
	}
	cmd.Stdout = &session.stdout
	cmd.Stderr = &session.stderr

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start port-forward: %w", err)
	}

	go func() {
		session.done <- cmd.Wait()
	}()

	client := &http.Client{Timeout: 2 * time.Second}
	metricsURL := fmt.Sprintf("http://127.0.0.1:%d/metrics", localPort)
	deadline := time.Now().Add(30 * time.Second)

	for time.Now().Before(deadline) {
		response, requestErr := client.Get(metricsURL)
		if requestErr == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return session, nil
			}
		}

		select {
		case waitErr := <-session.done:
			cancel()
			return nil, fmt.Errorf("port-forward exited early: %w: %s", waitErr, session.output())
		default:
		}

		time.Sleep(200 * time.Millisecond)
	}

	_ = session.Stop()
	return nil, fmt.Errorf("timed out waiting for metrics port-forward readiness: %s", session.output())
}

func (s *portForwardSession) Stop() error {
	if s == nil {
		return nil
	}

	s.cancel()

	select {
	case err := <-s.done:
		if err == nil || strings.Contains(err.Error(), "signal: killed") {
			return nil
		}
		return fmt.Errorf("wait for port-forward shutdown: %w: %s", err, s.output())
	case <-time.After(5 * time.Second):
		if s.cmd != nil && s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
		}

		select {
		case err := <-s.done:
			if err == nil || strings.Contains(err.Error(), "signal: killed") {
				return nil
			}
			return fmt.Errorf("force-stop port-forward: %w: %s", err, s.output())
		case <-time.After(2 * time.Second):
			return fmt.Errorf("timed out stopping metrics port-forward: %s", s.output())
		}
	}
}

func (s *portForwardSession) output() string {
	if s == nil {
		return ""
	}

	return strings.TrimSpace(s.stdout.String() + "\n" + s.stderr.String())
}

func metricsOutput(localPort int) (string, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/metrics", localPort))
	if err != nil {
		return "", err
	}
	defer func() {
		_ = response.Body.Close()
	}()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected metrics status code %d", response.StatusCode)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

func freeLocalPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = listener.Close()
	}()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("unexpected listener address type %T", listener.Addr())
	}

	return addr.Port, nil
}

func writeTempManifest(prefix, content string) (string, error) {
	path := filepath.Join(os.TempDir(), fmt.Sprintf("%s.yaml", prefix))
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)), 0o600); err != nil {
		return "", err
	}

	return path, nil
}
