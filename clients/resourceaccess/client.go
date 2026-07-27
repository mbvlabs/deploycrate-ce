package resourceaccess

import (
	"context"
	"fmt"

	"deploycrate-ce/internal/hostcommand"

	"github.com/google/uuid"
)

type Client struct{}

func New() Client { return Client{} }

func (Client) ApplyListener(ctx context.Context, resourceID uuid.UUID, wireGuardAddress string, wireGuardPort int32, originAddress string, originPort int32) error {
	_, err := hostcommand.Run(ctx, "listener-apply", resourceID.String(), wireGuardAddress, fmt.Sprint(wireGuardPort), originAddress, fmt.Sprint(originPort))
	return err
}

func (Client) RemoveListener(ctx context.Context, resourceID uuid.UUID) error {
	_, err := hostcommand.Run(ctx, "listener-remove", resourceID.String())
	return err
}
