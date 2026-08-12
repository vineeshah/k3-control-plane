package client

import (
	"net/http"
	"strings"
	"time"

	"k8/internal/api"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (c *Client) ApplyService(service api.Service) error {
	return c.postJSON("/services", service, nil)
}

func (c *Client) DeleteService(name string) error {
	return c.do(http.MethodDelete, "/services/"+name, nil, nil)
}

func (c *Client) ApplyJob(job api.Job) error {
	return c.postJSON("/jobs", job, nil)
}

func (c *Client) DeleteJob(name string) error {
	return c.do(http.MethodDelete, "/jobs/"+name, nil, nil)
}

func (c *Client) RegisterNode(node api.Node) error {
	return c.postJSON("/nodes/register", node, nil)
}

func (c *Client) Heartbeat(nodeID string) error {
	payload := struct {
		ID string `json:"id"`
	}{ID: nodeID}
	return c.postJSON("/nodes/heartbeat", payload, nil)
}

func (c *Client) FetchAssignments(nodeID string) ([]api.Assignment, error) {
	var assignments []api.Assignment
	if err := c.getJSON("/nodes/"+nodeID+"/assignments", &assignments); err != nil {
		return nil, err
	}
	return assignments, nil
}

func (c *Client) UpdateAssignmentStatus(id string, phase api.AssignmentPhase, message string) error {
	payload := struct {
		Phase   api.AssignmentPhase `json:"phase"`
		Message string              `json:"message"`
	}{
		Phase:   phase,
		Message: message,
	}
	return c.postJSON("/assignments/"+id+"/status", payload, nil)
}

func (c *Client) FetchState() (api.StateSnapshot, error) {
	var snapshot api.StateSnapshot
	err := c.getJSON("/state", &snapshot)
	return snapshot, err
}
