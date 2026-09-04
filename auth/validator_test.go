package auth_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/MicahParks/jwkset"
	"github.com/atsokha/mcplake/auth"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testIssuer   = "https://issuer.example.test"
	testAudience = "mcp-gateway-test"
)

// jwksTestServer serves a mutable JWK Set (protected by a mutex) so tests can
// simulate key rotation, and counts requests so tests can assert on caching
// behavior.
type jwksTestServer struct {
	mu   sync.Mutex
	keys []jwkset.JWKMarshal
	hits int
	srv  *httptest.Server
}

func newJWKSTestServer() *jwksTestServer {
	s := &jwksTestServer{}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		s.mu.Lock()
		s.hits++
		keys := append([]jwkset.JWKMarshal(nil), s.keys...)
		s.mu.Unlock()

		body, err := json.Marshal(jwkset.JWKSMarshal{Keys: keys})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	return s
}

func (s *jwksTestServer) addKey(t *testing.T, pub *rsa.PublicKey, kid string) {
	t.Helper()
	jwk, err := jwkset.NewJWKFromKey(pub, jwkset.JWKOptions{
		Metadata: jwkset.JWKMetadataOptions{KID: kid, ALG: jwkset.AlgRS256},
	})
	require.NoError(t, err)

	s.mu.Lock()
	s.keys = append(s.keys, jwk.Marshal())
	s.mu.Unlock()
}

func (s *jwksTestServer) hitCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hits
}

func (s *jwksTestServer) close() { s.srv.Close() }

func (s *jwksTestServer) url() string { return s.srv.URL }

func generateRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return key
}

func signToken(t *testing.T, key *rsa.PrivateKey, kid string, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	signed, err := token.SignedString(key)
	require.NoError(t, err)
	return signed
}

func baseClaims() jwt.MapClaims {
	now := time.Now()
	return jwt.MapClaims{
		"iss":    testIssuer,
		"aud":    testAudience,
		"sub":    "user-123",
		"iat":    now.Unix(),
		"exp":    now.Add(time.Hour).Unix(),
		"groups": []string{"oncall-db", "team-data-eng"},
	}
}

func newValidator(t *testing.T, jwksURL string, ttl time.Duration) *auth.Validator {
	t.Helper()
	v, err := auth.NewValidator(context.Background(), auth.Config{
		JWKSURL:      jwksURL,
		Issuer:       testIssuer,
		Audience:     testAudience,
		JWKSCacheTTL: ttl,
		HTTPClient:   http.DefaultClient,
	})
	require.NoError(t, err)
	return v
}

func TestNewValidator_FailsWhenJWKSUnreachable(t *testing.T) {
	// Nothing listens on this address, so the initial synchronous fetch
	// inside NewValidator must fail.
	srv := newJWKSTestServer()
	unreachable := srv.url()
	srv.close()

	_, err := auth.NewValidator(context.Background(), auth.Config{
		JWKSURL:  unreachable,
		Issuer:   testIssuer,
		Audience: testAudience,
	})
	assert.Error(t, err)
}

func TestNewValidator_RequiresConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  auth.Config
	}{
		{"missing JWKSURL", auth.Config{Issuer: testIssuer, Audience: testAudience}},
		{"missing Issuer", auth.Config{JWKSURL: "http://example.test/jwks.json", Audience: testAudience}},
		{"missing Audience", auth.Config{JWKSURL: "http://example.test/jwks.json", Issuer: testIssuer}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := auth.NewValidator(context.Background(), tt.cfg)
			assert.Error(t, err)
		})
	}
}

func TestValidateToken_ValidTokenReturnsClaims(t *testing.T) {
	srv := newJWKSTestServer()
	defer srv.close()
	key := generateRSAKey(t)
	srv.addKey(t, &key.PublicKey, "kid1")

	v := newValidator(t, srv.url(), time.Hour)
	tokenStr := signToken(t, key, "kid1", baseClaims())

	claims, err := v.ValidateToken(context.Background(), tokenStr)
	require.NoError(t, err)
	assert.Equal(t, "user-123", claims.Subject)
	assert.Equal(t, testIssuer, claims.Issuer)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(claims.Raw, &decoded))
	groups, ok := decoded["groups"].([]any)
	require.True(t, ok)
	assert.Contains(t, groups, "oncall-db")
}

func TestValidateToken_ExpiredTokenIsUnauthorized(t *testing.T) {
	srv := newJWKSTestServer()
	defer srv.close()
	key := generateRSAKey(t)
	srv.addKey(t, &key.PublicKey, "kid1")
	v := newValidator(t, srv.url(), time.Hour)

	claims := baseClaims()
	claims["iat"] = time.Now().Add(-2 * time.Hour).Unix()
	claims["exp"] = time.Now().Add(-time.Hour).Unix()
	tokenStr := signToken(t, key, "kid1", claims)

	_, err := v.ValidateToken(context.Background(), tokenStr)
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrUnauthorized)
}

func TestValidateToken_WrongSignatureIsUnauthorized(t *testing.T) {
	srv := newJWKSTestServer()
	defer srv.close()
	trustedKey := generateRSAKey(t)
	srv.addKey(t, &trustedKey.PublicKey, "kid1")
	v := newValidator(t, srv.url(), time.Hour)

	attackerKey := generateRSAKey(t)
	// Same kid as the trusted key, but signed with a different private key.
	tokenStr := signToken(t, attackerKey, "kid1", baseClaims())

	_, err := v.ValidateToken(context.Background(), tokenStr)
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrUnauthorized)
}

func TestValidateToken_WrongIssuerIsUnauthorized(t *testing.T) {
	srv := newJWKSTestServer()
	defer srv.close()
	key := generateRSAKey(t)
	srv.addKey(t, &key.PublicKey, "kid1")
	v := newValidator(t, srv.url(), time.Hour)

	claims := baseClaims()
	claims["iss"] = "https://not-the-issuer.example.test"
	tokenStr := signToken(t, key, "kid1", claims)

	_, err := v.ValidateToken(context.Background(), tokenStr)
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrUnauthorized)
}

func TestValidateToken_WrongAudienceIsUnauthorized(t *testing.T) {
	srv := newJWKSTestServer()
	defer srv.close()
	key := generateRSAKey(t)
	srv.addKey(t, &key.PublicKey, "kid1")
	v := newValidator(t, srv.url(), time.Hour)

	claims := baseClaims()
	claims["aud"] = "someone-else"
	tokenStr := signToken(t, key, "kid1", claims)

	_, err := v.ValidateToken(context.Background(), tokenStr)
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrUnauthorized)
}

func TestValidateToken_MalformedTokenIsUnauthorized(t *testing.T) {
	srv := newJWKSTestServer()
	defer srv.close()
	key := generateRSAKey(t)
	srv.addKey(t, &key.PublicKey, "kid1")
	v := newValidator(t, srv.url(), time.Hour)

	_, err := v.ValidateToken(context.Background(), "not-a-jwt")
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrUnauthorized)
}

func TestValidateToken_NestedAndNamespacedClaimsPreservedInRaw(t *testing.T) {
	srv := newJWKSTestServer()
	defer srv.close()
	key := generateRSAKey(t)
	srv.addKey(t, &key.PublicKey, "kid1")
	v := newValidator(t, srv.url(), time.Hour)

	claims := baseClaims()
	claims["https://example.com/org"] = map[string]any{"tier": "enterprise"}
	tokenStr := signToken(t, key, "kid1", claims)

	got, err := v.ValidateToken(context.Background(), tokenStr)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(got.Raw, &decoded))
	org, ok := decoded["https://example.com/org"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "enterprise", org["tier"])
}

func TestValidateToken_JWKSNotRefetchedOnKnownKID(t *testing.T) {
	srv := newJWKSTestServer()
	defer srv.close()
	key := generateRSAKey(t)
	srv.addKey(t, &key.PublicKey, "kid1")
	v := newValidator(t, srv.url(), time.Hour)

	require.Equal(t, 1, srv.hitCount(), "NewValidator should fetch the JWKS exactly once")

	tokenStr := signToken(t, key, "kid1", baseClaims())
	for range 5 {
		_, err := v.ValidateToken(context.Background(), tokenStr)
		require.NoError(t, err)
	}

	assert.Equal(t, 1, srv.hitCount(), "a known kid within the TTL must not trigger a refetch")
}

func TestValidateToken_UnknownKIDTriggersOutOfBandRefresh(t *testing.T) {
	srv := newJWKSTestServer()
	defer srv.close()
	key1 := generateRSAKey(t)
	srv.addKey(t, &key1.PublicKey, "kid1")

	// Long TTL: only an unknown-kid refresh should be able to pick up kid2.
	v := newValidator(t, srv.url(), time.Hour)
	require.Equal(t, 1, srv.hitCount())

	key2 := generateRSAKey(t)
	srv.addKey(t, &key2.PublicKey, "kid2") // simulate key rotation on the provider

	tokenStr := signToken(t, key2, "kid2", baseClaims())
	claims, err := v.ValidateToken(context.Background(), tokenStr)
	require.NoError(t, err)
	assert.Equal(t, "user-123", claims.Subject)
	assert.GreaterOrEqual(t, srv.hitCount(), 2, "an unrecognized kid should trigger an out-of-band refresh")
}

func TestValidator_Close(t *testing.T) {
	srv := newJWKSTestServer()
	defer srv.close()
	key := generateRSAKey(t)
	srv.addKey(t, &key.PublicKey, "kid1")
	v := newValidator(t, srv.url(), time.Hour)

	assert.NoError(t, v.Close())
}
