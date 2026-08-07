package setup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"filippo.io/age"
	"golang.org/x/crypto/ssh"
)

const (
	SSHCARecoveryBundlePath = "/var/lib/deploycrate/ssh-ca/deploycrate-ssh-ca-recovery-v1.age"
	sshCADirectory          = "/var/lib/deploycrate/ssh-ca"
	maxSSHCARecoverySize    = 4 * 1024 * 1024
)

func SSHCARecoveryBundleChecksum() (string, error) {
	file, err := os.Open(SSHCARecoveryBundlePath)
	if err != nil {
		return "", fmt.Errorf("open SSH CA recovery bundle: %w", err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("checksum SSH CA recovery bundle: %w", err)
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

type sshCARecoveryManifest struct {
	Version           int    `json:"version"`
	CreatedAt         string `json:"created_at"`
	UserCAFingerprint string `json:"user_ca_fingerprint"`
	HostCAFingerprint string `json:"host_ca_fingerprint"`
}

func CreateSSHCARecoveryBundle(passphrase string) error {
	passphrase = strings.TrimSpace(passphrase)
	if len(passphrase) < 32 {
		return errors.New("SSH CA recovery passphrase is unavailable")
	}

	files := make(map[string][]byte, 5)
	for _, name := range []string{"user-ca", "user-ca.pub", "host-ca", "host-ca.pub"} {
		content, err := os.ReadFile(filepath.Join(sshCADirectory, name))
		if err != nil {
			return fmt.Errorf("read SSH CA recovery source %s: %w", name, err)
		}
		files[name] = content
	}
	defer clearSSHCARecoveryKeys(files)

	manifest, err := json.MarshalIndent(sshCARecoveryManifest{
		Version:           1,
		CreatedAt:         time.Now().UTC().Format(time.RFC3339),
		UserCAFingerprint: sshFingerprint(files["user-ca.pub"]),
		HostCAFingerprint: sshFingerprint(files["host-ca.pub"]),
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode SSH CA recovery manifest: %w", err)
	}
	files["manifest.json"] = append(manifest, '\n')
	if err := validateSSHCARecovery(files); err != nil {
		return fmt.Errorf("validate SSH CA recovery source: %w", err)
	}
	archive, err := writeSSHCARecoveryArchive(files)
	if err != nil {
		return err
	}
	defer clear(archive)

	recipient, err := age.NewScryptRecipient(passphrase)
	if err != nil {
		return fmt.Errorf("create SSH CA recovery recipient: %w", err)
	}
	temporary, err := os.CreateTemp(sshCADirectory, ".ssh-ca-recovery-*.age")
	if err != nil {
		return fmt.Errorf("create SSH CA recovery bundle: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	encrypted, err := age.Encrypt(temporary, recipient)
	if err != nil {
		temporary.Close()
		return fmt.Errorf("start SSH CA recovery encryption: %w", err)
	}
	if _, err := io.Copy(encrypted, bytes.NewReader(archive)); err != nil {
		encrypted.Close()
		temporary.Close()
		return fmt.Errorf("encrypt SSH CA recovery bundle: %w", err)
	}
	if err := encrypted.Close(); err != nil {
		temporary.Close()
		return fmt.Errorf("finish SSH CA recovery encryption: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync SSH CA recovery bundle: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close SSH CA recovery bundle: %w", err)
	}
	uid, gid, err := serviceAccountOwnership()
	if err != nil {
		return err
	}
	if err := os.Chown(temporaryPath, uid, gid); err != nil {
		return fmt.Errorf("own SSH CA recovery bundle: %w", err)
	}
	if err := os.Rename(temporaryPath, SSHCARecoveryBundlePath); err != nil {
		return fmt.Errorf("activate SSH CA recovery bundle: %w", err)
	}

	bundle, err := os.ReadFile(SSHCARecoveryBundlePath)
	if err != nil {
		return fmt.Errorf("read completed SSH CA recovery bundle: %w", err)
	}
	verifiedArchive, err := decryptSSHCARecoveryBundle(bundle, passphrase)
	if err != nil {
		return fmt.Errorf("verify SSH CA recovery encryption: %w", err)
	}
	defer clear(verifiedArchive)
	verifiedFiles, err := readSSHCARecoveryArchive(verifiedArchive)
	if err != nil {
		return fmt.Errorf("verify SSH CA recovery archive: %w", err)
	}
	defer clearSSHCARecoveryKeys(verifiedFiles)
	if err := validateSSHCARecovery(verifiedFiles); err != nil {
		return fmt.Errorf("verify SSH CA recovery contents: %w", err)
	}
	return nil
}

func RecoverSSHCA(bundlePath, passphrase string) error {
	passphrase = strings.TrimSpace(passphrase)
	if passphrase == "" {
		return errors.New("SSH CA recovery passphrase is required")
	}
	bundle, err := os.ReadFile(bundlePath)
	if err != nil {
		return fmt.Errorf("read SSH CA recovery bundle: %w", err)
	}
	decrypted, err := decryptSSHCARecoveryBundle(bundle, passphrase)
	if err != nil {
		return errors.New("decrypt SSH CA recovery bundle: passphrase or bundle is invalid")
	}
	files, err := readSSHCARecoveryArchive(decrypted)
	for index := range decrypted {
		decrypted[index] = 0
	}
	if err != nil {
		return err
	}
	defer clearSSHCARecoveryKeys(files)
	if err := validateSSHCARecovery(files); err != nil {
		return err
	}

	uid, gid, err := serviceAccountOwnership()
	if err != nil {
		return err
	}
	parent := filepath.Dir(filepath.Dir(SSHCARecoveryBundlePath))
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create SSH CA state parent: %w", err)
	}
	staging, err := os.MkdirTemp(parent, ".ssh-ca-recovery-")
	if err != nil {
		return fmt.Errorf("create SSH CA recovery staging directory: %w", err)
	}
	stagingMoved := false
	defer func() {
		if !stagingMoved {
			_ = os.RemoveAll(staging)
		}
	}()
	if err := os.Chmod(staging, 0o700); err != nil {
		return err
	}
	if err := os.Chown(staging, uid, gid); err != nil {
		return err
	}
	for _, name := range []string{"user-ca", "user-ca.pub", "host-ca", "host-ca.pub"} {
		mode := os.FileMode(0o600)
		if strings.HasSuffix(name, ".pub") {
			mode = 0o644
		}
		path := filepath.Join(staging, name)
		if err := os.WriteFile(path, files[name], mode); err != nil {
			return fmt.Errorf("stage recovered %s: %w", name, err)
		}
		if err := os.Chown(path, uid, gid); err != nil {
			return fmt.Errorf("own recovered %s: %w", name, err)
		}
	}
	recoveryPath := filepath.Join(staging, filepath.Base(SSHCARecoveryBundlePath))
	if err := os.WriteFile(recoveryPath, bundle, 0o600); err != nil {
		return fmt.Errorf("preserve SSH CA recovery bundle: %w", err)
	}
	if err := os.Chown(recoveryPath, uid, gid); err != nil {
		return err
	}

	active := filepath.Dir(SSHCARecoveryBundlePath)
	backup := filepath.Join(
		parent,
		fmt.Sprintf(".ssh-ca-before-recovery-%d", time.Now().UnixNano()),
	)
	if err := os.Rename(active, backup); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stage active SSH CA directory: %w", err)
	}
	if err := os.Rename(staging, active); err != nil {
		_ = os.Rename(backup, active)
		return fmt.Errorf("activate recovered SSH CA directory: %w", err)
	}
	stagingMoved = true
	if err := os.RemoveAll(backup); err != nil {
		return fmt.Errorf("remove superseded SSH CA directory: %w", err)
	}
	return nil
}

func decryptSSHCARecoveryBundle(bundle []byte, passphrase string) ([]byte, error) {
	identity, err := age.NewScryptIdentity(passphrase)
	if err != nil {
		return nil, err
	}
	decrypted, err := age.Decrypt(bytes.NewReader(bundle), identity)
	if err != nil {
		return nil, err
	}
	archive, err := io.ReadAll(io.LimitReader(decrypted, maxSSHCARecoverySize+1))
	if err != nil {
		return nil, err
	}
	if len(archive) > maxSSHCARecoverySize {
		clear(archive)
		return nil, errors.New("SSH CA recovery archive exceeds the size limit")
	}
	return archive, nil
}

func writeSSHCARecoveryArchive(files map[string][]byte) ([]byte, error) {
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	for _, name := range []string{"manifest.json", "user-ca", "user-ca.pub", "host-ca", "host-ca.pub"} {
		mode := int64(0o600)
		if strings.HasSuffix(name, ".pub") {
			mode = 0o644
		}
		header := &tar.Header{
			Name:    "deploycrate-ssh-ca-recovery-v1/" + name,
			Mode:    mode,
			Size:    int64(len(files[name])),
			ModTime: time.Now().UTC(),
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			tarWriter.Close()
			gzipWriter.Close()
			return nil, fmt.Errorf("write SSH CA recovery header: %w", err)
		}
		if _, err := tarWriter.Write(files[name]); err != nil {
			tarWriter.Close()
			gzipWriter.Close()
			return nil, fmt.Errorf("write SSH CA recovery content: %w", err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		gzipWriter.Close()
		return nil, fmt.Errorf("finish SSH CA recovery tar archive: %w", err)
	}
	if err := gzipWriter.Close(); err != nil {
		return nil, fmt.Errorf("finish SSH CA recovery gzip archive: %w", err)
	}
	return buffer.Bytes(), nil
}

func serviceAccountOwnership() (int, int, error) {
	account, err := user.Lookup("deploycrate")
	if err != nil {
		return 0, 0, fmt.Errorf("find deploycrate account: %w", err)
	}
	uid, err := strconv.Atoi(account.Uid)
	if err != nil {
		return 0, 0, fmt.Errorf("parse deploycrate uid: %w", err)
	}
	gid, err := strconv.Atoi(account.Gid)
	if err != nil {
		return 0, 0, fmt.Errorf("parse deploycrate gid: %w", err)
	}
	return uid, gid, nil
}

func clearSSHCARecoveryKeys(files map[string][]byte) {
	for _, name := range []string{"user-ca", "host-ca"} {
		clear(files[name])
	}
}

func readSSHCARecoveryArchive(archive []byte) (map[string][]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, errors.New("SSH CA recovery archive is invalid")
	}
	defer reader.Close()
	tarReader := tar.NewReader(reader)
	files := make(map[string][]byte)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, errors.New("SSH CA recovery archive is invalid")
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		name := strings.TrimPrefix(filepath.Clean(header.Name), "deploycrate-ssh-ca-recovery-v1/")
		allowed := name == "manifest.json" || name == "user-ca" || name == "user-ca.pub" ||
			name == "host-ca" ||
			name == "host-ca.pub"
		if !allowed || header.Size < 1 || header.Size > 1024*1024 {
			return nil, errors.New("SSH CA recovery archive contains an unexpected file")
		}
		content, err := io.ReadAll(io.LimitReader(tarReader, 1024*1024+1))
		if err != nil || int64(len(content)) != header.Size {
			return nil, errors.New("SSH CA recovery archive contains an invalid file")
		}
		files[name] = content
	}
	return files, nil
}

func validateSSHCARecovery(files map[string][]byte) error {
	for _, name := range []string{"manifest.json", "user-ca", "user-ca.pub", "host-ca", "host-ca.pub"} {
		if len(files[name]) == 0 {
			return fmt.Errorf("SSH CA recovery archive is missing %s", name)
		}
	}
	var manifest sshCARecoveryManifest
	if err := json.Unmarshal(
		files["manifest.json"],
		&manifest,
	); err != nil ||
		manifest.Version != 1 {
		return errors.New("SSH CA recovery manifest is invalid")
	}
	createdAt, err := time.Parse(time.RFC3339, manifest.CreatedAt)
	if err != nil || createdAt.After(time.Now().UTC().Add(5*time.Minute)) {
		return errors.New("SSH CA recovery manifest creation time is invalid")
	}
	if err := validateRecoveredKeyPair(
		files["user-ca"],
		files["user-ca.pub"],
		manifest.UserCAFingerprint,
	); err != nil {
		return fmt.Errorf("validate recovered user CA: %w", err)
	}
	if err := validateRecoveredKeyPair(
		files["host-ca"],
		files["host-ca.pub"],
		manifest.HostCAFingerprint,
	); err != nil {
		return fmt.Errorf("validate recovered host CA: %w", err)
	}
	for name, trustedPath := range map[string]string{
		"user-ca.pub": "/etc/ssh/deploycrate-user-ca.pub",
		"host-ca.pub": "/etc/ssh/deploycrate-host-ca.pub",
	} {
		trusted, err := os.ReadFile(trustedPath)
		if err != nil {
			return fmt.Errorf("read trusted SSH CA public key: %w", err)
		}
		if !containsSSHFingerprint(trusted, sshFingerprint(files[name])) {
			return fmt.Errorf("recovered %s does not match the key already trusted by nodes", name)
		}
	}
	return nil
}

func validateRecoveredKeyPair(privateKey, publicKey []byte, expectedFingerprint string) error {
	signer, err := ssh.ParsePrivateKey(privateKey)
	if err != nil {
		return errors.New("private key is invalid")
	}
	public, _, _, remainder, err := ssh.ParseAuthorizedKey(publicKey)
	if err != nil || len(strings.TrimSpace(string(remainder))) != 0 {
		return errors.New("public key is invalid")
	}
	fingerprint := ssh.FingerprintSHA256(public)
	if fingerprint != expectedFingerprint ||
		ssh.FingerprintSHA256(signer.PublicKey()) != fingerprint {
		return errors.New("private key, public key, and manifest fingerprint do not match")
	}
	return nil
}

func sshFingerprint(publicKey []byte) string {
	key, _, _, _, err := ssh.ParseAuthorizedKey(publicKey)
	if err != nil {
		return ""
	}
	return ssh.FingerprintSHA256(key)
}

func containsSSHFingerprint(publicKeys []byte, expected string) bool {
	remaining := publicKeys
	for len(strings.TrimSpace(string(remaining))) > 0 {
		key, _, _, next, err := ssh.ParseAuthorizedKey(remaining)
		if err != nil {
			return false
		}
		if ssh.FingerprintSHA256(key) == expected {
			return true
		}
		remaining = next
	}
	return false
}
