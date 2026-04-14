package api

import "time"

type ResourceRequirements struct {
	CPU    int `json:"cpu"`
	Memory int `json:"memory"`
}

type Placement struct {
	RequiredLabels map[string]string `json:"requiredLabels,omitempty"`
}

type Volume struct {
	Name      string `json:"name"`
	HostPath  string `json:"hostPath"`
	MountPath string `json:"mountPath"`
}

type Node struct {
	ID            string               `json:"id"`
	Labels        map[string]string    `json:"labels,omitempty"`
	Capacity      ResourceRequirements `json:"capacity"`
	LastHeartbeat time.Time            `json:"lastHeartbeat"`
}

type Service struct {
	Name      string               `json:"name"`
	Image     string               `json:"image"`
	Command   []string             `json:"command,omitempty"`
	Env       map[string]string    `json:"env,omitempty"`
	Replicas  int                  `json:"replicas"`
	Resources ResourceRequirements `json:"resources"`
	Placement Placement            `json:"placement,omitempty"`
	Volumes   []Volume             `json:"volumes,omitempty"`
	Status    ServiceStatus        `json:"status"`
}

type ServiceStatus struct {
	DesiredReplicas    int      `json:"desiredReplicas"`
	PendingReplicas    int      `json:"pendingReplicas"`
	RunningReplicas    int      `json:"runningReplicas"`
	AssignmentIDs      []string `json:"assignmentIds,omitempty"`
	LastReconciledUnix int64    `json:"lastReconciledUnix"`
}

type Job struct {
	Name      string               `json:"name"`
	Image     string               `json:"image"`
	Command   []string             `json:"command,omitempty"`
	Env       map[string]string    `json:"env,omitempty"`
	Resources ResourceRequirements `json:"resources"`
	Placement Placement            `json:"placement,omitempty"`
	Volumes   []Volume             `json:"volumes,omitempty"`
	Retries   int                  `json:"retries"`
	Status    JobStatus            `json:"status"`
}

type JobStatus struct {
	ActiveAssignmentID string     `json:"activeAssignmentId,omitempty"`
	LastAssignmentID   string     `json:"lastAssignmentId,omitempty"`
	Succeeded          bool       `json:"succeeded"`
	FailedAttempts     int        `json:"failedAttempts"`
	LastFailure        string     `json:"lastFailure,omitempty"`
	CompletedAt        *time.Time `json:"completedAt,omitempty"`
}

type WorkloadKind string

const (
	WorkloadKindService WorkloadKind = "service"
	WorkloadKindJob     WorkloadKind = "job"
)

type AssignmentPhase string

const (
	AssignmentPhasePending   AssignmentPhase = "Pending"
	AssignmentPhaseRunning   AssignmentPhase = "Running"
	AssignmentPhaseSucceeded AssignmentPhase = "Succeeded"
	AssignmentPhaseFailed    AssignmentPhase = "Failed"
	AssignmentPhaseStopped   AssignmentPhase = "Stopped"
)

type Assignment struct {
	ID            string               `json:"id"`
	OwnerKind     WorkloadKind         `json:"ownerKind"`
	OwnerName     string               `json:"ownerName"`
	NodeID        string               `json:"nodeId"`
	Image         string               `json:"image"`
	Command       []string             `json:"command,omitempty"`
	Env           map[string]string    `json:"env,omitempty"`
	Resources     ResourceRequirements `json:"resources"`
	Volumes       []Volume             `json:"volumes,omitempty"`
	Attempt       int                  `json:"attempt"`
	Phase         AssignmentPhase      `json:"phase"`
	StatusMessage string               `json:"statusMessage,omitempty"`
	CreatedAt     time.Time            `json:"createdAt"`
	StartedAt     *time.Time           `json:"startedAt,omitempty"`
	FinishedAt    *time.Time           `json:"finishedAt,omitempty"`
}

type StateSnapshot struct {
	Nodes       []Node       `json:"nodes"`
	Services    []Service    `json:"services"`
	Jobs        []Job        `json:"jobs"`
	Assignments []Assignment `json:"assignments"`
}

func IsTerminalPhase(phase AssignmentPhase) bool {
	switch phase {
	case AssignmentPhaseSucceeded, AssignmentPhaseFailed, AssignmentPhaseStopped:
		return true
	default:
		return false
	}
}

func IsActivePhase(phase AssignmentPhase) bool {
	return phase == AssignmentPhasePending || phase == AssignmentPhaseRunning
}

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
