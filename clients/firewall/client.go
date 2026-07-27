package firewall

import (
	"context"
	"fmt"

	"deploycrate-ce/internal/hostcommand"

	"github.com/google/uuid"
)

type Client struct{}

func New() Client { return Client{} }

func (Client) ApplyRule(ctx context.Context, grantID uuid.UUID, sourceAddress string, port int32) error {
	_, err := hostcommand.Run(ctx, "firewall-apply", grantID.String(), sourceAddress, fmt.Sprint(port), "tcp")
	return err
}

func (Client) RemoveRule(ctx context.Context, grantID uuid.UUID, sourceAddress string, port int32) error {
	_, err := hostcommand.Run(ctx, "firewall-remove", grantID.String(), sourceAddress, fmt.Sprint(port), "tcp")
	return err
}
