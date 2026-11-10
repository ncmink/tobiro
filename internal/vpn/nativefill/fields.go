package nativefill

import (
	"sort"
	"strings"
)

// The login-field mapping below is pure geometry and text, so it carries no
// build tag: the Windows window code feeds it real controls, and the tests feed
// it a captured dialog layout.

type rect struct{ Left, Top, Right, Bottom int32 }

// control is one edit box or one static label, as the OS reports it.
type control struct {
	id   uintptr
	rect rect
	text string
}

type fieldKind int

const (
	kindUnknown fieldKind = iota
	kindUsername
	kindPassword
)

var (
	passwordLabels = []string{"password", "passcode", "รหัสผ่าน"}
	usernameLabels = []string{"user", "login", "account", "e-mail", "email", "ชื่อผู้ใช้"}
)

// classifyFields splits edit boxes into plain (username) and secret (password)
// fields, each ordered top to bottom.
//
// A field is secret when the control says so (isSecret: ES_PASSWORD or a
// password character). WinForms dialogs such as Forcepoint's "User
// Authentication" report neither, and expose every control as a plain pane, so
// the fields are classified by the label beside them ("User Name:",
// "Password:") using screen position, never tree order. As a last resort a bare
// pair of fields is read as username above password.
func classifyFields(edits, labels []control, isSecret func(control) bool) (plain, secret []control) {
	edits = append([]control(nil), edits...)
	sort.SliceStable(edits, func(i, j int) bool { return edits[i].rect.Top < edits[j].rect.Top })

	var unknown []control
	for _, e := range edits {
		if isSecret != nil && isSecret(e) {
			secret = append(secret, e)
			continue
		}
		switch labelKind(e, labels) {
		case kindPassword:
			secret = append(secret, e)
		case kindUsername:
			plain = append(plain, e)
		default:
			unknown = append(unknown, e)
		}
	}

	// Only read a bare pair as username-over-password in a window that labels a
	// password somewhere, so a settings window's two text boxes are not mistaken
	// for a login form.
	if len(secret) == 0 && len(plain) == 0 && len(unknown) == 2 && hasPasswordLabel(labels) {
		return []control{unknown[0]}, []control{unknown[1]}
	}
	return append(plain, unknown...), secret
}

// labelKind classifies an edit by the nearest label on its row to the left, or
// directly above it.
func labelKind(edit control, labels []control) fieldKind {
	best := -1
	bestDist := int32(1 << 30)
	editMid := (edit.rect.Top + edit.rect.Bottom) / 2
	for i, l := range labels {
		var dist int32
		switch {
		case l.rect.Right <= edit.rect.Left+8 && l.rect.Top <= editMid && l.rect.Bottom >= editMid:
			dist = edit.rect.Left - l.rect.Right
		case l.rect.Bottom <= edit.rect.Top+4 && l.rect.Left < edit.rect.Right && l.rect.Right > edit.rect.Left:
			dist = edit.rect.Top - l.rect.Bottom + 1000
		default:
			continue
		}
		if dist < bestDist {
			best, bestDist = i, dist
		}
	}
	if best < 0 {
		return kindUnknown
	}
	text := strings.ToLower(labels[best].text)
	switch {
	case containsAny(text, passwordLabels):
		return kindPassword
	case containsAny(text, usernameLabels):
		return kindUsername
	}
	return kindUnknown
}

func hasPasswordLabel(labels []control) bool {
	for _, l := range labels {
		if containsAny(strings.ToLower(l.text), passwordLabels) {
			return true
		}
	}
	return false
}

func containsAny(s string, words []string) bool {
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}
