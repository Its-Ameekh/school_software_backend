// Package services contains backend-internal integrations that aren't
// tied to a single HTTP handler. This file implements Stage 3 item 16:
// pre-provisioning School Software accounts via the Supabase Admin API.
//
// CreateAuthUser is the ONLY place in the codebase that should call the
// Supabase Admin API to create a user. Stage 4's admission-intake and
// teacher-onboarding routes are expected to call this function rather
// than reimplementing the HTTP call themselves.
package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/Its-Ameekh/school_software_backend/internal/models" // adjust if your model lives elsewhere
)

// validRoles is the strict allow-list of application roles this backend
// recognizes. The `users.role` column itself is a plain, unconstrained
// VARCHAR(20) — confirmed no CHECK constraint at the DB level — so this
// Go-level allow-list is the ONLY thing enforcing the system design
// doc's PRINCIPAL | TEACHER | PARENT contract. Deliberately ignoring the
// stale "ADMIN"/"GUARDIAN" example values in the models.User comment —
// those predate the finalized role naming in the system design doc.
var validRoles = map[string]bool{
	"PRINCIPAL": true,
	"TEACHER":   true,
	"PARENT":    true,
}

// supabaseAdminCreateUserRequest mirrors the subset of the Supabase Admin
// API's user-creation payload this backend actually uses.
type supabaseAdminCreateUserRequest struct {
	Phone        string         `json:"phone"`
	Password     string         `json:"password"`
	PhoneConfirm bool           `json:"phone_confirm"` // true = mark verified immediately, skips sending a confirmation SMS entirely — this is what avoids per-OTP billing
	UserMetadata map[string]any `json:"user_metadata,omitempty"`
}

// supabaseAdminUserResponse mirrors the subset of the Admin API's create
// response this backend reads. Supabase's own field name is `id` — `sub`
// is what that same UUID is called once it later shows up inside a JWT,
// they're the same value under two different names in two different
// contexts.
type supabaseAdminUserResponse struct {
	ID    string `json:"id"`
	Phone string `json:"phone"`
}

// supabaseErrorResponse best-effort mirrors Supabase Auth's typical error
// body shape. Not guaranteed stable across Supabase versions — used only
// to produce a more readable error message, falling back to the raw body
// if parsing doesn't match.
type supabaseErrorResponse struct {
	ErrorCode string `json:"error_code"`
	Msg       string `json:"msg"`
	Error     string `json:"error"`
}

// CreateAuthUser pre-provisions a School Software account: creates the
// Supabase Auth user (phone + a temporary DOB-derived password, via the
// Admin API, bypassing SMS OTP entirely) and, on success, links it into
// our local `users` table.
//
// Design notes worth confirming before relying on this:
//   - phone must already be in the exact format Supabase expects (E.164,
//     e.g. "+919876543210") — this function does not normalize it.
//   - dob is expected as "YYYY-MM-DD" (matches a Postgres DATE column
//     read back through GORM). See deriveTempPassword's doc comment for
//     the exact password scheme — that's a real decision, not just
//     formatting, since parents/teachers will type this to log in.
//   - role is validated against the strict allow-list above.
//   - serviceRoleKey must never be logged, returned in an error string,
//     or reach any frontend-facing code path — treat it like a database
//     password. It's taken as a parameter here deliberately, so the
//     caller controls exactly where it's loaded from (should be your
//     Stage 1 config system, env-var only, never hardcoded).
//   - Known limitation: if the Supabase Admin API call succeeds but the
//     local DB write then fails, this function attempts a compensating
//     delete of the just-created Supabase user, to keep the two systems
//     from drifting out of sync. If THAT delete also fails, the returned
//     error includes the orphaned auth_id for manual cleanup — there's
//     no way to make this fully atomic across two separate systems.
func CreateAuthUser(
	ctx context.Context,
	db *gorm.DB,
	supabaseURL string,
	serviceRoleKey string,
	phone string,
	dob string,
	role string,
	name string,
) (string, error) {
	role = strings.ToUpper(strings.TrimSpace(role))
	if !validRoles[role] {
		return "", fmt.Errorf("auth_provisioning: invalid role %q — must be one of PRINCIPAL, TEACHER, PARENT", role)
	}

	phone = strings.TrimSpace(phone)
	if phone == "" {
		return "", errors.New("auth_provisioning: phone is required")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("auth_provisioning: name is required")
	}

	tempPassword, err := deriveTempPassword(dob)
	if err != nil {
		return "", fmt.Errorf("auth_provisioning: %w", err)
	}

	authID, err := createSupabaseAuthUser(ctx, supabaseURL, serviceRoleKey, phone, tempPassword)
	if err != nil {
		return "", fmt.Errorf("auth_provisioning: supabase admin create failed: %w", err)
	}

	user := models.User{
		AuthID: authID,
		Phone:  phone,
		Role:   role,
		Name:   name,
	}

	if err := db.WithContext(ctx).Create(&user).Error; err != nil {
		// The Supabase account now exists with no local row to match it.
		// Try to undo it rather than leave the two systems inconsistent.
		if delErr := deleteSupabaseAuthUser(ctx, supabaseURL, serviceRoleKey, authID); delErr != nil {
			return "", fmt.Errorf(
				"auth_provisioning: local DB write failed (%v) AND rollback of Supabase user %s also failed (%v) — manual cleanup required for this auth_id",
				err, authID, delErr,
			)
		}
		return "", fmt.Errorf("auth_provisioning: local DB write failed, Supabase user %s was rolled back successfully: %w", authID, err)
	}

	return authID, nil
}

// deriveTempPassword turns a "YYYY-MM-DD" date of birth into a temporary
// login password. Current scheme: reorder to DDMMYYYY digits, e.g.
// "2020-05-14" -> "14052020" — 8 digits, clears Supabase's default
// minimum password length (6). This is a real design choice worth
// confirming matches what you actually want (vs. e.g. YYYYMMDD, or
// appending the phone's last 4 digits for a bit more entropy) — flagging
// it rather than silently picking one and moving on.
func deriveTempPassword(dob string) (string, error) {
	dob = strings.TrimSpace(dob)
	parsed, err := time.Parse("2006-01-02", dob)
	if err != nil {
		return "", fmt.Errorf("dob %q is not in YYYY-MM-DD format: %w", dob, err)
	}
	return parsed.Format("02012006"), nil // DDMMYYYY
}

// createSupabaseAuthUser calls Supabase's Admin API to create a
// phone+password account with phone_confirm=true, which marks it
// verified immediately and skips sending any confirmation SMS — the
// mechanism that avoids per-OTP billing entirely. Returns the new
// Supabase auth_id (UUID) on success.
func createSupabaseAuthUser(ctx context.Context, supabaseURL, serviceRoleKey, phone, tempPassword string) (string, error) {
	endpoint := strings.TrimRight(supabaseURL, "/") + "/auth/v1/admin/users"

	reqBody := supabaseAdminCreateUserRequest{
		Phone:        phone,
		Password:     tempPassword,
		PhoneConfirm: true,
		UserMetadata: map[string]any{
			"must_change_password": true, // read by the frontend in Stage 6 to force the password-change overlay
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to encode request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", serviceRoleKey)
	req.Header.Set("Authorization", "Bearer "+serviceRoleKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request to supabase admin api failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read supabase admin api response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("supabase admin api returned %d: %s", resp.StatusCode, summarizeSupabaseError(respBody))
	}

	var parsed supabaseAdminUserResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("failed to parse supabase admin api response: %w", err)
	}
	if parsed.ID == "" {
		return "", errors.New("supabase admin api response did not include a user id")
	}

	return parsed.ID, nil
}

// deleteSupabaseAuthUser removes a Supabase Auth user by ID. Used only
// as a compensating action when the local DB write after account
// creation fails — see CreateAuthUser's doc comment.
func deleteSupabaseAuthUser(ctx context.Context, supabaseURL, serviceRoleKey, authID string) error {
	endpoint := strings.TrimRight(supabaseURL, "/") + "/auth/v1/admin/users/" + authID

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to build delete request: %w", err)
	}
	req.Header.Set("apikey", serviceRoleKey)
	req.Header.Set("Authorization", "Bearer "+serviceRoleKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("request to supabase admin api failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("supabase admin api returned %d on delete: %s", resp.StatusCode, summarizeSupabaseError(respBody))
	}
	return nil
}

// summarizeSupabaseError best-effort parses a Supabase error response
// body into a readable string, falling back to the raw (truncated) body
// if the shape doesn't match — Supabase's error format isn't perfectly
// consistent across every endpoint.
func summarizeSupabaseError(body []byte) string {
	var parsed supabaseErrorResponse
	if err := json.Unmarshal(body, &parsed); err == nil {
		if parsed.Msg != "" {
			return parsed.Msg
		}
		if parsed.Error != "" {
			return parsed.Error
		}
	}
	raw := string(body)
	if len(raw) > 300 {
		raw = raw[:300] + "...(truncated)"
	}
	return raw
}
