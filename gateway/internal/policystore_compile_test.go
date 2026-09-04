package internal_test

import (
	"github.com/atsokha/mcplake/internal"
	"github.com/atsokha/mcplake/router"
)

// A compile-time assertion that *router.PolicyStore satisfies
// internal.PolicyEngine — router.PolicyStore (ticket #45) is meant to be a
// drop-in for gateway.Config.Policy wherever *router.Engine was used
// directly before.
var _ internal.PolicyEngine = (*router.PolicyStore)(nil)
