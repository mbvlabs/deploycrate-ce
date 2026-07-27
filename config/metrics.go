package config

import "github.com/caarlos0/env/v11"

type Metrics struct {
	Enabled            bool   `env:"METRICS_ROLLUP_ENABLED" envDefault:"false"`
	PrometheusURL      string `env:"PROMETHEUS_URL"     envDefault:"http://127.0.0.1:9090"`
	ClickHouseURL      string `env:"CLICKHOUSE_URL"     envDefault:"http://127.0.0.1:8123"`
	ClickHouseDatabase string `env:"CLICKHOUSE_DATABASE" envDefault:"deploycrate"`
	ClickHouseUser     string `env:"CLICKHOUSE_USER"    envDefault:"deploycrate"`
	ClickHousePassword string `env:"CLICKHOUSE_PASSWORD" envDefault:""`
}

func newMetricsConfig() Metrics {
	configuration := Metrics{}
	if err := env.ParseWithOptions(&configuration, env.Options{RequiredIfNoDef: true}); err != nil {
		panic(err)
	}
	return configuration
}
