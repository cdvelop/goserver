package httpd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"webtyp.com/model"
	"webtyp.com/router"
)

type mockPolicy struct {
	grants []model.RoleGrant
}

func (m mockPolicy) Grants() []model.RoleGrant {
	return m.grants
}

func dummyAuthorizer(userID string, resource model.Resource, action model.Action) bool {
	return false
}

type routeItem struct {
	Path        string   `json:"path"`
	Method      string   `json:"method"`
	Roles       []string `json:"roles"`
	PolicyKnown bool     `json:"policy_known"`
}

type routesEnvelope struct {
	Routes []routeItem `json:"routes"`
}

func TestRoutesEndpointDisabled(t *testing.T) {
	cfg := Config{
		RoutesEndpoint: false,
	}
	s := New(cfg)

	handler, err := s.Handler()
	if err != nil {
		t.Fatalf("Handler() error: %v", err)
	}

	req := httptest.NewRequest("GET", RoutesPath, nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404 Not Found when RoutesEndpoint is disabled, got %d", w.Code)
	}
}

func TestRoutesEndpointListsItself(t *testing.T) {
	cfg := Config{
		RoutesEndpoint: true,
	}
	s := New(cfg)

	handler, err := s.Handler()
	if err != nil {
		t.Fatalf("Handler() error: %v", err)
	}

	req := httptest.NewRequest("GET", RoutesPath, nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d", w.Code)
	}

	var env routesEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("Failed to unmarshal routes response: %v", err)
	}

	found := false
	for _, r := range env.Routes {
		if r.Path == RoutesPath {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected route table to list %s, but it was missing", RoutesPath)
	}
}

func TestRoutesEndpointReportsRoles(t *testing.T) {
	pol := mockPolicy{
		grants: []model.RoleGrant{
			{
				Role: model.RoleCode("admin"),
				Grant: model.Grant{
					Resource: model.Resource("catalog"),
					Actions:  model.Read,
				},
			},
		},
	}

	cfg := Config{
		RoutesEndpoint: true,
		Policy:         pol,
		Authorize:      dummyAuthorizer,
	}
	s := New(cfg)
	s.Router().Get("/items", func(ctx router.Context) {}).Requires("catalog", model.Read)

	handler, err := s.Handler()
	if err != nil {
		t.Fatalf("Handler() error: %v", err)
	}

	req := httptest.NewRequest("GET", RoutesPath, nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d", w.Code)
	}

	var env routesEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("Failed to unmarshal routes response: %v", err)
	}

	var target *routeItem
	for i := range env.Routes {
		if env.Routes[i].Path == "/items" {
			target = &env.Routes[i]
			break
		}
	}

	if target == nil {
		t.Fatalf("Expected route /items in endpoint response")
	}
	if !target.PolicyKnown {
		t.Errorf("Expected PolicyKnown to be true")
	}
	if len(target.Roles) != 1 || target.Roles[0] != "admin" {
		t.Errorf("Expected roles [\"admin\"], got %v", target.Roles)
	}
}

func TestRoutesEndpointReportsUnheldPermission(t *testing.T) {
	pol := mockPolicy{
		grants: []model.RoleGrant{},
	}

	cfg := Config{
		RoutesEndpoint: true,
		Policy:         pol,
		Authorize:      dummyAuthorizer,
	}
	s := New(cfg)
	s.Router().Get("/items", func(ctx router.Context) {}).Requires("catalog", model.Read)

	handler, err := s.Handler()
	if err != nil {
		t.Fatalf("Handler() error: %v", err)
	}

	req := httptest.NewRequest("GET", RoutesPath, nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d", w.Code)
	}

	var env routesEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("Failed to unmarshal routes response: %v", err)
	}

	var target *routeItem
	for i := range env.Routes {
		if env.Routes[i].Path == "/items" {
			target = &env.Routes[i]
			break
		}
	}

	if target == nil {
		t.Fatalf("Expected route /items in endpoint response")
	}
	if !target.PolicyKnown {
		t.Errorf("Expected PolicyKnown to be true for unheld permission")
	}
	if len(target.Roles) != 0 {
		t.Errorf("Expected empty roles slice [], got %v", target.Roles)
	}
}

func TestRoutesEndpointWithoutPolicy(t *testing.T) {
	cfg := Config{
		RoutesEndpoint: true,
		Policy:         nil,
		Authorize:      dummyAuthorizer,
	}
	s := New(cfg)
	s.Router().Get("/items", func(ctx router.Context) {}).Requires("catalog", model.Read)

	handler, err := s.Handler()
	if err != nil {
		t.Fatalf("Handler() error: %v", err)
	}

	req := httptest.NewRequest("GET", RoutesPath, nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d", w.Code)
	}

	var env routesEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("Failed to unmarshal routes response: %v", err)
	}

	for _, r := range env.Routes {
		if r.Path == "/items" && r.PolicyKnown {
			t.Errorf("Expected PolicyKnown to be false when Policy is nil for route /items")
		}
	}
}

func TestRoutesEndpointRegisteredOnce(t *testing.T) {
	cfg := Config{
		RoutesEndpoint: true,
	}
	s := New(cfg)

	if _, err := s.Handler(); err != nil {
		t.Fatalf("First Handler() error: %v", err)
	}
	if _, err := s.Handler(); err != nil {
		t.Fatalf("Second Handler() error: %v", err)
	}

	routes := s.Router().Routes()
	count := 0
	for _, r := range routes {
		if r.Path == RoutesPath {
			count++
		}
	}

	if count != 1 {
		t.Errorf("Expected exactly 1 registration of %s, got %d", RoutesPath, count)
	}
}
