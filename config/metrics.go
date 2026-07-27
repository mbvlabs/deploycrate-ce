package config

import "github.com/caarlos0/env/v11"

type Metrics struct {
	Enabled       bool   `env:"METRICS_ROLLUP_ENABLED" envDefault:"false"`
	PrometheusURL string `env:"PROMETHEUS_URL"         envDefault:"http://127.0.0.1:9090"`
}

func newMetricsConfig() Metrics {
	configuration := Metrics{}
	if err := env.ParseWithOptions(&configuration, env.Options{RequiredIfNoDef: true}); err != nil {
		panic(err)
	}
	return configuration
}
