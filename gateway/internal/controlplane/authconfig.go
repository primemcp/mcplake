package controlplane

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// AuthInfo is what the embedded admin web UI needs to know before it can
// authenticate: whether it has to at all, and — if so — the OIDC
// coordinates for an Authorization Code + PKCE flow against the provider
// the gateway already verifies tokens from. See ADR-0014.
//
// It is served unauthenticated (see NewServer), which is safe by
// construction: every field here is the *public* half of a PKCE client. The
// client_id and the provider's endpoints travel in the authorization
// request itself, visible to anyone who watches the redirect, and a public
// client has no secret to leak. Gating it would be circular — a caller
// would need a token to learn how to obtain a token. Do not add anything
// here that isn't public in that sense.
type AuthInfo struct {
	// AuthRequired mirrors whether the admin-auth gate is installed. When
	// false the UI skips the login flow entirely and behaves as it did
	// before ADR-0010.
	AuthRequired bool `json:"auth_required"`
	// Issuer is oidc.issuer, shown to the operator so they can see which
	// provider they are about to be sent to.
	Issuer string `json:"issuer,omitempty"`
	// ClientID is the public client registered for the UI.
	ClientID string `json:"client_id,omitempty"`
	// AuthorizationEndpoint is where the UI redirects the browser.
	AuthorizationEndpoint string `json:"authorization_endpoint,omitempty"`
	// TokenEndpoint is where the UI exchanges the code for a token.
	TokenEndpoint string `json:"token_endpoint,omitempty"`
	// Scopes is what to request. Empty when login isn't configured.
	Scopes []string `json:"scopes,omitempty"`
	// The omitempty above is load-bearing: with AuthRequired true and no
	// [admin_auth.login], the UI must be able to tell "log in like this"
	// from "auth is on but nobody configured a login", and it does so by
	// these fields being absent rather than empty strings.
}

// @Summary      Admin UI login configuration
// @Description  How the embedded admin web UI should authenticate: whether admin auth is enforced and, if so, the OIDC Authorization Code + PKCE coordinates to use. Unauthenticated — a browser with no token yet has to be able to read it (ADR-0014). Discloses only the public half of a PKCE client (client_id, provider endpoints, scopes); there is no client secret.
// @Tags         auth
// @Produce      json
// @Success      200  {object}  controlplane.AuthInfo
// @Router       /auth/config [get]
func authConfig(info AuthInfo) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, info)
	}
}

// LoginConfigured reports whether the info carries a usable login flow, so
// the caller can warn about "admin auth on, but the UI has no way in".
func (a AuthInfo) LoginConfigured() bool {
	return a.ClientID != "" && a.AuthorizationEndpoint != "" && a.TokenEndpoint != ""
}
