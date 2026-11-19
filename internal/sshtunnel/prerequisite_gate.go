package sshtunnel

import (
	"fmt"

	"tobiro/internal/domain"
	"tobiro/internal/session"
)

type prerequisiteGate struct {
	profile  domain.VPNProfile
	profiles ProfileReader
	sessions *session.Manager
}

func newPrerequisiteGate(profile domain.VPNProfile, profiles ProfileReader, sessions *session.Manager) *prerequisiteGate {
	if len(profile.Prerequisites()) == 0 {
		return nil
	}
	return &prerequisiteGate{profile: profile, profiles: profiles, sessions: sessions}
}

func (g *prerequisiteGate) evaluate() (block bool, detail string) {
	if g == nil {
		return false, ""
	}
	waiting := ""
	for _, ref := range g.profile.Prerequisites() {
		needed, err := g.profiles.Get(ref)
		if err != nil {
			continue
		}
		sess, ok := g.sessions.Get(ref)
		if !ok || sess.State != domain.StateConnected {
			waiting = fmt.Sprintf("Waiting for %s", needed.Name)
			if ref == g.profile.JumpHost {
				return true, waiting
			}
		}
	}
	if waiting != "" {
		return false, waiting
	}
	return false, ""
}