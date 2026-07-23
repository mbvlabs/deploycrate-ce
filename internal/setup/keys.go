package setup

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"

	"golang.org/x/crypto/ssh"
)

func GenerateSSHKeyPair() (publicKey string, privateKey string, err error) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("generate Ed25519 key: %w", err)
	}

	privateDER, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		return "", "", fmt.Errorf("marshal Ed25519 private key: %w", err)
	}
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})

	sshPublic, err := ssh.NewPublicKey(public)
	if err != nil {
		return "", "", fmt.Errorf("marshal Ed25519 public key: %w", err)
	}
	publicLine := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPublic))) + " deploycrate-ce"

	return publicLine, string(privatePEM), nil
}

func ValidateSSHPublicKey(value string) error {
	_, _, _, remainder, err := ssh.ParseAuthorizedKey([]byte(strings.TrimSpace(value)))
	if err != nil {
		return fmt.Errorf("invalid SSH public key: %w", err)
	}
	if len(strings.TrimSpace(string(remainder))) != 0 {
		return fmt.Errorf("invalid SSH public key: provide exactly one key")
	}
	return nil
}

func SSHFingerprint(value string) string {
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(strings.TrimSpace(value)))
	if err != nil {
		return "unavailable"
	}
	return ssh.FingerprintSHA256(key)
}
