package controller

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/f1lzz/k8s-lb-controller/internal/config"
)

func TestServiceReconcilerReconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	requeueAfter := 30 * time.Second
	class := "iedge.local/service-lb"

	tests := []struct {
		name       string
		objects    []runtime.Object
		request    types.NamespacedName
		wantResult ctrl.Result
	}{
		{
			name: "returns empty result when service is not found",
			request: types.NamespacedName{
				Namespace: "default",
				Name:      "missing",
			},
			wantResult: ctrl.Result{},
		},
		{
			name: "ignores non matching service",
			objects: []runtime.Object{
				newReconcileService("ignored", "default", corev1.ServiceTypeClusterIP, nil),
			},
			request: types.NamespacedName{
				Namespace: "default",
				Name:      "ignored",
			},
			wantResult: ctrl.Result{},
		},
		{
			name: "requeues matching service",
			objects: []runtime.Object{
				newReconcileService("demo", "default", corev1.ServiceTypeLoadBalancer, &class),
			},
			request: types.NamespacedName{
				Namespace: "default",
				Name:      "demo",
			},
			wantResult: ctrl.Result{RequeueAfter: requeueAfter},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientBuilder := fake.NewClientBuilder().WithScheme(scheme)
			if len(tt.objects) > 0 {
				clientBuilder = clientBuilder.WithRuntimeObjects(tt.objects...)
			}

			reconciler := &ServiceReconciler{
				Client: clientBuilder.Build(),
				Scheme: scheme,
				Config: config.Config{
					LoadBalancerClass: class,
					RequeueAfter:      requeueAfter,
				},
			}

			result, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: tt.request})
			if err != nil {
				t.Fatalf("Reconcile() error = %v", err)
			}

			if result != tt.wantResult {
				t.Fatalf("Reconcile() result = %+v, want %+v", result, tt.wantResult)
			}
		})
	}
}

func newReconcileService(name, namespace string, serviceType corev1.ServiceType, loadBalancerClass *string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Type:              serviceType,
			LoadBalancerClass: loadBalancerClass,
		},
	}
}
