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

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDiagnoseLocalKubeconfigDetectsMissingCurrentContext(t *testing.T) {
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "config"))

	kubeconfigPath := os.Getenv("KUBECONFIG")
	kubeconfig := `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://example.invalid
  name: cluster1
contexts:
- context:
    cluster: cluster1
    user: user1
  name: kind-k8s-lb-controller
- context:
    cluster: cluster1
    user: user1
  name: minikube
current-context: ""
users:
- name: user1
  user:
    token: test
`
	if err := os.WriteFile(kubeconfigPath, []byte(kubeconfig), 0o600); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}

	diagnostic, err := diagnoseLocalKubeconfig(nil)
	if err != nil {
		t.Fatalf("diagnose kubeconfig failure: %v", err)
	}
	if diagnostic == nil {
		t.Fatal("expected diagnostic, got nil")
	}
	if diagnostic.kind != kubeconfigDiagnosticMissingCurrentContext {
		t.Fatalf("expected missing current-context diagnostic, got %q", diagnostic.kind)
	}
	if diagnostic.kubeconfigPath != kubeconfigPath {
		t.Fatalf("expected kubeconfig path %q, got %q", kubeconfigPath, diagnostic.kubeconfigPath)
	}

	wantContexts := []string{"kind-k8s-lb-controller", "minikube"}
	if !reflect.DeepEqual(diagnostic.availableContexts, wantContexts) {
		t.Fatalf("expected contexts %v, got %v", wantContexts, diagnostic.availableContexts)
	}

	wantCommand := "kubectl config use-context kind-k8s-lb-controller"
	if diagnostic.suggestedCommand != wantCommand {
		t.Fatalf("expected suggested command %q, got %q", wantCommand, diagnostic.suggestedCommand)
	}
}

func TestDiagnoseLocalKubeconfigDetectsMissingKubeconfig(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "missing-config")
	t.Setenv("KUBECONFIG", missingPath)

	diagnostic, err := diagnoseLocalKubeconfig(nil)
	if err != nil {
		t.Fatalf("diagnose kubeconfig failure: %v", err)
	}
	if diagnostic == nil {
		t.Fatal("expected diagnostic, got nil")
	}
	if diagnostic.kind != kubeconfigDiagnosticNoKubeconfig {
		t.Fatalf("expected no kubeconfig diagnostic, got %q", diagnostic.kind)
	}

	wantSources := []string{missingPath}
	if !reflect.DeepEqual(diagnostic.checkedSources, wantSources) {
		t.Fatalf("expected checked sources %v, got %v", wantSources, diagnostic.checkedSources)
	}
}
