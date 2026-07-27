package config

import (
	"time"

	"github.com/caarlos0/env/v11"
)

type SSHCA struct {
	UserPrivateKeyPath string        `env:"SSH_CA_USER_PRIVATE_KEY_PATH" envDefault:"/var/lib/deploycrate/ssh-ca/user-ca"`
	HostPrivateKeyPath string        `env:"SSH_CA_HOST_PRIVATE_KEY_PATH" envDefault:"/var/lib/deploycrate/ssh-ca/host-ca"`
	UserPrincipal      string        `env:"SSH_CA_USER_PRINCIPAL"        envDefault:"admin"`
	SourceCIDR         string        `env:"SSH_CA_SOURCE_CIDR"           envDefault:"10.99.0.1/32"`
	UserValidity       time.Duration `env:"SSH_CA_USER_VALIDITY"         envDefault:"30m"`
}

func newSSHCAConfig() SSHCA {
	configuration := SSHCA{}
	if err := env.ParseWithOptions(&configuration, env.Options{RequiredIfNoDef: true}); err != nil {
		panic(err)
	}
	return configuration
}
