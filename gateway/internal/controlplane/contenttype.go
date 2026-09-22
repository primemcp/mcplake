package controlplane

import (
	"mime"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const jsonMediaType = "application/json"

// requireJSONContentType rejects a body-carrying admin request that does not
// declare a JSON body.
//
// This is the control plane's CSRF defence, and it works by leaning on the
// browser rather than on a token. A cross-origin request needs no preflight
// only when it is a "simple request", which limits it to GET/HEAD/POST with
// a Content-Type of text/plain, application/x-www-form-urlencoded or
// multipart/form-data. Gin's ShouldBindJSON binds with binding.JSON without
// consulting Content-Type, so before this guard a page the operator merely
// visited could POST a registration to a control plane on localhost and get
// arbitrary command execution out of it (mcp.NewClient runs connect.command).
// Demanding application/json forces a preflight, which an unconfigured
// control plane never answers, so the browser never sends the request.
//
// Scope is deliberately narrow:
//
//   - PUT and PATCH are included because every one of them carries a JSON
//     body here, and a uniform rule is easier to reason about than a list of
//     exceptions. They already require a preflight of their own.
//   - DELETE is excluded: it carries no body in this API, and a
//     cross-origin DELETE is already preflighted, so requiring a header
//     would break `curl -X DELETE` in exchange for nothing.
//   - GET is excluded for the same reason, which also keeps the two open
//     bootstrap routes (healthz, auth/config) reachable.
//
// It is installed ahead of admin auth, so a rejected request never reaches
// the authenticator and the response does not depend on whether admin auth
// is configured.
func requireJSONContentType() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch:
		default:
			c.Next()
			return
		}

		if !isJSONContentType(c.GetHeader("Content-Type")) {
			c.AbortWithStatusJSON(http.StatusUnsupportedMediaType, errorResponse{
				Error:   "unsupported_media_type",
				Message: "Content-Type must be application/json",
			})
			return
		}
		c.Next()
	}
}

// isJSONContentType accepts application/json with any parameters
// ("application/json; charset=utf-8") and any casing, since both are
// well-formed and commonly sent, and rejects everything else including an
// absent header.
func isJSONContentType(header string) bool {
	if header == "" {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(header)
	if err != nil {
		return false
	}
	return strings.EqualFold(mediaType, jsonMediaType)
}
