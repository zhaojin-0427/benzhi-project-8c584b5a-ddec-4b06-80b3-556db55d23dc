package httpui_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"manuscript-conservation-gate/internal/application"
	"manuscript-conservation-gate/internal/audit"
	"manuscript-conservation-gate/internal/httpui"
	"manuscript-conservation-gate/internal/store"
)

func TestWorkbenchHealthAndBadJSON(t *testing.T) {
	repository, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	issuer, _ := audit.NewIssuer("http-test-credential-secret")
	handler := httpui.New(application.NewService(repository, issuer))
	page := httptest.NewRecorder()
	handler.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "古籍修复放行门禁") {
		t.Fatalf("workbench unavailable: %d", page.Code)
	}
	if page.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("missing security headers")
	}
	health := httptest.NewRecorder()
	handler.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("health failed: %d %s", health.Code, health.Body.String())
	}
	bad := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/cases", strings.NewReader("{"))
	request.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(bad, request)
	if bad.Code != http.StatusBadRequest || !strings.Contains(bad.Body.String(), "BAD_JSON") {
		t.Fatalf("bad json mapping failed: %d %s", bad.Code, bad.Body.String())
	}
}
