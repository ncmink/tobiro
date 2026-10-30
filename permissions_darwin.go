//go:build darwin

package main

/*
#cgo LDFLAGS: -framework ApplicationServices
#include <ApplicationServices/ApplicationServices.h>
#include <stdbool.h>

bool tobiro_ax_trusted(void) {
	return AXIsProcessTrusted();
}
*/
import "C"

func accessibilityGranted() bool {
	return bool(C.tobiro_ax_trusted())
}
