package services

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"strings"
	"time"

	"deploycrate-ce/config"

	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"
)

type SSHUserCertificate struct {
	PrivateKey  []byte
	PublicKey   []byte
	Certificate []byte
	ValidBefore time.Time
}

type SSHCAService struct {
	configuration config.SSHCA
	now           func() time.Time
}

func NewSSHCAService(configuration config.Config) SSHCAService {
	return SSHCAService{configuration: configuration.SSHCA, now: time.Now}
}

func (service SSHCAService) GenerateUserCertificate(
	validity time.Duration,
) (SSHUserCertificate, error) {
	if service.configuration.UserPrincipal != "admin" {
		return SSHUserCertificate{}, errors.New("SSH user certificate principal must be admin")
	}
	if validity == 0 {
		validity = service.configuration.UserValidity
	}
	if validity <= 0 || validity > service.configuration.UserValidity {
		return SSHUserCertificate{}, fmt.Errorf(
			"SSH user certificate validity must be between one second and %s",
			service.configuration.UserValidity,
		)
	}
	if _, err := netip.ParsePrefix(service.configuration.SourceCIDR); err != nil {
		return SSHUserCertificate{}, fmt.Errorf("parse SSH CA source restriction: %w", err)
	}

	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return SSHUserCertificate{}, fmt.Errorf("generate ephemeral SSH key: %w", err)
	}
	sshPublic, err := ssh.NewPublicKey(public)
	if err != nil {
		return SSHUserCertificate{}, fmt.Errorf("encode ephemeral SSH public key: %w", err)
	}
	signer, err := loadCASigner(service.configuration.UserPrivateKeyPath)
	if err != nil {
		return SSHUserCertificate{}, err
	}
	now := service.now().UTC()
	validBefore := now.Add(validity)
	certificate := &ssh.Certificate{
		Key:             sshPublic,
		Serial:          uint64(now.UnixNano()),
		CertType:        ssh.UserCert,
		KeyId:           "deploycrate-" + uuid.NewString(),
		ValidPrincipals: []string{service.configuration.UserPrincipal},
		ValidAfter:      uint64(now.Add(-time.Minute).Unix()),
		ValidBefore:     uint64(validBefore.Unix()),
		Permissions: ssh.Permissions{CriticalOptions: map[string]string{
			"source-address": service.configuration.SourceCIDR,
		}},
	}
	if err := certificate.SignCert(rand.Reader, signer); err != nil {
		return SSHUserCertificate{}, fmt.Errorf("sign ephemeral SSH certificate: %w", err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		return SSHUserCertificate{}, fmt.Errorf("encode ephemeral SSH private key: %w", err)
	}
	return SSHUserCertificate{
		PrivateKey:  pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER}),
		PublicKey:   ssh.MarshalAuthorizedKey(sshPublic),
		Certificate: ssh.MarshalAuthorizedKey(certificate),
		ValidBefore: validBefore,
	}, nil
}

func (service SSHCAService) SignHostKey(
	publicKey []byte,
	principals []string,
	validity time.Duration,
) ([]byte, error) {
	if validity <= 0 {
		return nil, errors.New("SSH host certificate validity must be positive")
	}
	if len(principals) == 0 {
		return nil, errors.New("SSH host certificate requires at least one principal")
	}
	for _, principal := range principals {
		if strings.TrimSpace(principal) == "" || strings.ContainsAny(principal, "\r\n,") {
			return nil, errors.New("SSH host certificate contains an invalid principal")
		}
	}
	key, _, _, remainder, err := ssh.ParseAuthorizedKey(publicKey)
	if err != nil || len(strings.TrimSpace(string(remainder))) != 0 {
		return nil, errors.New("SSH host public key is invalid")
	}
	signer, err := loadCASigner(service.configuration.HostPrivateKeyPath)
	if err != nil {
		return nil, err
	}
	now := service.now().UTC()
	certificate := &ssh.Certificate{
		Key:             key,
		Serial:          uint64(now.UnixNano()),
		CertType:        ssh.HostCert,
		KeyId:           "deploycrate-host-" + uuid.NewString(),
		ValidPrincipals: principals,
		ValidAfter:      uint64(now.Add(-time.Minute).Unix()),
		ValidBefore:     uint64(now.Add(validity).Unix()),
	}
	if err := certificate.SignCert(rand.Reader, signer); err != nil {
		return nil, fmt.Errorf("sign SSH host certificate: %w", err)
	}
	return ssh.MarshalAuthorizedKey(certificate), nil
}

func loadCASigner(path string) (ssh.Signer, error) {
	privateKey, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read SSH CA private key: %w", err)
	}
	signer, err := ssh.ParsePrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("parse SSH CA private key: %w", err)
	}
	return signer, nil
}
