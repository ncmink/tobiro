module tobiro

go 1.25.0

require (
	github.com/google/uuid v1.6.0
	github.com/ncmink/biebie-protocol v0.0.0-00010101000000-000000000000
	github.com/wailsapp/wails/v3 v3.0.0-beta.9
	github.com/zalando/go-keyring v0.2.8
	golang.design/x/hotkey v0.6.4
	golang.org/x/crypto v0.53.0
	golang.org/x/sys v0.46.0
	gopkg.in/yaml.v3 v3.0.1
)

// The Biebie Context Protocol is the contract this application shares with
// Biebie Kube, and it is developed alongside them rather than published: the
// three repositories sit side by side in one checkout. Both applications point
// here, so a change to the wire format is a compile error in both at once
// instead of a mismatch discovered at runtime.
replace github.com/ncmink/biebie-protocol => ../biebie-protocol

require (
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/adrg/xdg v0.5.3 // indirect
	github.com/coder/websocket v1.8.14 // indirect
	github.com/danieljoos/wincred v1.2.3 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/jchv/go-winloader v0.0.0-20250406163304-c1995be93bd1 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/niemeyer/pretty v0.0.0-20200227124842-a10e7caefd8e // indirect
	golang.org/x/mod v0.37.0 // indirect
	gopkg.in/check.v1 v1.0.0-20200227125254-8fa46927fb4f // indirect
)

// replace github.com/wailsapp/wails/v2 v2.15.0 => /Users/napisoot/go/pkg/mod
