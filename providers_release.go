//go:build !dev || production

package main

import "tobiro/internal/vpn"

func registerDevelopmentProviders(_ *vpn.Registry) {}
