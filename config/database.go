package config

import (
	"net"
	"net/url"

	"github.com/caarlos0/env/v11"
)

type Database struct {
	Port         string `env:"DB_PORT"`
	Host         string `env:"DB_HOST"`
	Name         string `env:"DB_NAME"`
	User         string `env:"DB_USER"`
	Password     string `env:"DB_PASSWORD"`
	DatabaseKind string `env:"DB_KIND"`
	SslMode      string `env:"DB_SSL_MODE"`
	SslRootCert  string `env:"DB_SSL_ROOT_CERT" envDefault:""`
}

func (d Database) GetDatabaseURL() string {
	databaseURL := &url.URL{
		Scheme: d.DatabaseKind,
		User:   url.UserPassword(d.User, d.Password),
		Host:   net.JoinHostPort(d.Host, d.Port),
		Path:   d.Name,
	}
	query := databaseURL.Query()
	query.Set("sslmode", d.SslMode)
	if d.SslRootCert != "" {
		query.Set("sslrootcert", d.SslRootCert)
	}
	databaseURL.RawQuery = query.Encode()
	return databaseURL.String()
}

func newDatabaseConfig() Database {
	dataCfg := Database{}

	if err := env.ParseWithOptions(&dataCfg, env.Options{
		RequiredIfNoDef: true,
	}); err != nil {
		panic(err)
	}

	return dataCfg
}
