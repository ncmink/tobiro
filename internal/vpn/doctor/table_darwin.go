//go:build darwin

package doctor

import (
	"context"

	"tobiro/internal/sysexec"
)

const netstat = "/usr/sbin/netstat"

// ReadInetTable returns the live IPv4 routing table.
func ReadInetTable(ctx context.Context) ([]InetRoute, error) {
	res, err := sysexec.Run(ctx, netstat, "-rn", "-f", "inet")
	if err != nil {
		return nil, err
	}
	return ParseNetstatInet(res.Stdout), nil
}
