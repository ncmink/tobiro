package vpn

import (
	"tobiro/internal/vpn/tailscale"
)

var _ VendorProfileLister = (*tailscale.Provider)(nil)
