package openvpn

import "tobiro/internal/vpn"

var (
	_ vpn.VPNProviderAdapter = (*Provider)(nil)
	_ vpn.VendorProfileLister = (*Provider)(nil)
)