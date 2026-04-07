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

package metrics

import (
	"context"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/voronkov44/k8s-lb-controller/internal/provider"
)

const metricNamespace = "k8s_lb_controller"

const (
	allocationResultAllocated = "allocated"
	allocationResultReused    = "reused"
	allocationResultExhausted = "exhausted"
	allocationResultError     = "error"
)

var (
	registerOnce sync.Once

	serviceReconcileTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: metricNamespace,
		Name:      "service_reconcile_total",
		Help:      "Total number of Service reconcile attempts handled by the controller.",
	})
	serviceReconcileErrorsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: metricNamespace,
		Name:      "service_reconcile_errors_total",
		Help:      "Total number of Service reconcile attempts that returned an error.",
	})
	serviceReconcileDurationSeconds = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: metricNamespace,
		Name:      "service_reconcile_duration_seconds",
		Help:      "Duration of Service reconcile attempts.",
		Buckets:   prometheus.DefBuckets,
	})
	ipAllocationsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: metricNamespace,
		Name:      "ip_allocations_total",
		Help:      "Total number of IP allocation decisions grouped by result.",
	}, []string{"result"})
	providerOperationsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: metricNamespace,
		Name:      "provider_operations_total",
		Help:      "Total number of provider operations grouped by operation and result.",
	}, []string{"operation", "result"})
	providerManagedServices = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: metricNamespace,
		Name:      "provider_managed_services",
		Help:      "Current number of managed Services stored in the provider state.",
	})
)

// Register adds the custom controller metrics to the controller-runtime registry once.
func Register() {
	registerOnce.Do(func() {
		ctrlmetrics.Registry.MustRegister(
			serviceReconcileTotal,
			serviceReconcileErrorsTotal,
			serviceReconcileDurationSeconds,
			ipAllocationsTotal,
			providerOperationsTotal,
			providerManagedServices,
		)
	})
}

// IncServiceReconcile records the start of a Service reconcile attempt.
func IncServiceReconcile() {
	Register()
	serviceReconcileTotal.Inc()
}

// ObserveServiceReconcile records the duration and error outcome of a Service reconcile attempt.
func ObserveServiceReconcile(duration time.Duration, err error) {
	Register()
	serviceReconcileDurationSeconds.Observe(duration.Seconds())
	if err != nil {
		serviceReconcileErrorsTotal.Inc()
	}
}

// RecordIPAllocationAllocated records a newly assigned IP allocation.
func RecordIPAllocationAllocated() {
	recordIPAllocation(allocationResultAllocated)
}

// RecordIPAllocationReused records a reused IP allocation.
func RecordIPAllocationReused() {
	recordIPAllocation(allocationResultReused)
}

// RecordIPAllocationExhausted records a pool exhaustion outcome.
func RecordIPAllocationExhausted() {
	recordIPAllocation(allocationResultExhausted)
}

// RecordIPAllocationError records a non-exhaustion allocation failure.
func RecordIPAllocationError() {
	recordIPAllocation(allocationResultError)
}

func recordIPAllocation(result string) {
	Register()
	ipAllocationsTotal.WithLabelValues(result).Inc()
}

// WrapProvider instruments provider operations without changing the provider interface.
func WrapProvider(next provider.Provider) provider.Provider {
	Register()
	if next == nil {
		return nil
	}

	return &instrumentedProvider{
		next: next,
		refs: make(map[provider.ServiceRef]struct{}),
	}
}

type instrumentedProvider struct {
	next provider.Provider

	mu   sync.Mutex
	refs map[provider.ServiceRef]struct{}
}

func (p *instrumentedProvider) Ensure(ctx context.Context, service provider.Service) (bool, error) {
	changed, err := p.next.Ensure(ctx, service)
	if err != nil {
		recordProviderOperation("ensure", "error")
		return false, err
	}

	recordProviderOperation("ensure", "success")
	p.trackEnsure(service.Ref())
	return changed, nil
}

func (p *instrumentedProvider) Delete(ctx context.Context, ref provider.ServiceRef) (bool, error) {
	changed, err := p.next.Delete(ctx, ref)
	if err != nil {
		recordProviderOperation("delete", "error")
		return false, err
	}

	recordProviderOperation("delete", "success")
	p.trackDelete(ref)
	return changed, nil
}

func (p *instrumentedProvider) trackEnsure(ref provider.ServiceRef) {
	p.mu.Lock()
	p.refs[ref] = struct{}{}
	count := len(p.refs)
	p.mu.Unlock()

	providerManagedServices.Set(float64(count))
}

func (p *instrumentedProvider) trackDelete(ref provider.ServiceRef) {
	p.mu.Lock()
	delete(p.refs, ref)
	count := len(p.refs)
	p.mu.Unlock()

	providerManagedServices.Set(float64(count))
}

func recordProviderOperation(operation, result string) {
	Register()
	providerOperationsTotal.WithLabelValues(operation, result).Inc()
}

var _ provider.Provider = (*instrumentedProvider)(nil)

func init() {
	Register()
}
