//go:build !darwin

package ivanti

import (
	"context"

	"tobiro/internal/domain"
)

// DiagnoseNetwork has no verified route source off macOS.
func (p *Provider) DiagnoseNetwork(_ context.Context, _ domain.VPNProfile) (domain.NetworkDiagnosis, error) {
	return domain.NetworkDiagnosis{Known: false}, nil
}
