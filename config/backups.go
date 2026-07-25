package config

import "github.com/caarlos0/env/v11"

type Backups struct {
	Enabled          bool   `env:"BACKUPS_ENABLED"          envDefault:"false"`
	ServerSchedule   string `env:"BACKUP_SERVER_SCHEDULE"  envDefault:"0 2 * * *"`
	DatabaseEnabled  bool   `env:"BACKUP_DATABASE_ENABLED" envDefault:"false"`
	DatabaseSchedule string `env:"BACKUP_DATABASE_SCHEDULE" envDefault:"0 */6 * * *"`
}

func newBackupsConfig() Backups {
	configuration := Backups{}
	if err := env.ParseWithOptions(&configuration, env.Options{RequiredIfNoDef: true}); err != nil {
		panic(err)
	}
	return configuration
}
