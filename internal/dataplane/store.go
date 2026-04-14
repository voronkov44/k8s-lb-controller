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

package dataplane

import (
	"sort"

	"github.com/voronkov44/k8s-lb-controller/internal/provider"
)

// Store keeps desired Service state in memory.
type Store struct {
	services map[provider.ServiceRef]provider.Service
}

// NewStore creates an empty Store.
func NewStore() *Store {
	return &Store{
		services: make(map[provider.ServiceRef]provider.Service),
	}
}

// Clone returns a detached copy of the current Store.
func (s *Store) Clone() *Store {
	cloned := NewStore()
	for ref, service := range s.services {
		cloned.services[ref] = normalizeService(service)
	}

	return cloned
}

// Upsert stores a Service in canonical form.
func (s *Store) Upsert(service provider.Service) {
	normalized := normalizeService(service)
	s.services[normalized.Ref()] = normalized
}

// Delete removes a Service when it exists.
func (s *Store) Delete(ref provider.ServiceRef) bool {
	if _, ok := s.services[ref]; !ok {
		return false
	}

	delete(s.services, ref)
	return true
}

// Get returns a detached copy of the stored Service when present.
func (s *Store) Get(ref provider.ServiceRef) (provider.Service, bool) {
	service, ok := s.services[ref]
	if !ok {
		return provider.Service{}, false
	}

	return service.DeepCopy(), true
}

// List returns all stored Services sorted by namespace and name.
func (s *Store) List() []provider.Service {
	return sortedServiceListFromMap(s.services)
}

// Snapshot returns a detached copy of the current store contents.
func (s *Store) Snapshot() map[provider.ServiceRef]provider.Service {
	snapshot := make(map[provider.ServiceRef]provider.Service, len(s.services))
	for ref, service := range s.services {
		snapshot[ref] = service.DeepCopy()
	}

	return snapshot
}

// Len returns the number of stored Services.
func (s *Store) Len() int {
	return len(s.services)
}

func sortedServiceListFromMap(services map[provider.ServiceRef]provider.Service) []provider.Service {
	list := make([]provider.Service, 0, len(services))
	for _, service := range services {
		list = append(list, normalizeService(service))
	}

	sort.Slice(list, func(i, j int) bool {
		if list[i].Namespace == list[j].Namespace {
			return list[i].Name < list[j].Name
		}

		return list[i].Namespace < list[j].Namespace
	})

	return list
}

func normalizeService(service provider.Service) provider.Service {
	normalized := service.DeepCopy()

	sort.Slice(normalized.Ports, func(i, j int) bool {
		if normalized.Ports[i].Port != normalized.Ports[j].Port {
			return normalized.Ports[i].Port < normalized.Ports[j].Port
		}
		if normalized.Ports[i].Protocol != normalized.Ports[j].Protocol {
			return normalized.Ports[i].Protocol < normalized.Ports[j].Protocol
		}
		if normalized.Ports[i].Name != normalized.Ports[j].Name {
			return normalized.Ports[i].Name < normalized.Ports[j].Name
		}

		return normalized.Ports[i].TargetPort < normalized.Ports[j].TargetPort
	})

	for index := range normalized.Ports {
		sort.Slice(normalized.Ports[index].Backends, func(i, j int) bool {
			if normalized.Ports[index].Backends[i].Address == normalized.Ports[index].Backends[j].Address {
				return normalized.Ports[index].Backends[i].Port < normalized.Ports[index].Backends[j].Port
			}

			return normalized.Ports[index].Backends[i].Address < normalized.Ports[index].Backends[j].Address
		})
	}

	return normalized
}
