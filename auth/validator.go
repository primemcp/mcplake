package auth

import (
	"context"
	"fmt"
)

type Validator struct {
	// TODO: Add OIDC provider configuration
	// TODO: Add JWKS cache
}

type Claims struct {
	Subject string
	Issuer  string
	// Custom claims extracted from JWT
	Custom map[string]interface{}
}

func NewValidator(ctx context.Context) (*Validator, error) {
	// TODO: Initialize OIDC provider
	// TODO: Fetch and cache JWKS
	return &Validator{}, nil
}

func (v *Validator) ValidateToken(ctx context.Context, token string) (*Claims, error) {
	// TODO: Parse JWT
	// TODO: Verify signature using cached JWKS
	// TODO: Validate standard claims (exp, iat, iss)
	// TODO: Extract custom claims
	return nil, fmt.Errorf("not implemented")
}

func (v *Validator) Close() error {
	// TODO: Cleanup resources
	return nil
}
