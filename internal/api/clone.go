package api

import "time"

func (n Node) Clone() Node {
	return Node{
		ID:            n.ID,
		Labels:        cloneStringMap(n.Labels),
		Capacity:      n.Capacity,
		LastHeartbeat: n.LastHeartbeat,
	}
}

func (s Service) Clone() Service {
	return Service{
		Name:      s.Name,
		Image:     s.Image,
		Command:   cloneStringSlice(s.Command),
		Env:       cloneStringMap(s.Env),
		Replicas:  s.Replicas,
		Resources: s.Resources,
		Placement: Placement{RequiredLabels: cloneStringMap(s.Placement.RequiredLabels)},
		Volumes:   cloneVolumes(s.Volumes),
		Ports:     cloneInts(s.Ports),
		Status: ServiceStatus{
			DesiredReplicas:    s.Status.DesiredReplicas,
			PendingReplicas:    s.Status.PendingReplicas,
			RunningReplicas:    s.Status.RunningReplicas,
			AssignmentIDs:      cloneStringSlice(s.Status.AssignmentIDs),
			LastReconciledUnix: s.Status.LastReconciledUnix,
		},
	}
}

func (j Job) Clone() Job {
	return Job{
		Name:      j.Name,
		Image:     j.Image,
		Command:   cloneStringSlice(j.Command),
		Env:       cloneStringMap(j.Env),
		Resources: j.Resources,
		Placement: Placement{RequiredLabels: cloneStringMap(j.Placement.RequiredLabels)},
		Volumes:   cloneVolumes(j.Volumes),
		Retries:   j.Retries,
		Status: JobStatus{
			ActiveAssignmentID: j.Status.ActiveAssignmentID,
			LastAssignmentID:   j.Status.LastAssignmentID,
			Succeeded:          j.Status.Succeeded,
			FailedAttempts:     j.Status.FailedAttempts,
			LastFailure:        j.Status.LastFailure,
			CompletedAt:        cloneTimePtr(j.Status.CompletedAt),
		},
	}
}

func (a Assignment) Clone() Assignment {
	return Assignment{
		ID:            a.ID,
		OwnerKind:     a.OwnerKind,
		OwnerName:     a.OwnerName,
		NodeID:        a.NodeID,
		Image:         a.Image,
		Command:       cloneStringSlice(a.Command),
		Env:           cloneStringMap(a.Env),
		Resources:     a.Resources,
		Volumes:       cloneVolumes(a.Volumes),
		Ports:         cloneInts(a.Ports),
		Attempt:       a.Attempt,
		Phase:         a.Phase,
		StatusMessage: a.StatusMessage,
		CreatedAt:     a.CreatedAt,
		StartedAt:     cloneTimePtr(a.StartedAt),
		FinishedAt:    cloneTimePtr(a.FinishedAt),
	}
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneStringSlice(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func cloneInts(in []int) []int {
	if len(in) == 0 {
		return nil
	}
	out := make([]int, len(in))
	copy(out, in)
	return out
}

func cloneVolumes(in []Volume) []Volume {
	if len(in) == 0 {
		return nil
	}
	out := make([]Volume, len(in))
	copy(out, in)
	return out
}

func cloneTimePtr(in *time.Time) *time.Time {
	if in == nil {
		return nil
	}
	value := *in
	return &value
}
