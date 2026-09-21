package gpu

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type RequestState string

const (
	RequestStatePending   RequestState = "Pending"
	RequestStateAdmitted  RequestState = "Admitted"
	RequestStateRunning   RequestState = "Running"
	RequestStateCompleted RequestState = "Completed"
	RequestStateCancelled RequestState = "Cancelled"
	RequestStateFailed    RequestState = "Failed"
)

type GPURequest struct {
	ID        string       `json:"id"`
	Tenant    string       `json:"tenant"`
	GPUType   string       `json:"gpuType"`
	Quantity  int          `json:"quantity"`
	State     RequestState `json:"state"`
	Cluster   string       `json:"cluster,omitempty"`
	Reason    string       `json:"reason,omitempty"`
	CreatedAt time.Time    `json:"createdAt"`
	UpdatedAt time.Time    `json:"updatedAt"`
}

type AdmissionResult struct {
	Admitted bool         `json:"admitted"`
	Cluster  string       `json:"cluster,omitempty"`
	Reason   string       `json:"reason,omitempty"`
	State    RequestState `json:"state"`
}

var (
	ErrRequestNotFound    = errors.New("request not found")
	ErrDuplicateRequest   = errors.New("request with this ID already exists")
	ErrRequestTerminal    = errors.New("request is in a terminal state")
	ErrRequestCancelled   = errors.New("request has been cancelled")
)

func (m *Manager) SubmitRequest(ctx context.Context, req GPURequest) (*AdmissionResult, error) {
	if req.ID == "" {
		return nil, fmt.Errorf("request ID is required")
	}
	if req.Tenant == "" {
		return nil, fmt.Errorf("tenant is required")
	}
	if req.GPUType == "" {
		return nil, fmt.Errorf("GPU type is required")
	}
	if req.Quantity <= 0 {
		return nil, fmt.Errorf("quantity must be positive")
	}

	m.mu.Lock()
	if _, exists := m.reqs[req.ID]; exists {
		m.mu.Unlock()
		return nil, ErrDuplicateRequest
	}

	now := time.Now()
	req.State = RequestStatePending
	req.CreatedAt = now
	req.UpdatedAt = now
	m.reqs[req.ID] = &req
	m.mu.Unlock()

	m.logger.Info("gpu.SubmitRequest", "id", req.ID, "tenant", req.Tenant, "gpuType", req.GPUType)

	cluster, err := m.BestClusterWithTimeout(ctx, req.GPUType, m.requestTimeout())
	if err != nil {
		m.mu.Lock()
		defer m.mu.Unlock()

		reason := "placement pending"
		if errors.Is(err, ErrGPUNoMatch) {
			reason = "no matching cluster"
		} else if !errors.Is(err, ErrGPUPending) {
			return nil, fmt.Errorf("routing request: %w", err)
		}

		m.reqs[req.ID].Reason = reason
		m.reqs[req.ID].UpdatedAt = time.Now()
		return &AdmissionResult{
			Admitted: false,
			Reason:   reason,
			State:    RequestStatePending,
		}, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.reqs[req.ID].State = RequestStateAdmitted
	m.reqs[req.ID].Cluster = cluster
	m.reqs[req.ID].UpdatedAt = time.Now()

	return &AdmissionResult{
		Admitted: true,
		Cluster:  cluster,
		State:    RequestStateAdmitted,
	}, nil
}

func (m *Manager) GetRequestStatus(_ context.Context, requestID string) (*GPURequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	req, ok := m.reqs[requestID]
	if !ok {
		return nil, ErrRequestNotFound
	}

	copy := *req
	return &copy, nil
}

func (m *Manager) CancelRequest(_ context.Context, requestID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	req, ok := m.reqs[requestID]
	if !ok {
		return ErrRequestNotFound
	}

	if req.State == RequestStateCompleted || req.State == RequestStateCancelled {
		return ErrRequestTerminal
	}

	req.State = RequestStateCancelled
	req.UpdatedAt = time.Now()
	return nil
}

func (m *Manager) CompleteRequest(_ context.Context, requestID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	req, ok := m.reqs[requestID]
	if !ok {
		return ErrRequestNotFound
	}

	if req.State == RequestStateCancelled {
		return ErrRequestCancelled
	}

	req.State = RequestStateCompleted
	req.UpdatedAt = time.Now()
	return nil
}
