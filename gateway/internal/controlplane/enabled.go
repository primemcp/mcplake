package controlplane

// enabledOrTrue resolves an optional request `enabled` field to a concrete
// bool. Omitting it (nil) means enabled, matching the documented default and
// preserving pre-#95 behaviour for existing API clients. PUT replaces the
// whole record, so omitting `enabled` on a PUT re-enables the policy.
func enabledOrTrue(enabled *bool) bool {
	return enabled == nil || *enabled
}
