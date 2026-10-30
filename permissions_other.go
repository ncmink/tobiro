//go:build !darwin

package main

func accessibilityGranted() bool { return false }
