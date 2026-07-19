// Package services — Supabase Admin API client, used exclusively for the
// change-temporary-password flow (Stage 6). This is the only place in
// the backend that holds/uses the service_role key.
package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// SupabaseAdminClient wraps the subset of Supabase's GoTrue Admin API
// this backend needs. Build once at startup, reuse for the process
// lifetime — same pattern as AuthMiddleware/RateLimiter.
type SupabaseAdminClient struct {
	baseURL         string
	serviceRoleKey  string
	httpClient      *http.Client
}

func NewSupabaseAdminClient(supabaseURL, serviceRoleKey string) *SupabaseAdminClient {
	return &SupabaseAdminClient{
		baseURL:        strings.TrimRight(supabaseURL, "/"),
		serviceRoleKey: serviceRoleKey,
		httpClient:     &http.Client{Timeout: 10 * time.Second},
	}
}

// AdminUser models the subset of GoTrue's admin user response this
// backend reads.
type AdminUser struct {
	ID           string                 `json:"id"`
	UserMetadata map[string]interface{} `json:"user_metadata"`
}

// GetUserByID fetches a user's current record (including user_metadata)
// via GET /auth/v1/admin/users/{id}.
func (s *SupabaseAdminClient) GetUserByID(ctx context.Context, authID string) (*AdminUser, error) {
	url := fmt.Sprintf("%s/auth/v1/admin/users/%s", s.baseURL, authID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("supabase admin: build get request: %w", err)
	}
	s.setAuthHeaders(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("supabase admin: get user request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("supabase admin: get user returned %d: %s", resp.StatusCode, string(body))
	}

	var user AdminUser
	if err := json.Unmarshal(body, &user); err != nil {
		return nil, fmt.Errorf("supabase admin: decode get user response: %w", err)
	}
	return &user, nil
}

// UpdateUser changes the password and replaces user_metadata via
// PUT /auth/v1/admin/users/{id}. Pass the FULL desired metadata map —
// GoTrue's admin update replaces user_metadata wholesale, it does not
// deep-merge. Callers must fetch-then-merge themselves (see the handler).
func (s *SupabaseAdminClient) UpdateUser(ctx context.Context, authID, newPassword string, metadata map[string]interface{}) error {
	url := fmt.Sprintf("%s/auth/v1/admin/users/%s", s.baseURL, authID)

	payload := map[string]interface{}{
		"password":      newPassword,
		"user_metadata": metadata,
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("supabase admin: marshal update payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("supabase admin: build update request: %w", err)
	}
	s.setAuthHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("supabase admin: update user request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("supabase admin: update user returned %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func (s *SupabaseAdminClient) setAuthHeaders(req *http.Request) {
	req.Header.Set("apikey", s.serviceRoleKey)
	req.Header.Set("Authorization", "Bearer "+s.serviceRoleKey)
}