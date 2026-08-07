package telemetry

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"github.com/google/uuid"
)

const (
	Audience = "deploycrate-telemetry"
	KeyID    = "deploycrate-telemetry-v1"

	// Tokens are installation credentials rather than user sessions. Keeping a
	// fixed expiry makes the signed value stable across Environment revisions.
	tokenExpiresAt = int64(4102444800) // 2100-01-01T00:00:00Z
)

type Identity struct {
	issuer     string
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
}

func New(signingKey, issuer string) (Identity, error) {
	signingKey = strings.TrimSpace(signingKey)
	issuer = strings.TrimSpace(issuer)
	if signingKey == "" {
		return Identity{}, errors.New("telemetry identity signing key is required")
	}
	parsed, err := url.Parse(issuer)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.User != nil ||
		parsed.RawQuery != "" ||
		parsed.Fragment != "" {
		return Identity{}, errors.New(
			"telemetry identity issuer must be an absolute HTTP URL without credentials, query, or fragment",
		)
	}

	derivation := hmac.New(sha256.New, []byte(signingKey))
	_, _ = derivation.Write([]byte("deploycrate-ce/telemetry-identity/ed25519/v1"))
	seed := derivation.Sum(nil)
	privateKey := ed25519.NewKeyFromSeed(seed)
	publicKey := make(ed25519.PublicKey, ed25519.PublicKeySize)
	copy(publicKey, privateKey.Public().(ed25519.PublicKey))

	return Identity{issuer: issuer, privateKey: privateKey, publicKey: publicKey}, nil
}

func (identity Identity) Issuer() string {
	return identity.issuer
}

func (identity Identity) EnvironmentToken(environmentID uuid.UUID) (string, error) {
	if environmentID == uuid.Nil {
		return "", errors.New("telemetry Environment identity is required")
	}
	return identity.sign(tokenClaims{
		Issuer:        identity.issuer,
		Audience:      Audience,
		Subject:       "environment:" + environmentID.String(),
		ExpiresAt:     tokenExpiresAt,
		EnvironmentID: environmentID.String(),
	})
}

func (identity Identity) NodeToken(serverID uuid.UUID) (string, error) {
	if serverID == uuid.Nil {
		return "", errors.New("telemetry Node identity is required")
	}
	return identity.sign(tokenClaims{
		Issuer:    identity.issuer,
		Audience:  Audience,
		Subject:   "node:" + serverID.String(),
		ExpiresAt: tokenExpiresAt,
		ServerID:  serverID.String(),
	})
}

func (identity Identity) PublicJWKSet() (string, error) {
	encoded, err := json.Marshal(jwkSet{Keys: []jwk{{
		KeyType:   "OKP",
		Use:       "sig",
		KeyID:     KeyID,
		Curve:     "Ed25519",
		X:         base64.RawURLEncoding.EncodeToString(identity.publicKey),
		Algorithm: "EdDSA",
	}}})
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func (identity Identity) sign(claims tokenClaims) (string, error) {
	header, err := json.Marshal(tokenHeader{Algorithm: "EdDSA", KeyID: KeyID, Type: "JWT"})
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encodedHeader := base64.RawURLEncoding.EncodeToString(header)
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	signed := encodedHeader + "." + encodedPayload
	signature := ed25519.Sign(identity.privateKey, []byte(signed))
	return signed + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

type tokenHeader struct {
	Algorithm string `json:"alg"`
	KeyID     string `json:"kid"`
	Type      string `json:"typ"`
}

type tokenClaims struct {
	Issuer        string `json:"iss"`
	Audience      string `json:"aud"`
	Subject       string `json:"sub"`
	ExpiresAt     int64  `json:"exp"`
	EnvironmentID string `json:"deploycrate_environment_id,omitempty"`
	ServerID      string `json:"deploycrate_server_id,omitempty"`
}

type jwkSet struct {
	Keys []jwk `json:"keys"`
}

type jwk struct {
	KeyType   string `json:"kty"`
	Use       string `json:"use"`
	KeyID     string `json:"kid"`
	Curve     string `json:"crv"`
	X         string `json:"x"`
	Algorithm string `json:"alg"`
}
