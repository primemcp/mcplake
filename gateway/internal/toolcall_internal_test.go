package internal

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
)

func newToolCallCtx(method, body, authHeader string) *fasthttp.RequestCtx {
	var ctx fasthttp.RequestCtx
	ctx.Request.Header.SetMethod(method)
	ctx.Request.SetRequestURI("/v1/call")
	ctx.Request.SetBodyString(body)
	if authHeader != "" {
		ctx.Request.Header.Set("Authorization", authHeader)
	}
	return &ctx
}

func TestParseToolCallRequest_ValidRequest(t *testing.T) {
	ctx := newToolCallCtx(fasthttp.MethodPost, `{"mcp":"postgres-ro","tool":"get_user","arguments":{"id":42}}`, "Bearer abc.def.ghi")

	req, err := parseToolCallRequest(ctx)

	require.NoError(t, err)
	assert.Equal(t, "postgres-ro", req.MCP)
	assert.Equal(t, "get_user", req.Tool)
	assert.Equal(t, "abc.def.ghi", req.Token)
	assert.Equal(t, float64(42), req.Arguments["id"])
}

func TestParseToolCallRequest_MalformedJSON(t *testing.T) {
	ctx := newToolCallCtx(fasthttp.MethodPost, `{not json`, "Bearer abc")

	_, err := parseToolCallRequest(ctx)

	require.Error(t, err)
	var reqErr *requestError
	require.ErrorAs(t, err, &reqErr)
	assert.Equal(t, fasthttp.StatusBadRequest, reqErr.status)
	assert.Equal(t, "invalid_json", reqErr.code)
}

func TestParseToolCallRequest_EmptyBody(t *testing.T) {
	ctx := newToolCallCtx(fasthttp.MethodPost, ``, "Bearer abc")

	_, err := parseToolCallRequest(ctx)

	require.Error(t, err)
	var reqErr *requestError
	require.ErrorAs(t, err, &reqErr)
	assert.Equal(t, fasthttp.StatusBadRequest, reqErr.status)
}

func TestParseToolCallRequest_MissingMCP(t *testing.T) {
	ctx := newToolCallCtx(fasthttp.MethodPost, `{"tool":"get_user"}`, "Bearer abc")

	_, err := parseToolCallRequest(ctx)

	require.Error(t, err)
	var reqErr *requestError
	require.ErrorAs(t, err, &reqErr)
	assert.Equal(t, fasthttp.StatusBadRequest, reqErr.status)
	assert.Equal(t, "missing_field", reqErr.code)
}

func TestParseToolCallRequest_MissingTool(t *testing.T) {
	ctx := newToolCallCtx(fasthttp.MethodPost, `{"mcp":"postgres-ro"}`, "Bearer abc")

	_, err := parseToolCallRequest(ctx)

	require.Error(t, err)
	var reqErr *requestError
	require.ErrorAs(t, err, &reqErr)
	assert.Equal(t, fasthttp.StatusBadRequest, reqErr.status)
	assert.Equal(t, "missing_field", reqErr.code)
}

func TestExtractBearerToken(t *testing.T) {
	tests := []struct {
		name       string
		header     string
		wantToken  string
		wantErr    bool
		wantStatus int
		wantCode   string
	}{
		{
			name:      "valid bearer token",
			header:    "Bearer abc.def.ghi",
			wantToken: "abc.def.ghi",
		},
		{
			name:       "missing header",
			header:     "",
			wantErr:    true,
			wantStatus: fasthttp.StatusUnauthorized,
			wantCode:   "missing_authorization",
		},
		{
			name:       "wrong scheme",
			header:     "Basic dXNlcjpwYXNz",
			wantErr:    true,
			wantStatus: fasthttp.StatusUnauthorized,
			wantCode:   "invalid_authorization",
		},
		{
			name:       "empty bearer token",
			header:     "Bearer ",
			wantErr:    true,
			wantStatus: fasthttp.StatusUnauthorized,
			wantCode:   "invalid_authorization",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := newToolCallCtx(fasthttp.MethodPost, `{"mcp":"m","tool":"t"}`, tt.header)

			token, err := extractBearerToken(ctx)

			if tt.wantErr {
				require.Error(t, err)
				var reqErr *requestError
				require.ErrorAs(t, err, &reqErr)
				assert.Equal(t, tt.wantStatus, reqErr.status)
				assert.Equal(t, tt.wantCode, reqErr.code)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantToken, token)
		})
	}
}
