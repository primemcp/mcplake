module github.com/atsokha/mcplake/cmd/gateway

go 1.27.1

replace (
	github.com/atsokha/mcplake => ../../gateway
	github.com/atsokha/mcplake/auth => ../../auth
	github.com/atsokha/mcplake/cache => ../../cache
	github.com/atsokha/mcplake/config => ../../config
	github.com/atsokha/mcplake/filter => ../../filter
	github.com/atsokha/mcplake/mcp => ../../mcp
	github.com/atsokha/mcplake/router => ../../router
)
