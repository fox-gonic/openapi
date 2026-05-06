package handler

import (
	"encoding/json"
	"net/http"

	"github.com/fox-gonic/fox"
)

type StatusResponse[T any] struct {
	status int
	data   T
}

func statusResponse[T any](status int, data T) StatusResponse[T] {
	return StatusResponse[T]{status: status, data: data}
}

func (r StatusResponse[T]) Render(w http.ResponseWriter) error {
	r.WriteContentType(w)
	w.WriteHeader(r.status)
	return json.NewEncoder(w).Encode(r.data)
}

func (r StatusResponse[T]) WriteContentType(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
}

type CreateSandboxRequest struct {
	// TemplateID selects the template used to create the sandbox.
	TemplateID string `json:"template_id" binding:"required"`
}

type SandboxResponse struct {
	// SandboxID is the sandbox instance ID.
	SandboxID string `json:"sandbox_id"`
	// TemplateID is the template used by the sandbox.
	TemplateID string `json:"template_id"`
	// ClientID identifies this platform to SDK clients.
	ClientID string `json:"client_id"`
}

type CreateTemplateRequest struct {
	// Name is the template name.
	Name string `json:"name" binding:"required"`
}

type TemplateResponse struct {
	// TemplateID is the template ID.
	TemplateID string `json:"template_id"`
	// BuildID is the latest template build ID.
	BuildID string `json:"build_id"`
	// Name is the template name.
	Name string `json:"name"`
}

func NewEngine() *fox.Engine {
	engine := fox.New()
	engine.POST("/api/v1/sbx/sandboxes", CreateSandbox)
	engine.POST("/api/v1/sbx/templates", CreateTemplate)
	return engine
}

// CreateSandbox creates a sandbox from a template.
func CreateSandbox(_ *fox.Context, req CreateSandboxRequest) (StatusResponse[SandboxResponse], error) {
	return statusResponse(http.StatusCreated, SandboxResponse{
		SandboxID:  "sbx_123",
		TemplateID: req.TemplateID,
		ClientID:   "client_123",
	}), nil
}

// CreateTemplate creates a template build request.
func CreateTemplate(_ *fox.Context, req CreateTemplateRequest) (StatusResponse[TemplateResponse], error) {
	return statusResponse(http.StatusAccepted, TemplateResponse{
		TemplateID: "tpl_123",
		BuildID:    "build_123",
		Name:       req.Name,
	}), nil
}
