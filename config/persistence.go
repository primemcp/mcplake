package config

import "github.com/atsokha/mcplake/persistence"

// PersistenceConfig converts the Persistence entry into a persistence.Config,
// ready to pass to persistence.Open. This is the only place the YAML shape
// becomes persistence's domain type — persistence itself has no knowledge of
// the YAML config shape.
func (c *Config) PersistenceConfig() persistence.Config {
	return persistence.Config{
		Driver: c.Persistence.Driver,
		DSN:    c.Persistence.DSN,
	}
}
