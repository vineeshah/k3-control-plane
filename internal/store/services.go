package store

import (
	"sort"

	"k8/internal/api"
)

func (s *MemoryStore) UpsertService(service api.Service) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.services[service.Name] = service.Clone()
}

func (s *MemoryStore) UpdateServiceStatus(name string, status api.ServiceStatus) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	service, ok := s.services[name]
	if !ok {
		return false
	}
	service.Status = status
	s.services[name] = service
	return true
}

func (s *MemoryStore) ListServices() []api.Service {
	s.mu.RLock()
	defer s.mu.RUnlock()

	services := make([]api.Service, 0, len(s.services))
	for _, service := range s.services {
		services = append(services, service.Clone())
	}
	sort.Slice(services, func(i, j int) bool { return services[i].Name < services[j].Name })
	return services
}

func (s *MemoryStore) DeleteService(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.services[name]; !ok {
		return false
	}
	delete(s.services, name)
	for id, assignment := range s.assignments {
		if assignment.OwnerKind == api.WorkloadKindService && assignment.OwnerName == name {
			delete(s.assignments, id)
		}
	}
	return true
}
