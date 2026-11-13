package services

import (
	"testing"

	"github.com/joho/godotenv"
)

func TestFormatEnvironmentSecrets(t *testing.T) {
	secrets := []ResolvedEnvironmentSecret{
		{Key: "ZEROES", Value: "00123"},
		{Key: "SPECIAL", Value: " spaces # = \"quoted\" 'single' $TOKEN ${OTHER} "},
		{Key: "MULTILINE", Value: "first\nsecond\r\nthird"},
		{Key: "BACKSLASH", Value: `path\to\file`},
		{Key: "EMPTY", Value: ""},
	}
	text := formatEnvironmentSecrets(secrets)
	parsed, err := godotenv.Unmarshal(text)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range secrets {
		if parsed[secret.Key] != secret.Value {
			t.Errorf("value for %s did not round-trip", secret.Key)
		}
	}
	if formatEnvironmentSecrets(nil) != "" {
		t.Fatal("empty secrets should produce empty output")
	}
}
