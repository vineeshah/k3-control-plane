package store

import (
	"sort"

	"k8/internal/api"
)

func (s *MemoryStore) UpsertJob(job api.Job) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job.Name] = job.Clone()
}

func (s *MemoryStore) UpdateJobStatus(name string, status api.JobStatus) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.jobs[name]
	if !ok {
		return false
	}
	job.Status = status
	s.jobs[name] = job
	return true
}

func (s *MemoryStore) ListJobs() []api.Job {
	s.mu.RLock()
	defer s.mu.RUnlock()

	jobs := make([]api.Job, 0, len(s.jobs))
	for _, job := range s.jobs {
		jobs = append(jobs, job.Clone())
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].Name < jobs[j].Name })
	return jobs
}

func (s *MemoryStore) DeleteJob(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.jobs[name]; !ok {
		return false
	}
	delete(s.jobs, name)
	for id, assignment := range s.assignments {
		if assignment.OwnerKind == api.WorkloadKindJob && assignment.OwnerName == name {
			delete(s.assignments, id)
		}
	}
	return true
}
