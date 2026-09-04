module github.com/atsokha/mcplake/config

go 1.27.1

require (
	github.com/atsokha/mcplake/router v0.0.0-00010101000000-000000000000
	github.com/stretchr/testify v1.12.1
)

require (
	github.com/MicahParks/jwkset v0.11.1 // indirect
	github.com/MicahParks/keyfunc/v3 v3.8.1 // indirect
	github.com/atsokha/mcplake/auth v0.0.0-00010101000000-000000000000 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/theory/jsonpath v0.12.1 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/time v0.15.0 // indirect
)

replace (
	github.com/atsokha/mcplake/auth => ../auth
	github.com/atsokha/mcplake/router => ../router
)
