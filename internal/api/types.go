package api

import "time"

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
