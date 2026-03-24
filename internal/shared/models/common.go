// Package models defines the shared API request/response types used by both
// the Trasker client and server.
package models

import "fmt"

// APIError is the standard error response returned by all API endpoints.
type APIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

// Error implements the error interface.
func (e *APIError) Error() string {
	return fmt.Sprintf("API error %d: %s", e.Code, e.Message)
}

// HealthResponse is returned by GET /api/v1/health.
type HealthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// Role represents a user role in the system.
type Role string

const (
	RoleMember  Role = "member"
	RoleManager Role = "manager"
	RoleAdmin   Role = "admin"
)

// ValidRoles is the set of allowed role values.
var ValidRoles = map[Role]bool{
	RoleMember:  true,
	RoleManager: true,
	RoleAdmin:   true,
}

// OSType represents a client operating system.
type OSType string

const (
	OSLinux   OSType = "linux"
	OSDarwin  OSType = "darwin"
	OSWindows OSType = "windows"
)
