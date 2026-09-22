package filter_test

import (
	"encoding/json"
	"testing"

	"github.com/atsokha/mcplake/filter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// envelope is what an MCP server actually returns: a CallToolResult whose
// payload appears TWICE -- once as structured content, once serialized into
// a text block, because the Go SDK adds that fallback whenever a typed tool
// handler leaves Content unset (go-sdk mcp/server.go, "return the
// serialized JSON in a TextContent block, as the spec suggests").
//
// Anything that filters only one of the two copies leaks the other.
func envelope(t *testing.T, payload string) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"content":           []any{map[string]any{"type": "text", "text": payload}},
		"structuredContent": json.RawMessage(payload),
		"isError":           false,
	})
	require.NoError(t, err)
	return raw
}

// decodeEnvelope returns the structured payload and the text of the first
// content block, both decoded, so assertions can check each copy.
func decodeEnvelope(t *testing.T, raw json.RawMessage) (structured map[string]any, text map[string]any, textRaw string) {
	t.Helper()
	var env struct {
		Content           []map[string]any `json:"content"`
		StructuredContent map[string]any   `json:"structuredContent"`
	}
	require.NoError(t, json.Unmarshal(raw, &env))
	if len(env.Content) > 0 {
		textRaw, _ = env.Content[0]["text"].(string)
		_ = json.Unmarshal([]byte(textRaw), &text)
	}
	return env.StructuredContent, text, textRaw
}

func TestStripToolResult_NoFieldsIsNoOp(t *testing.T) {
	in := envelope(t, `{"id":1,"hashed_password":"xyz"}`)

	out, removed, err := filter.StripToolResult(in, nil)

	require.NoError(t, err)
	assert.Zero(t, removed)
	assert.JSONEq(t, string(in), string(out))
}

// The whole point: drop_fields stay authored against the tool's own record
// (what docs/CONFIG.md and config.example.toml show), not against the
// envelope the transport happens to wrap it in.
func TestStripToolResult_RecordRelativePathStripsBothCopies(t *testing.T) {
	in := envelope(t, `{"id":1,"email":"a@example.com","hashed_password":"xyz"}`)

	out, removed, err := filter.StripToolResult(in, []string{"$.hashed_password"})

	require.NoError(t, err)
	assert.Equal(t, 2, removed, "one location in structuredContent, one in the text copy")

	structured, text, textRaw := decodeEnvelope(t, out)
	assert.NotContains(t, structured, "hashed_password")
	assert.NotContains(t, text, "hashed_password")
	assert.NotContains(t, textRaw, "xyz", "the secret must not survive anywhere in the text block")
	assert.Equal(t, "a@example.com", structured["email"], "untargeted fields are untouched")
	assert.Equal(t, "a@example.com", text["email"])
}

func TestStripToolResult_StripsNestedAndListFields(t *testing.T) {
	in := envelope(t, `{"rows":[{"id":1,"ssn":"a"},{"id":2,"ssn":"b"}],"meta":{"token":"t"}}`)

	out, removed, err := filter.StripToolResult(in, []string{"$.rows[*].ssn", "$.meta.token"})

	require.NoError(t, err)
	assert.Equal(t, 6, removed, "3 locations per copy")

	_, _, textRaw := decodeEnvelope(t, out)
	assert.NotContains(t, textRaw, `"ssn"`)
	assert.NotContains(t, textRaw, `"token"`)
	assert.Contains(t, textRaw, `"id"`)
}

func TestStripToolResult_StructuredOnlyResult(t *testing.T) {
	in := json.RawMessage(`{"content":[],"structuredContent":{"id":1,"secret":"s"}}`)

	out, removed, err := filter.StripToolResult(in, []string{"$.secret"})

	require.NoError(t, err)
	assert.Equal(t, 1, removed)
	assert.NotContains(t, string(out), "secret")
}

func TestStripToolResult_TextOnlyJSONResult(t *testing.T) {
	in := json.RawMessage(`{"content":[{"type":"text","text":"{\"id\":1,\"secret\":\"s\"}"}]}`)

	out, removed, err := filter.StripToolResult(in, []string{"$.secret"})

	require.NoError(t, err)
	assert.Equal(t, 1, removed)
	assert.NotContains(t, string(out), "secret")
}

// A field path that genuinely isn't in this response stays a no-op --
// DropFields are authored once against a tool's general shape, and an
// optional field being absent is not an error.
func TestStripToolResult_UnmatchedPathIsANoOp(t *testing.T) {
	in := envelope(t, `{"id":1}`)

	out, removed, err := filter.StripToolResult(in, []string{"$.nonexistent"})

	require.NoError(t, err)
	assert.Zero(t, removed, "the caller uses this to warn about a filter that does nothing")
	assert.JSONEq(t, string(in), string(out))
}

// Fail closed: a filter policy matched this call, but the response carries
// free-form text that field paths cannot be applied to. Returning it
// unfiltered would silently hand over whatever the operator meant to strip.
func TestStripToolResult_FailsClosedOnTextThatIsNotJSON(t *testing.T) {
	in := json.RawMessage(`{"content":[{"type":"text","text":"the ssn is 123-45-6789"}]}`)

	_, _, err := filter.StripToolResult(in, []string{"$.ssn"})

	require.Error(t, err)
	assert.ErrorIs(t, err, filter.ErrUnenforceable)
}

// Same response, but with no filter in force -- nothing to enforce, so it
// passes through untouched.
func TestStripToolResult_FreeFormTextIsFineWhenNoFilterApplies(t *testing.T) {
	in := json.RawMessage(`{"content":[{"type":"text","text":"the ssn is 123-45-6789"}]}`)

	out, removed, err := filter.StripToolResult(in, nil)

	require.NoError(t, err)
	assert.Zero(t, removed)
	assert.JSONEq(t, string(in), string(out))
}

// Non-text content (images, audio, embedded resources) carries no JSON
// fields to strip, so it is passed through rather than failing the call.
func TestStripToolResult_NonTextContentIsPassedThrough(t *testing.T) {
	in := json.RawMessage(`{"content":[{"type":"image","data":"AAAA","mimeType":"image/png"}],"structuredContent":{"secret":"s"}}`)

	out, removed, err := filter.StripToolResult(in, []string{"$.secret"})

	require.NoError(t, err)
	assert.Equal(t, 1, removed)
	assert.Contains(t, string(out), "image/png")
	assert.NotContains(t, string(out), `"secret"`)
}

func TestStripToolResult_PreservesEnvelopeFieldsItDoesNotUnderstand(t *testing.T) {
	in := json.RawMessage(`{"content":[],"structuredContent":{"a":1,"secret":"s"},"isError":false,"_meta":{"trace":"t"}}`)

	out, _, err := filter.StripToolResult(in, []string{"$.secret"})

	require.NoError(t, err)
	var env map[string]any
	require.NoError(t, json.Unmarshal(out, &env))
	assert.Equal(t, false, env["isError"])
	assert.Equal(t, map[string]any{"trace": "t"}, env["_meta"])
}

func TestStripToolResult_RejectsAMalformedPathAndAMalformedResponse(t *testing.T) {
	t.Run("malformed field path", func(t *testing.T) {
		_, _, err := filter.StripToolResult(envelope(t, `{"a":1}`), []string{"not a path"})
		require.Error(t, err)
	})

	t.Run("response is not JSON", func(t *testing.T) {
		_, _, err := filter.StripToolResult(json.RawMessage(`{not json`), []string{"$.a"})
		require.Error(t, err)
	})
}
