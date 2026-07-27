package config

import (
	"net"
	"net/url"

	"github.com/caarlos0/env/v11"
)

type ClickHouse struct {
	Protocol string `env:"CLICKHOUSE_PROTOCOL"`
	Host     string `env:"CLICKHOUSE_HOST"`
	Port     string `env:"CLICKHOUSE_PORT"`
	Database string `env:"CLICKHOUSE_DATABASE"`
	User     string `env:"CLICKHOUSE_USER"`
	Password string `env:"CLICKHOUSE_PASSWORD"`
}

func (c ClickHouse) GetURL() string {
	clickHouseURL := &url.URL{
		Scheme: c.Protocol,
		Host:   net.JoinHostPort(c.Host, c.Port),
	}
	return clickHouseURL.String()
}

func newClickHouseConfig() ClickHouse {
	configuration := ClickHouse{}
	if err := env.ParseWithOptions(&configuration, env.Options{RequiredIfNoDef: true}); err != nil {
		panic(err)
	}
	return configuration
}
