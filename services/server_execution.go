package services

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	sshclient "deploycrate-ce/clients/ssh"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"

	"github.com/google/uuid"
)

type ServerExecutionTarget struct {
	Server     models.ServerEntity
	Credential models.ServerSSHCredentialEntity
	Peer       models.WireGuardPeerEntity
	Remote     bool
}

type ServerExecution struct {
	db    storage.Pool
	ssh   sshclient.Client
	sshCA SSHCAService
}

func NewServerExecution(db storage.Pool, sshCA SSHCAService) *ServerExecution {
	return &ServerExecution{db: db, ssh: sshclient.New(), sshCA: sshCA}
}

func (service *ServerExecution) Target(
	ctx context.Context,
	serverID uuid.UUID,
	capability models.ServerCapability,
) (ServerExecutionTarget, error) {
	server, err := models.RequireServerCapability(ctx, service.db.Executor(), serverID, capability)
	if err != nil {
		return ServerExecutionTarget{}, err
	}
	target := ServerExecutionTarget{Server: server, Remote: server.Kind == "worker"}
	if !target.Remote {
		return target, nil
	}
	target.Credential, err = models.ServerSSHCredential.FindForServer(
		ctx,
		service.db.Executor(),
		server.ID,
	)
	if err != nil || !target.Credential.HostKeyConfirmedAt.Valid ||
		strings.TrimSpace(target.Credential.KnownHostKey) == "" {
		return ServerExecutionTarget{}, errors.New("selected Server has no trusted SSH identity")
	}
	target.Peer, err = models.WireGuardPeer.FindActiveForServer(
		ctx,
		service.db.Executor(),
		server.ID,
	)
	if err != nil {
		return ServerExecutionTarget{}, errors.New("selected Server has no active WireGuard peer")
	}
	return target, nil
}

func (service *ServerExecution) RunRootScript(
	ctx context.Context,
	target ServerExecutionTarget,
	script []byte,
) (sshclient.Result, error) {
	return service.run(ctx, target, "sudo -n /bin/bash -s", bytes.NewReader(script))
}

func (service *ServerExecution) RunRootCommand(
	ctx context.Context,
	target ServerExecutionTarget,
	input io.Reader,
	executable string,
	arguments ...string,
) (sshclient.Result, error) {
	return service.run(ctx, target, "sudo -n -- "+shellJoin(executable, arguments...), input)
}

func (service *ServerExecution) RunRootCommandStreaming(
	ctx context.Context,
	target ServerExecutionTarget,
	input io.Reader,
	output io.Writer,
	executable string,
	arguments ...string,
) error {
	if !target.Remote {
		return errors.New("SSH execution requires a worker Server")
	}
	certificate, err := service.sshCA.GenerateUserCertificate(5 * time.Minute)
	if err != nil {
		return err
	}
	address := net.JoinHostPort(
		target.Peer.PrivateAddress,
		strconv.Itoa(int(target.Credential.Port)),
	)
	var stderr bytes.Buffer
	err = service.ssh.RunWithCertificateStreaming(
		ctx,
		address,
		"admin",
		target.Credential.KnownHostKey,
		certificate.PrivateKey,
		certificate.Certificate,
		"sudo -n -- "+shellJoin(executable, arguments...),
		input,
		output,
		&stderr,
	)
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return errors.New(message)
	}
	return nil
}

func (service *ServerExecution) run(
	ctx context.Context,
	target ServerExecutionTarget,
	command string,
	input io.Reader,
) (sshclient.Result, error) {
	if !target.Remote {
		return sshclient.Result{}, errors.New("SSH execution requires a worker Server")
	}
	certificate, err := service.sshCA.GenerateUserCertificate(5 * time.Minute)
	if err != nil {
		return sshclient.Result{}, err
	}
	address := net.JoinHostPort(
		target.Peer.PrivateAddress,
		strconv.Itoa(int(target.Credential.Port)),
	)
	return service.ssh.RunWithCertificate(
		ctx,
		address,
		"admin",
		target.Credential.KnownHostKey,
		certificate.PrivateKey,
		certificate.Certificate,
		command,
		input,
	)
}
