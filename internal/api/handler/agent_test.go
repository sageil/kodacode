package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/sageil/kodacode/v1/internal/agent"
	"github.com/sageil/kodacode/v1/internal/api/handler"
)

// ---- controllable fake agent service for agent tests -----------------------

type controlAgentSvc struct {
	agents map[string]agent.Agent
}

func newControlAgentSvc(initial ...agent.Agent) *controlAgentSvc {
	s := &controlAgentSvc{agents: make(map[string]agent.Agent)}
	for _, a := range initial {
		s.agents[a.ID] = a
	}
	return s
}

func (s *controlAgentSvc) List() []agent.Agent {
	out := make([]agent.Agent, 0, len(s.agents))
	for _, a := range s.agents {
		out = append(out, a)
	}
	return out
}

func (s *controlAgentSvc) Get(id string) (agent.Agent, error) {
	a, ok := s.agents[id]
	if !ok {
		return agent.Agent{}, agent.ErrNotFound
	}
	return a, nil
}

func (s *controlAgentSvc) Create(a agent.Agent) (agent.Agent, error) {
	s.agents[a.ID] = a
	return a, nil
}

func (s *controlAgentSvc) Update(id string, a agent.Agent) (agent.Agent, error) {
	existing, ok := s.agents[id]
	if !ok {
		return agent.Agent{}, agent.ErrNotFound
	}
	if existing.Builtin {
		return agent.Agent{}, agent.ErrBuiltin
	}
	s.agents[id] = a
	return a, nil
}

func (s *controlAgentSvc) Delete(id string) error {
	a, ok := s.agents[id]
	if !ok {
		return agent.ErrNotFound
	}
	if a.Builtin {
		return agent.ErrBuiltin
	}
	delete(s.agents, id)
	return nil
}

// ---- GET /agents -----------------------------------------------------------

func TestAgentHandler_List(t *testing.T) {
	e := newEcho()
	agents := newControlAgentSvc(
		agent.Agent{ID: "default", Name: "Default"},
		agent.Agent{ID: "builder", Name: "builder"},
	)
	handler.RegisterRoutes(e, newFakeSvc(), agents, nil, nil, nil)

	rec := doRequest(t, e, http.MethodGet, "/agents", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /agents = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body)
	}

	var got []agent.Agent
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("GET /agents len = %d, want 2", len(got))
	}
}

// ---- GET /agents/:id -------------------------------------------------------

func TestAgentHandler_Get(t *testing.T) {
	e := newEcho()
	a := agent.Agent{ID: "default", Name: "Default"}
	agents := newControlAgentSvc(a)
	handler.RegisterRoutes(e, newFakeSvc(), agents, nil, nil, nil)

	rec := doRequest(t, e, http.MethodGet, "/agents/default", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /agents/default = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body)
	}

	var got agent.Agent
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != "default" {
		t.Errorf("GET /agents/default ID = %q, want %q", got.ID, "default")
	}
}

func TestAgentHandler_Get_notFound(t *testing.T) {
	e := newEcho()
	handler.RegisterRoutes(e, newFakeSvc(), newControlAgentSvc(), nil, nil, nil)

	rec := doRequest(t, e, http.MethodGet, "/agents/nope", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /agents/nope = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// ---- POST /agents ----------------------------------------------------------

func TestAgentHandler_Create(t *testing.T) {
	e := newEcho()
	handler.RegisterRoutes(e, newFakeSvc(), newControlAgentSvc(), nil, nil, nil)

	rec := doRequest(t, e, http.MethodPost, "/agents", map[string]any{
		"id":            "my-agent",
		"name":          "My Agent",
		"system_prompt": "You are helpful.",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /agents = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body)
	}

	var got agent.Agent
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != "my-agent" {
		t.Errorf("POST /agents ID = %q, want %q", got.ID, "my-agent")
	}
}

func TestAgentHandler_Create_missingID(t *testing.T) {
	e := newEcho()
	handler.RegisterRoutes(e, newFakeSvc(), newControlAgentSvc(), nil, nil, nil)

	rec := doRequest(t, e, http.MethodPost, "/agents", map[string]any{
		"name": "No ID Agent",
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST /agents (no id) = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// ---- PUT /agents/:id -------------------------------------------------------

func TestAgentHandler_Update(t *testing.T) {
	e := newEcho()
	existing := agent.Agent{ID: "my-agent", Name: "Old Name"}
	agents := newControlAgentSvc(existing)
	handler.RegisterRoutes(e, newFakeSvc(), agents, nil, nil, nil)

	rec := doRequest(t, e, http.MethodPut, "/agents/my-agent", map[string]any{
		"name":          "New Name",
		"system_prompt": "Updated prompt.",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT /agents/my-agent = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body)
	}

	var got agent.Agent
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Name != "New Name" {
		t.Errorf("PUT /agents/my-agent Name = %q, want %q", got.Name, "New Name")
	}
}

func TestAgentHandler_Update_notFound(t *testing.T) {
	e := newEcho()
	handler.RegisterRoutes(e, newFakeSvc(), newControlAgentSvc(), nil, nil, nil)

	rec := doRequest(t, e, http.MethodPut, "/agents/nope", map[string]any{
		"name": "X",
	})
	if rec.Code != http.StatusNotFound {
		t.Errorf("PUT /agents/nope = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestAgentHandler_Update_builtin(t *testing.T) {
	e := newEcho()
	builtin := agent.Agent{ID: "default", Name: "Default", Builtin: true}
	agents := newControlAgentSvc(builtin)
	handler.RegisterRoutes(e, newFakeSvc(), agents, nil, nil, nil)

	rec := doRequest(t, e, http.MethodPut, "/agents/default", map[string]any{
		"name": "Override",
	})
	if rec.Code != http.StatusForbidden {
		t.Errorf("PUT /agents/default (builtin) = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

// ---- DELETE /agents/:id ----------------------------------------------------

func TestAgentHandler_Delete(t *testing.T) {
	e := newEcho()
	a := agent.Agent{ID: "my-agent", Name: "My Agent"}
	agents := newControlAgentSvc(a)
	handler.RegisterRoutes(e, newFakeSvc(), agents, nil, nil, nil)

	rec := doRequest(t, e, http.MethodDelete, "/agents/my-agent", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE /agents/my-agent = %d, want %d; body=%s", rec.Code, http.StatusNoContent, rec.Body)
	}

	// Verify deleted.
	rec2 := doRequest(t, e, http.MethodGet, "/agents/my-agent", nil)
	if rec2.Code != http.StatusNotFound {
		t.Errorf("GET after DELETE /agents/my-agent = %d, want %d", rec2.Code, http.StatusNotFound)
	}
}

func TestAgentHandler_Delete_notFound(t *testing.T) {
	e := newEcho()
	handler.RegisterRoutes(e, newFakeSvc(), newControlAgentSvc(), nil, nil, nil)

	rec := doRequest(t, e, http.MethodDelete, "/agents/nope", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("DELETE /agents/nope = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestAgentHandler_Delete_builtin(t *testing.T) {
	e := newEcho()
	builtin := agent.Agent{ID: "default", Name: "Default", Builtin: true}
	agents := newControlAgentSvc(builtin)
	handler.RegisterRoutes(e, newFakeSvc(), agents, nil, nil, nil)

	rec := doRequest(t, e, http.MethodDelete, "/agents/default", nil)
	if rec.Code != http.StatusForbidden {
		t.Errorf("DELETE /agents/default (builtin) = %d, want %d", rec.Code, http.StatusForbidden)
	}
}
