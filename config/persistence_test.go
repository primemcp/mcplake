package config_test

import (
	"testing"

	"github.com/primemcp/mcplake/config"
	"github.com/primemcp/mcplake/persistence"
	"github.com/stretchr/testify/assert"
)

func TestConfig_PersistenceConfig_MapsFields(t *testing.T) {
	cfg := config.Config{
		Persistence: config.PersistenceConfig{Driver: "postgres", DSN: "host=db"},
	}

	assert.Equal(t, persistence.Config{Driver: "postgres", DSN: "host=db"}, cfg.PersistenceConfig())
}

func TestConfig_PersistenceConfig_EmptyDriverDefersToPersistenceDefault(t *testing.T) {
	cfg := config.Config{}

	assert.Equal(t, persistence.Config{}, cfg.PersistenceConfig())
}
