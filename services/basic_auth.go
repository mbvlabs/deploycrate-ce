package services

import (
	"errors"
	"strings"

	"deploycrate-ce/models"

	"golang.org/x/crypto/bcrypt"
)

const defaultBasicAuthUsername = "deploycrate"

type basicAuthResolution struct {
	Username string
	Hash     string
	Password string
}

func hashBasicAuthPassword(password string) (string, error) {
	password = strings.TrimSpace(password)
	if password == "" {
		return "", errors.Join(models.ErrDomainValidation, errors.New("password is required"))
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func generateBasicAuthPassword() (string, error) {
	generated, err := models.GenerateSecureToken()
	if err != nil {
		return "", err
	}
	return strings.ToLower(generated), nil
}

func resolveBasicAuth(
	existingUsername, existingHash, username, password string,
	generate bool,
) (basicAuthResolution, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		username = strings.TrimSpace(existingUsername)
	}
	if username == "" {
		username = defaultBasicAuthUsername
	}
	password = strings.TrimSpace(password)
	if password != "" {
		hash, err := hashBasicAuthPassword(password)
		if err != nil {
			return basicAuthResolution{}, err
		}
		return basicAuthResolution{Username: username, Hash: hash}, nil
	}
	if generate || strings.TrimSpace(existingHash) == "" {
		generated, err := generateBasicAuthPassword()
		if err != nil {
			return basicAuthResolution{}, err
		}
		hash, err := hashBasicAuthPassword(generated)
		if err != nil {
			return basicAuthResolution{}, err
		}
		return basicAuthResolution{
			Username: username, Hash: hash, Password: generated,
		}, nil
	}
	return basicAuthResolution{Username: username, Hash: existingHash}, nil
}
