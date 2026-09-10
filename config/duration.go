package config

import "time"

// Duration is a time.Duration that decodes from a Go duration string in the
// config file — "1h", "15m", "500ms" (see time.ParseDuration). An empty or
// absent value decodes to 0, letting each consumer apply its own default.
//
// It implements encoding.TextUnmarshaler, which is honoured by
// github.com/BurntSushi/toml and by encoding/json alike, so the config
// shape is not tied to one file format.
type Duration struct {
	time.Duration
}

// UnmarshalText parses a Go duration string. Empty text is 0, not an error.
func (d *Duration) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		d.Duration = 0
		return nil
	}
	parsed, err := time.ParseDuration(string(text))
	if err != nil {
		return err
	}
	d.Duration = parsed
	return nil
}
