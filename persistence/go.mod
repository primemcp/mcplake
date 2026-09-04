module github.com/atsokha/mcplake/persistence

go 1.27.1

require (
	github.com/atsokha/mcplake/cache v0.0.0-00010101000000-000000000000
	github.com/glebarez/sqlite v1.11.0
	github.com/stretchr/testify v1.12.1
	gorm.io/datatypes v1.2.7
	gorm.io/driver/postgres v1.6.2
	gorm.io/gorm v1.31.2
)

require (
	filippo.io/edwards25519 v1.1.0 // indirect
	github.com/atsokha/mcplake/mcp v0.0.0-00010101000000-000000000000 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/glebarez/go-sqlite v1.21.2 // indirect
	github.com/go-sql-driver/mysql v1.8.1 // indirect
	github.com/google/jsonschema-go v0.4.3 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.10.0 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/mattn/go-isatty v0.0.17 // indirect
	github.com/modelcontextprotocol/go-sdk v1.7.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/segmentio/asm v1.1.3 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/oauth2 v0.35.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.29.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	gorm.io/driver/mysql v1.5.6 // indirect
	modernc.org/libc v1.22.5 // indirect
	modernc.org/mathutil v1.5.0 // indirect
	modernc.org/memory v1.5.0 // indirect
	modernc.org/sqlite v1.23.1 // indirect
)

replace github.com/atsokha/mcplake/cache => ../cache

replace github.com/atsokha/mcplake/mcp => ../mcp
