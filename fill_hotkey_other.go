//go:build !darwin

package main

func (a *App) startFillHotkey()  {}
func (a *App) restartFillHotkey(string) error { return nil }
func (a *App) stopFillHotkey()   {}
