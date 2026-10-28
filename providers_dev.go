//go:build dev && !production

package main

import (
	"tobiro/internal/vpn"
	"tobiro/internal/vpn/mock"
)

func registerDevelopmentProviders(registry *vpn.Registry) {
	registry.Register(mock.New())
}
