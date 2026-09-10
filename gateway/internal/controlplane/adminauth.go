package controlplane

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/atsokha/mcplake/auth"
	"github.com/gin-gonic/gin"
)

// adminClaimsKey is the gin.Context key under which AdminAuth stashes the
// verified *auth.Claims for downstream handlers (e.g. a future audit log
// attributing an admin action to claims.Subject).
const adminClaimsKey = "admin_claims"

const bearerPrefix = "Bearer "

// tokenValidator verifies a caller's bearer token and returns its decoded
// claims. Satisfied by *auth.Validator; defined here (consumer-side) so the
// middleware can be tested without a real JWKS.
type tokenValidator interface {
	ValidateToken(ctx context.Context, token string) (*auth.Claims, error)
}

// claimAuthorizer decides whether a verified token's claims belong to an
// admin. Satisfied by router.ClaimMatcher (value receiver), built from
// config's admin_auth.match rules.
type claimAuthorizer interface {
	Matches(claims json.RawMessage) (bool, error)
}

// AdminAuth builds the Gin middleware that guards the control-plane admin
// API (ADR-0010): it requires a Bearer JWT that v accepts and whose claims
// authorizer approves. It is installed by NewServer between the health
// endpoint (which stays open) and every other /admin route.
//
// Rejections use the same {"error","message"} body as the rest of the admin
// API:
//
//   - no Authorization header            -> 401 missing_authorization
//   - not a non-empty "Bearer <token>"   -> 401 invalid_authorization
//   - token fails validation             -> 401 unauthorized
//   - claims don't satisfy authorizer    -> 403 forbidden
func AdminAuth(v tokenValidator, authorizer claimAuthorizer) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" {
			abortAdminAuth(c, http.StatusUnauthorized, "missing_authorization", "Authorization header is required")
			return
		}
		rest, ok := strings.CutPrefix(header, bearerPrefix)
		if !ok {
			abortAdminAuth(c, http.StatusUnauthorized, "invalid_authorization", "Authorization header must use the Bearer scheme")
			return
		}
		token := strings.TrimSpace(rest)
		if token == "" {
			abortAdminAuth(c, http.StatusUnauthorized, "invalid_authorization", "bearer token is empty")
			return
		}

		claims, err := v.ValidateToken(c.Request.Context(), token)
		if err != nil {
			abortAdminAuth(c, http.StatusUnauthorized, "unauthorized", "token failed validation")
			return
		}

		// A matcher error means the verified claims could not be evaluated
		// against the admin rules (e.g. an unexpected claim shape). Fail
		// closed: an admin call must be positively authorized, never
		// authorized by the absence of a clean "no".
		if ok, err := authorizer.Matches(claims.Raw); err != nil || !ok {
			abortAdminAuth(c, http.StatusForbidden, "forbidden", "claims do not satisfy the admin authorization rules")
			return
		}

		c.Set(adminClaimsKey, claims)
		c.Next()
	}
}

func abortAdminAuth(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, errorResponse{Error: code, Message: message})
}
