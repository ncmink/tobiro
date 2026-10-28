package domain

// VPNProvider identifies which native VPN client vendor a profile is bound to.
//
// Tobiro never implements VPN protocols. Each provider value maps to an
// adapter that orchestrates the officially installed vendor client.
type VPNProvider string

const (
	ProviderForcepoint  VPNProvider = "forcepoint"
	ProviderIvanti      VPNProvider = "ivanti"
	ProviderFortiClient VPNProvider = "forticlient"
	ProviderTailscale   VPNProvider = "tailscale"
	ProviderSurfshark   VPNProvider = "surfshark"

	// ProviderMacOSVPN covers the VPN services configured in macOS System
	// Settings — L2TP over IPsec, IKEv2 and Cisco IPsec. macOS owns these, so
	// unlike every vendor client it can be connected and disconnected through a
	// documented interface.
	ProviderMacOSVPN VPNProvider = "macosvpn"

	// ProviderMock is retained only so older profiles and tests can still name
	// the value. It is not offered in the UI and no adapter registers it.
	ProviderMock VPNProvider = "mock"

	// ProviderSSH marks a profile as an SSH tunnel rather than a vendor client.
	//
	// It is deliberately absent from SupportedProviders: there is no adapter to
	// register, nothing to detect on the machine, and no native GUI to fall
	// back to. The Add Connection form offers SSH as a kind, and this value is
	// what the profile then carries so a stored record still says what it is.
	ProviderSSH VPNProvider = "ssh"
)

// VendorProfileField describes the vendor-side selector a provider connects by,
// such as a tunnel name already configured in the client or a macOS VPN service.
// An empty Label means the provider does not use one and the field is hidden.
type VendorProfileField struct {
	Label    string `json:"label"`
	Hint     string `json:"hint"`
	Required bool   `json:"required"`
}

// VendorProfileOption is one selector an adapter found already configured on
// this machine, so the form can offer real choices instead of a name the user
// would otherwise have to type from memory.
type VendorProfileOption struct {
	// Name is the value stored in VPNProfile.VendorProfile.
	Name string `json:"name"`

	// Detail describes what it is, e.g. "L2TP over IPsec".
	Detail string `json:"detail"`

	// State is the vendor's current word for it, when the adapter can read one.
	State string `json:"state"`

	Connected bool `json:"connected"`
}

// ProviderInfo describes a provider for presentation in the UI.
type ProviderInfo struct {
	ID          VPNProvider `json:"id"`
	DisplayName string      `json:"displayName"`
	Vendor      string      `json:"vendor"`
	DefaultPort int         `json:"defaultPort"`

	// RequiresEndpoint mirrors VPNProvider.RequiresEndpoint so the Add
	// Connection form can hide the gateway fields without hard-coding a
	// provider key of its own.
	RequiresEndpoint bool `json:"requiresEndpoint"`

	// EndpointNote explains what stands in for a gateway when there is none.
	EndpointNote string `json:"endpointNote"`

	// VendorProfileField describes the vendor-side selector, so the form can
	// label it for this provider instead of assuming "tunnel name".
	VendorProfileField VendorProfileField `json:"vendorProfileField"`
}

// SupportedProviders is the catalogue offered in the "Add Connection" form.
//
// Adding a new vendor (Cisco Secure Client, GlobalProtect, OpenVPN, ...) means
// appending an entry here and registering an adapter — the manager, services
// and UI need no changes.
func SupportedProviders() []ProviderInfo {
	providers := []ProviderInfo{
		{ID: ProviderForcepoint, DisplayName: "Forcepoint NGFW VPN Client", Vendor: "Forcepoint", DefaultPort: 443},
		{ID: ProviderIvanti, DisplayName: "Ivanti Secure Access Client", Vendor: "Ivanti", DefaultPort: 443},
		{ID: ProviderFortiClient, DisplayName: "FortiClient", Vendor: "Fortinet", DefaultPort: 443},
		{ID: ProviderTailscale, DisplayName: "Tailscale", Vendor: "Tailscale", DefaultPort: 41641},
		{ID: ProviderSurfshark, DisplayName: "Surfshark VPN", Vendor: "Surfshark", DefaultPort: 51820},
		{ID: ProviderMacOSVPN, DisplayName: "macOS VPN (L2TP/IPsec, IKEv2)", Vendor: "Apple", DefaultPort: 500},
	}
	for i := range providers {
		providers[i].RequiresEndpoint = providers[i].ID.RequiresEndpoint()
		providers[i].EndpointNote = providers[i].ID.EndpointNote()
		providers[i].VendorProfileField = providers[i].ID.VendorProfileField()
	}
	return providers
}

// IsKnownProvider reports whether the value is valid on a stored profile.
//
// Mock is kept for legacy records and tests but is not offered in
// SupportedProviders.
func IsKnownProvider(p VPNProvider) bool {
	if p == ProviderMock {
		return true
	}
	for _, info := range SupportedProviders() {
		if info.ID == p {
			return true
		}
	}
	return false
}

// DisplayName returns the human-readable vendor client name.
func (p VPNProvider) DisplayName() string {
	if p == ProviderSSH {
		return "SSH tunnel"
	}
	for _, info := range SupportedProviders() {
		if info.ID == p {
			return info.DisplayName
		}
	}
	return string(p)
}

// RequiresEndpoint reports whether a profile for this provider needs a gateway
// host:port.
//
// Corporate gateways are addressed by hostname, so the user has to supply one.
// Clients that own their own server list do not: a Tailscale connection is the
// machine's own mesh, a Surfshark connection is whichever location is chosen
// inside the Surfshark app, and a macOS VPN service already carries its server
// address in System Settings.
func (p VPNProvider) RequiresEndpoint() bool {
	switch p {
	case ProviderTailscale, ProviderSurfshark, ProviderMacOSVPN:
		return false
	default:
		return true
	}
}

// ManagesOwnAccount reports whether the vendor client holds the identity
// itself, leaving Tobiro no account to store.
//
// A Tailscale connection is this machine's own node, a Surfshark connection is a
// subscription kept signed in inside the Surfshark app, and a macOS VPN service
// keeps its password in the login Keychain that macOS itself reads. Corporate
// gateways are the opposite case — one gateway is reached with a different
// account per customer, which is why a profile owns a credential list at all.
func (p VPNProvider) ManagesOwnAccount() bool {
	switch p {
	case ProviderTailscale, ProviderSurfshark, ProviderMacOSVPN:
		return true
	default:
		return false
	}
}

// EndpointLabel is what to show where a gateway address would normally go, for
// providers that have none. It is empty when the provider has a real endpoint.
func (p VPNProvider) EndpointLabel() string {
	switch p {
	case ProviderTailscale:
		return "Tailscale mesh"
	case ProviderSurfshark:
		return "Surfshark locations"
	case ProviderMacOSVPN:
		return "macOS VPN service"
	default:
		return ""
	}
}

// EndpointNote explains in the Add Connection form why no gateway is asked for.
func (p VPNProvider) EndpointNote() string {
	switch p {
	case ProviderTailscale:
		return "Tailscale is a mesh on this machine — there is no gateway host. " +
			"Add one connection per customer tailnet and pick the Tailscale account below; " +
			"Connect switches to that account and brings the mesh up. " +
			"Join a new org tailnet once in the Tailscale app, then the account appears here."
	case ProviderSurfshark:
		return "Surfshark picks its own servers, so there is no gateway host. " +
			"Choose the location in the Surfshark app; Tobiro opens it for you " +
			"and reports whether the tunnel is up."
	case ProviderMacOSVPN:
		return "Pick a VPN you already set up in System Settings — its address and " +
			"password stay there. Connect it in System Settings too, because macOS will " +
			"not share its IPSec shared secret with an app; Tobiro then reports " +
			"the tunnel and can disconnect it."
	default:
		return ""
	}
}

// VendorProfileField describes the vendor-side selector this provider connects
// by. Most providers reach a gateway by address and do not use one.
func (p VPNProvider) VendorProfileField() VendorProfileField {
	switch p {
	case ProviderMacOSVPN:
		return VendorProfileField{
			Label:    "macOS VPN service",
			Hint:     "Name as it appears in System Settings › Network › VPN",
			Required: true,
		}
	case ProviderFortiClient:
		return VendorProfileField{
			Label: "Saved FortiClient connection",
			Hint:  "Connection name already configured in FortiClient",
		}
	case ProviderTailscale:
		return VendorProfileField{
			Label:    "Tailscale account",
			Hint:     "Pick a profile (nickname or tailnet). Login email works only if it is unique on this machine.",
			Required: true,
		}
	default:
		return VendorProfileField{}
	}
}
