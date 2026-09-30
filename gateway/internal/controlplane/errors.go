package controlplane

import (
	"errors"
	"net/http"

	"github.com/atsokha/mcplake/internal/controlplane/adminservice"
	"github.com/gin-gonic/gin"
)

// errorResponse is the structured error body every admin handler returns on
// failure, matching the {"error","message"} shape already used by the
// data-plane's POST /v1/call (see docs/reference/data-plane-api.rst).
type errorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// respondServiceError maps an adminservice sentinel error to the admin API's
// HTTP status + {"error","message"} body. Anything unrecognized is a 500.
// It is the single place the Gin handlers translate the transport-agnostic
// service layer's failures.
func respondServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, adminservice.ErrInvalidRequest):
		c.JSON(http.StatusBadRequest, errorResponse{Error: "invalid_request", Message: err.Error()})
	case errors.Is(err, adminservice.ErrInvalidPolicy):
		c.JSON(http.StatusBadRequest, errorResponse{Error: "invalid_policy", Message: err.Error()})
	case errors.Is(err, adminservice.ErrRegistrationFailed):
		c.JSON(http.StatusBadGateway, errorResponse{Error: "registration_failed", Message: err.Error()})
	case errors.Is(err, adminservice.ErrNotFound):
		c.JSON(http.StatusNotFound, errorResponse{Error: "not_found", Message: err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "internal_error", Message: err.Error()})
	}
}
