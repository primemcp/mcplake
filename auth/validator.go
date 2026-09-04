// Package auth verifies caller JWTs against an OIDC provider's JWKS before
// any policy evaluation runs. See
// docs/architecture/components.md#auth-validator-auth for this package's role
// in the request pipeline, and ADR-0002 for why Claims carries the full
// decoded claim document rather than a flattened map.
package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/MicahParks/jwkset"
	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/time/rate"
)

// ErrUnauthorized wraps every token-validation failure (malformed token, bad
// signature, expired, wrong issuer/audience) so callers can distinguish
// "caller is unauthorized" from a configuration/programmer error with
// errors.Is(err, ErrUnauthorized), regardless of the specific cause.
var ErrUnauthorized = errors.New("auth: unauthorized")

// defaultJWKSCacheTTL is used when Config.JWKSCacheTTL is unset.
const defaultJWKSCacheTTL = time.Hour

// defaultUnknownKIDRefreshInterval bounds how often an unrecognized key ID
// can trigger an out-of-band JWKS refresh, so a burst of tokens signed with
// an unknown kid can't be used to hammer the OIDC provider.
const defaultUnknownKIDRefreshInterval = time.Minute

// Config configures a Validator.
type Config struct {
	// JWKSURL is the OIDC provider's JSON Web Key Set endpoint.
	JWKSURL string
	// Issuer is the required `iss` claim value.
	Issuer string
	// Audience is the required `aud` claim value.
	Audience string
	// JWKSCacheTTL is how long fetched keys are cached before a background
	// refresh. Defaults to 1 hour. The cache is also refreshed out-of-band
	// (rate-limited) whenever a token references an unrecognized key ID.
	JWKSCacheTTL time.Duration
	// HTTPClient is used to fetch the JWKS. Defaults to http.DefaultClient;
	// tests should supply one pointed at a local test server.
	HTTPClient *http.Client
}

// Claims is the result of successfully validating a token.
type Claims struct {
	Subject string
	Issuer  string
	// Raw is the full decoded claim set, exactly as it appeared in the
	// token's payload segment. The Policy Engine (see ADR-0002) runs
	// JSONPath expressions against this to reach nested/namespaced claims
	// that Subject/Issuer don't expose.
	Raw json.RawMessage
}

// Validator verifies JWTs against a cached JWKS.
type Validator struct {
	cfg Config
	kf  keyfunc.Keyfunc
}

// NewValidator fetches the OIDC provider's JWKS once (failing if that first
// fetch fails) and returns a Validator that keeps the key set current via a
// background refresh every Config.JWKSCacheTTL, plus a rate-limited
// out-of-band refresh whenever a token references an unrecognized key ID.
func NewValidator(ctx context.Context, cfg Config) (*Validator, error) {
	if cfg.JWKSURL == "" {
		return nil, fmt.Errorf("auth: Config.JWKSURL is required")
	}
	if cfg.Issuer == "" {
		return nil, fmt.Errorf("auth: Config.Issuer is required")
	}
	if cfg.Audience == "" {
		return nil, fmt.Errorf("auth: Config.Audience is required")
	}
	if cfg.JWKSCacheTTL <= 0 {
		cfg.JWKSCacheTTL = defaultJWKSCacheTTL
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = http.DefaultClient
	}

	perURLStorage, err := jwkset.NewStorageFromHTTP(cfg.JWKSURL, jwkset.HTTPClientStorageOptions{
		Client:          cfg.HTTPClient,
		Ctx:             ctx,
		RefreshInterval: cfg.JWKSCacheTTL,
	})
	if err != nil {
		return nil, fmt.Errorf("auth: initial JWKS fetch from %s: %w", cfg.JWKSURL, err)
	}

	combined, err := jwkset.NewHTTPClient(jwkset.HTTPClientOptions{
		HTTPURLs: map[string]jwkset.Storage{
			cfg.JWKSURL: perURLStorage,
		},
		RefreshUnknownKID: rate.NewLimiter(rate.Every(defaultUnknownKIDRefreshInterval), 1),
	})
	if err != nil {
		return nil, fmt.Errorf("auth: build JWKS client: %w", err)
	}

	kf, err := keyfunc.New(keyfunc.Options{Storage: combined})
	if err != nil {
		return nil, fmt.Errorf("auth: build keyfunc: %w", err)
	}

	return &Validator{cfg: cfg, kf: kf}, nil
}

// ValidateToken verifies token's signature against the cached JWKS and
// validates the standard exp/iat/iss/aud claims. On success it returns the
// full decoded claim set. Every failure is wrapped in ErrUnauthorized.
func (v *Validator) ValidateToken(ctx context.Context, token string) (*Claims, error) {
	var mapClaims jwt.MapClaims
	_, err := jwt.ParseWithClaims(
		token,
		&mapClaims,
		v.kf.KeyfuncCtx(ctx),
		jwt.WithIssuer(v.cfg.Issuer),
		jwt.WithAudience(v.cfg.Audience),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnauthorized, err)
	}

	raw, err := rawPayload(token)
	if err != nil {
		return nil, fmt.Errorf("%w: decode claim payload: %w", ErrUnauthorized, err)
	}

	subject, _ := mapClaims["sub"].(string)
	issuer, _ := mapClaims["iss"].(string)

	return &Claims{
		Subject: subject,
		Issuer:  issuer,
		Raw:     raw,
	}, nil
}

// Close releases the Validator's resources (the JWKS background refresh
// goroutine, which is tied to the context passed to NewValidator).
func (v *Validator) Close() error {
	return nil
}

// rawPayload base64url-decodes a JWT's payload (second) segment, preserving
// the claim set exactly as issued rather than round-tripping it through
// jwt.MapClaims (which would renumber/reorder values).
func rawPayload(token string) (json.RawMessage, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("malformed token: expected 3 segments, got %d", len(parts))
	}
	decoded, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("base64url decode payload: %w", err)
	}
	if !json.Valid(decoded) {
		return nil, fmt.Errorf("payload is not valid JSON")
	}
	return json.RawMessage(decoded), nil
}
