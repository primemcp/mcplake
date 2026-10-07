module github.com/primemcp/mcplake/router

go 1.27.1

require (
	github.com/primemcp/mcplake/auth v0.0.0-00010101000000-000000000000
	github.com/primemcp/mcplake/filter v0.0.0-00010101000000-000000000000
	github.com/stretchr/testify v1.12.1
	github.com/theory/jsonpath v0.12.1
)

require (
	github.com/MicahParks/jwkset v0.11.1 // indirect
	github.com/MicahParks/keyfunc/v3 v3.8.1 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/time v0.15.0 // indirect
)

replace (
	github.com/primemcp/mcplake/auth => ../auth
	github.com/primemcp/mcplake/filter => ../filter
)
