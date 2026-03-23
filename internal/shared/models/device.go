package models

import "errors"

// DeviceRegistrationRequest is sent by the client on first run.
// POST /api/v1/devices
type DeviceRegistrationRequest struct {
	ClientDeviceID string `json:"client_device_id"`
	OS             string `json:"os"`
	Hostname       string `json:"hostname,omitempty"`
}

// Validate checks required fields.
func (r *DeviceRegistrationRequest) Validate() error {
	if r.ClientDeviceID == "" {
		return errors.New("client_device_id is required")
	}
	if r.OS == "" {
		return errors.New("os is required")
	}
	return nil
}

// DeviceRegistrationResponse is returned by the server after device registration.
type DeviceRegistrationResponse struct {
	DeviceID       string `json:"device_id"`
	ClientDeviceID string `json:"client_device_id"`
	Status         string `json:"status"`
}
