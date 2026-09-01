package ssh

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	cryptossh "golang.org/x/crypto/ssh"
)

const maximumCommandOutput = 2 << 20

var errHostKeyCaptured = errors.New("SSH host key captured")

type Client struct {
	timeout time.Duration
}

type Credentials struct {
	Username   string
	PrivateKey []byte
	Passphrase []byte
	HostKey    string
}

type Result struct {
	Stdout string
	Stderr string
}

func New() Client {
	return Client{timeout: 15 * time.Second}
}

func (client Client) ProbeHostKey(ctx context.Context, address string) (string, string, error) {
	connection, err := (&net.Dialer{Timeout: client.timeout}).DialContext(ctx, "tcp", address)
	if err != nil {
		return "", "", fmt.Errorf("connect to SSH server: %w", err)
	}
	defer connection.Close()

	var captured cryptossh.PublicKey
	configuration := &cryptossh.ClientConfig{
		User: "deploycrate-host-key-probe",
		Auth: []cryptossh.AuthMethod{cryptossh.Password("deploycrate-host-key-probe")},
		HostKeyCallback: func(_ string, _ net.Addr, key cryptossh.PublicKey) error {
			captured = key
			return errHostKeyCaptured
		},
		Timeout: client.timeout,
	}
	_, _, _, handshakeErr := client.handshake(ctx, connection, address, configuration)
	if captured == nil {
		return "", "", fmt.Errorf("read SSH host key: %w", handshakeErr)
	}
	return strings.TrimSpace(
			string(cryptossh.MarshalAuthorizedKey(captured)),
		), cryptossh.FingerprintSHA256(
			captured,
		), nil
}

func (client Client) Run(
	ctx context.Context,
	address string,
	credentials Credentials,
	command string,
	stdin io.Reader,
) (Result, error) {
	authentication, err := privateKeyAuthentication(credentials.PrivateKey, credentials.Passphrase)
	if err != nil {
		return Result{}, err
	}
	return client.run(
		ctx,
		address,
		credentials.Username,
		credentials.HostKey,
		authentication,
		command,
		stdin,
	)
}

func (client Client) RunWithCertificate(
	ctx context.Context,
	address, username, hostKey string,
	privateKey, certificate []byte,
	command string,
	stdin io.Reader,
) (Result, error) {
	authentication, err := certificateAuthentication(privateKey, certificate)
	if err != nil {
		return Result{}, err
	}
	return client.run(ctx, address, username, hostKey, authentication, command, stdin)
}

func (client Client) RunWithCertificateStreaming(
	ctx context.Context,
	address, username, hostKey string,
	privateKey, certificate []byte,
	command string,
	stdin io.Reader,
	stdout, stderr io.Writer,
) error {
	authentication, err := certificateAuthentication(privateKey, certificate)
	if err != nil {
		return err
	}
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	return client.runStreaming(
		ctx,
		address,
		username,
		hostKey,
		authentication,
		command,
		stdin,
		stdout,
		stderr,
	)
}

func certificateAuthentication(privateKey, certificate []byte) (cryptossh.AuthMethod, error) {
	signer, err := cryptossh.ParsePrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("parse ephemeral SSH private key: %w", err)
	}
	publicKey, _, _, remainder, err := cryptossh.ParseAuthorizedKey(certificate)
	if err != nil || len(bytes.TrimSpace(remainder)) != 0 {
		return nil, errors.New("parse SSH user certificate")
	}
	sshCertificate, ok := publicKey.(*cryptossh.Certificate)
	if !ok {
		return nil, errors.New("SSH user certificate has the wrong key type")
	}
	certificateSigner, err := cryptossh.NewCertSigner(sshCertificate, signer)
	if err != nil {
		return nil, fmt.Errorf("create SSH certificate signer: %w", err)
	}
	return cryptossh.PublicKeys(certificateSigner), nil
}

func (client Client) run(
	ctx context.Context,
	address, username, hostKey string,
	authentication cryptossh.AuthMethod,
	command string,
	stdin io.Reader,
) (Result, error) {
	stdout := &limitedBuffer{remaining: maximumCommandOutput}
	stderr := &limitedBuffer{remaining: maximumCommandOutput}
	if err := client.runStreaming(
		ctx,
		address,
		username,
		hostKey,
		authentication,
		command,
		stdin,
		stdout,
		stderr,
	); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return Result{
				Stdout: stdout.String(),
				Stderr: stderr.String(),
			}, fmt.Errorf(
				"run SSH command: %s",
				message,
			)
	}
	return Result{Stdout: stdout.String(), Stderr: stderr.String()}, nil
}

func (client Client) runStreaming(
	ctx context.Context,
	address, username, hostKey string,
	authentication cryptossh.AuthMethod,
	command string,
	stdin io.Reader,
	stdout, stderr io.Writer,
) error {
	expectedKey, _, _, remainder, err := cryptossh.ParseAuthorizedKey([]byte(hostKey))
	if err != nil || len(bytes.TrimSpace(remainder)) != 0 {
		return errors.New("pinned SSH host key is invalid")
	}
	configuration := &cryptossh.ClientConfig{
		User: username,
		Auth: []cryptossh.AuthMethod{authentication},
		HostKeyCallback: func(_ string, _ net.Addr, actual cryptossh.PublicKey) error {
			if !bytes.Equal(expectedKey.Marshal(), actual.Marshal()) {
				return errors.New("SSH host key does not match the confirmed key")
			}
			return nil
		},
		Timeout: client.timeout,
	}
	connection, err := (&net.Dialer{Timeout: client.timeout}).DialContext(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("connect to SSH server: %w", err)
	}
	clientConnection, channels, requests, err := client.handshake(
		ctx,
		connection,
		address,
		configuration,
	)
	if err != nil {
		connection.Close()
		return fmt.Errorf("authenticate to SSH server: %w", err)
	}
	sshClient := cryptossh.NewClient(clientConnection, channels, requests)
	defer sshClient.Close()
	finished := make(chan struct{})
	defer close(finished)
	go func() {
		select {
		case <-ctx.Done():
			_ = sshClient.Close()
		case <-finished:
		}
	}()
	session, err := sshClient.NewSession()
	if err != nil {
		return fmt.Errorf("create SSH session: %w", err)
	}
	defer session.Close()
	if stdin != nil {
		session.Stdin = stdin
	}
	session.Stdout = stdout
	session.Stderr = stderr
	return session.Run(command)
}

func (client Client) handshake(
	ctx context.Context,
	connection net.Conn,
	address string,
	configuration *cryptossh.ClientConfig,
) (cryptossh.Conn, <-chan cryptossh.NewChannel, <-chan *cryptossh.Request, error) {
	deadline := time.Now().Add(client.timeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := connection.SetDeadline(deadline); err != nil {
		return nil, nil, nil, fmt.Errorf("set SSH handshake deadline: %w", err)
	}
	finished := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = connection.Close()
		case <-finished:
		}
	}()
	clientConnection, channels, requests, err := cryptossh.NewClientConn(
		connection,
		address,
		configuration,
	)
	close(finished)
	if err != nil {
		if ctx.Err() != nil {
			return nil, nil, nil, ctx.Err()
		}
		return nil, nil, nil, err
	}
	if err := connection.SetDeadline(time.Time{}); err != nil {
		_ = clientConnection.Close()
		return nil, nil, nil, fmt.Errorf("clear SSH handshake deadline: %w", err)
	}
	return clientConnection, channels, requests, nil
}

func privateKeyAuthentication(privateKey, passphrase []byte) (cryptossh.AuthMethod, error) {
	var signer cryptossh.Signer
	var err error
	if len(passphrase) == 0 {
		signer, err = cryptossh.ParsePrivateKey(privateKey)
	} else {
		signer, err = cryptossh.ParsePrivateKeyWithPassphrase(privateKey, passphrase)
	}
	if err != nil {
		return nil, fmt.Errorf("parse SSH private key: %w", err)
	}
	return cryptossh.PublicKeys(signer), nil
}

type limitedBuffer struct {
	value     bytes.Buffer
	remaining int
}

func (buffer *limitedBuffer) Write(value []byte) (int, error) {
	written := len(value)
	if buffer.remaining > 0 {
		kept := min(len(value), buffer.remaining)
		_, _ = buffer.value.Write(value[:kept])
		buffer.remaining -= kept
	}
	return written, nil
}

func (buffer *limitedBuffer) String() string { return buffer.value.String() }
