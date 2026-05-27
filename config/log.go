package config

import "log/slog"

type Log struct {
	// note: slog.Level implements UnmarshalJSON so i will just use yaml.UseJSONUnmarshaler() to use it
	Level   slog.Level `yaml:"level"`
	NoColor bool       `yaml:"no_color"`
	InJson  bool       `yaml:"in_json"`
}
