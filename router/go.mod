module github.com/atsokha/mcplake/router

go 1.27.1

require (
	github.com/atsokha/mcplake/auth v0.0.0-00010101000000-000000000000
	github.com/stretchr/testify v1.12.1
	github.com/theory/jsonpath v0.12.1
)

require go.yaml.in/yaml/v3 v3.0.5 // indirect

replace github.com/atsokha/mcplake/auth => ../auth
