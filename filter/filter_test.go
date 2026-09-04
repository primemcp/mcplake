package filter_test

import (
	"encoding/json"
	"testing"

	"github.com/atsokha/mcplake/filter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStrip_NoFieldsIsNoOp(t *testing.T) {
	in := json.RawMessage(`{"id":1,"email":"a@example.com"}`)

	out, err := filter.Strip(in, nil)

	require.NoError(t, err)
	assert.JSONEq(t, string(in), string(out))
}

func TestStrip_RemovesTopLevelField(t *testing.T) {
	in := json.RawMessage(`{"id":1,"email":"a@example.com","hashed_password":"xyz"}`)

	out, err := filter.Strip(in, []string{"$.hashed_password"})

	require.NoError(t, err)
	assert.JSONEq(t, `{"id":1,"email":"a@example.com"}`, string(out))
}

func TestStrip_RemovesNestedFieldWithoutCorruptingSiblings(t *testing.T) {
	in := json.RawMessage(`{"id":1,"user":{"name":"Alice","email":"a@example.com"},"active":true}`)

	out, err := filter.Strip(in, []string{"$.user.email"})

	require.NoError(t, err)
	assert.JSONEq(t, `{"id":1,"user":{"name":"Alice"},"active":true}`, string(out))
}

func TestStrip_RemovesFieldInsideEveryListElement(t *testing.T) {
	in := json.RawMessage(`{"items":[{"id":1,"secret":"a"},{"id":2,"secret":"b"}]}`)

	out, err := filter.Strip(in, []string{"$.items[*].secret"})

	require.NoError(t, err)
	assert.JSONEq(t, `{"items":[{"id":1},{"id":2}]}`, string(out))
}

func TestStrip_NonexistentPathIsNoOp(t *testing.T) {
	in := json.RawMessage(`{"id":1}`)

	out, err := filter.Strip(in, []string{"$.does_not_exist", "$.nested.also_missing"})

	require.NoError(t, err)
	assert.JSONEq(t, `{"id":1}`, string(out))
}

func TestStrip_MultipleFieldsAllRemoved(t *testing.T) {
	in := json.RawMessage(`{"id":1,"email":"a@example.com","hashed_password":"xyz","internal_id":"i-1"}`)

	out, err := filter.Strip(in, []string{"$.email", "$.hashed_password", "$.internal_id"})

	require.NoError(t, err)
	assert.JSONEq(t, `{"id":1}`, string(out))
}

func TestStrip_OutputIsAlwaysValidJSON(t *testing.T) {
	in := json.RawMessage(`{"id":1,"nested":{"a":1,"b":2},"list":[1,2,3]}`)

	out, err := filter.Strip(in, []string{"$.nested.a", "$.list"})

	require.NoError(t, err)
	assert.True(t, json.Valid(out))
	assert.JSONEq(t, `{"id":1,"nested":{"b":2}}`, string(out))
}

func TestStrip_MalformedResponseIsError(t *testing.T) {
	_, err := filter.Strip(json.RawMessage(`{not json`), []string{"$.id"})

	assert.Error(t, err)
}

func TestStrip_MalformedFieldPathIsError(t *testing.T) {
	_, err := filter.Strip(json.RawMessage(`{"id":1}`), []string{"not a valid jsonpath $$"})

	assert.Error(t, err)
}

func TestStrip_BareArrayElementRemovalIsUnsupportedNoOp(t *testing.T) {
	// DropFields names fields, not array positions; a path resolving
	// directly to an array element (not a key within it) is left in place.
	in := json.RawMessage(`{"items":[1,2,3]}`)

	out, err := filter.Strip(in, []string{"$.items[0]"})

	require.NoError(t, err)
	assert.JSONEq(t, `{"items":[1,2,3]}`, string(out))
}
