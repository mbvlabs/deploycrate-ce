package setup

import (
	"fmt"
	"os"
	"strings"

	"deploycrate-ce/internal/secretcrypto"
)

const (
	WireGuardInterface      = "wg0"
	WireGuardNetworkCIDR    = "10.99.0.0/16"
	WireGuardPrivateAddress = "10.99.0.1"
	WireGuardListenPort     = 51820

	wireGuardPrivateKeyPath = "/etc/wireguard/deploycrate-ce.key"
	wireGuardPublicKeyPath  = "/etc/wireguard/deploycrate-ce.pub"
)

type WireGuardIdentity struct {
	PublicKey           string
	EncryptedPrivateKey []byte
}

func LoadWireGuardIdentity(encryptionKey string) (WireGuardIdentity, error) {
	privateKey, err := os.ReadFile(wireGuardPrivateKeyPath)
	if err != nil {
		return WireGuardIdentity{}, fmt.Errorf("read WireGuard private key: %w", err)
	}
	privateKey = []byte(strings.TrimSpace(string(privateKey)))
	if len(privateKey) == 0 {
		return WireGuardIdentity{}, fmt.Errorf("read WireGuard private key: key is empty")
	}
	publicKey, err := os.ReadFile(wireGuardPublicKeyPath)
	if err != nil {
		return WireGuardIdentity{}, fmt.Errorf("read WireGuard public key: %w", err)
	}
	public := strings.TrimSpace(string(publicKey))
	if public == "" {
		return WireGuardIdentity{}, fmt.Errorf("read WireGuard public key: key is empty")
	}
	encrypted, err := secretcrypto.Encrypt(privateKey, encryptionKey)
	if err != nil {
		return WireGuardIdentity{}, fmt.Errorf("encrypt WireGuard private key: %w", err)
	}
	return WireGuardIdentity{PublicKey: public, EncryptedPrivateKey: encrypted}, nil
}
