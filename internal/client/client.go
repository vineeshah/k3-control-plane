package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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

func (c *Client) ApplyJob(job api.Job) error {
	return c.postJSON("/jobs", job, nil)
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

func (c *Client) postJSON(path string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return decodeHTTPError(resp)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (c *Client) getJSON(path string, out any) error {
	resp, err := c.httpClient.Get(c.baseURL + path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return decodeHTTPError(resp)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func decodeHTTPError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
}
