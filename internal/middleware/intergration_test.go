package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Its-Ameekh/school_software_backend/internal/middleware"
	"github.com/gin-gonic/gin"
)

// Minimal mock user to simulate context extraction since we are testing isolation bounds
type mockUser struct {
	ID   uint
	Role string
}

func TestRateLimiter_BurstAndThrottle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	limiter := middleware.NewRateLimiter()

	r.GET("/test-limiter", limiter.Limit(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// The limiter allows 60/min + 10 burst = 70 capacity
	// Fire 70 requests instantly — they should all pass
	for i := 0; i < 70; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test-limiter", nil)
		req.RemoteAddr = "192.168.1.100:1234"
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Expected request %d to pass, but got status %d", i+1, w.Code)
		}
	}

	// The 71st request should be throttled immediately with a 429
	req := httptest.NewRequest(http.MethodGet, "/test-limiter", nil)
	req.RemoteAddr = "192.168.1.100:1234"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("Expected 71st request to return 429, got %d", w.Code)
	}

	// Verify error payload structure matches exactly what we specified
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("Failed to parse error body: %v", err)
	}
	if body["error"] != "too_many_requests" {
		t.Errorf("Expected error field to be 'too_many_requests', got %q", body["error"])
	}
}

func TestRequireRoles(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name         string
		allowedRoles []string
		userRole     string
		expectStatus int
	}{
		{
			name:         "Principal accessing Principal route",
			allowedRoles: []string{"PRINCIPAL"},
			userRole:     "PRINCIPAL",
			expectStatus: http.StatusOK,
		},
		{
			name:         "Teacher accessing Principal route",
			allowedRoles: []string{"PRINCIPAL"},
			userRole:     "TEACHER",
			expectStatus: http.StatusForbidden,
		},
		{
			name:         "Parent accessing shared route",
			allowedRoles: []string{"PRINCIPAL", "PARENT"},
			userRole:     "PARENT",
			expectStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()

			// Mock upstream middleware setting the context role
			r.Use(func(c *gin.Context) {
				c.Set("auth_user_role", tt.userRole)
				c.Next()
			})

			r.GET("/test-rbac", middleware.RequireRoles(tt.allowedRoles...), func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "/test-rbac", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.expectStatus {
				t.Errorf("Expected status %d, got %d. Body: %s", tt.expectStatus, w.Code, w.Body.String())
			}
		})
	}
}
