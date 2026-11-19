package vpn

import "tobiro/internal/vpn/wireguard"

var _ VendorProfileLister = (*wireguard.Provider)(nil)
