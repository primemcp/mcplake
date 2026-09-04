module github.com/atsokha/mcplake

go 1.27.1

require (
	github.com/stretchr/testify v1.12.1
	github.com/valyala/fasthttp v1.73.0
)

require (
	github.com/andybalholm/brotli v1.2.2 // indirect
	github.com/klauspost/compress v1.19.1 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
)

replace (
	github.com/atsokha/mcplake/auth => ../auth
	github.com/atsokha/mcplake/cache => ../cache
	github.com/atsokha/mcplake/config => ../config
	github.com/atsokha/mcplake/filter => ../filter
	github.com/atsokha/mcplake/mcp => ../mcp
	github.com/atsokha/mcplake/router => ../router
)
