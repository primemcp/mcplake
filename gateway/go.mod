module github.com/atsokha/mcplake

go 1.27.1

require (
	github.com/atsokha/mcplake/auth v0.0.0-00010101000000-000000000000
	github.com/atsokha/mcplake/cache v0.0.0-00010101000000-000000000000
	github.com/atsokha/mcplake/config v0.0.0-00010101000000-000000000000
	github.com/atsokha/mcplake/filter v0.0.0-00010101000000-000000000000
	github.com/atsokha/mcplake/router v0.0.0-00010101000000-000000000000
)

replace (
	github.com/atsokha/mcplake/auth => ../auth
	github.com/atsokha/mcplake/cache => ../cache
	github.com/atsokha/mcplake/config => ../config
	github.com/atsokha/mcplake/filter => ../filter
	github.com/atsokha/mcplake/mcp => ../mcp
	github.com/atsokha/mcplake/router => ../router
)
