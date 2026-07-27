package services

import (
	"strings"

	"deploycrate-ce/config"
	"deploycrate-ce/internal/storage"
)

type Identity struct {
	db              storage.Pool
	insertOnly      storage.InsertQueue
	pepper          string
	previousPeppers []string
	tokenSigningKey string
}

func NewIdentity(db storage.Pool, insertOnly storage.InsertQueue, cfg config.Config) Identity {
	previousPeppers := make([]string, 0, len(cfg.Auth.PreviousPeppers))
	for _, pepper := range cfg.Auth.PreviousPeppers {
		if pepper = strings.TrimSpace(pepper); pepper != "" && pepper != cfg.Auth.Pepper {
			previousPeppers = append(previousPeppers, pepper)
		}
	}

	return Identity{
		db:              db,
		insertOnly:      insertOnly,
		pepper:          cfg.Auth.Pepper,
		previousPeppers: previousPeppers,
		tokenSigningKey: cfg.App.TokenSigningKey,
	}
}
