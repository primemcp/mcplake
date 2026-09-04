package controlplane

// errorResponse is the structured error body every admin handler returns on
// failure, matching the {"error","message"} shape already used by the
// data-plane's POST /v1/call (see docs/api/data-plane.md).
type errorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}
