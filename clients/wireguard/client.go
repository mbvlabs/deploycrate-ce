package wireguard

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"deploycrate-ce/internal/hostcommand"
)

type Client struct{}

func New() Client { return Client{} }

func (Client) ApplyPeer(ctx context.Context, publicKey, privateAddress string) error {
	_, err := hostcommand.Run(ctx, "peer-apply", strings.TrimSpace(publicKey), strings.TrimSpace(privateAddress))
	return err
}

func (Client) RemovePeer(ctx context.Context, publicKey string) error {
	_, err := hostcommand.Run(ctx, "peer-remove", strings.TrimSpace(publicKey))
	return err
}

func (Client) LatestHandshake(ctx context.Context, publicKey string) (time.Time, bool, error) {
	output, err := hostcommand.Run(ctx, "peer-handshake", strings.TrimSpace(publicKey))
	if err != nil {
		return time.Time{}, false, err
	}
	seconds, err := strconv.ParseInt(output, 10, 64)
	if err != nil || seconds < 0 {
		return time.Time{}, false, fmt.Errorf("parse WireGuard handshake timestamp %q", output)
	}
	if seconds == 0 {
		return time.Time{}, false, nil
	}
	return time.Unix(seconds, 0), true, nil
}
