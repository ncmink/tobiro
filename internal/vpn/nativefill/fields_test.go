package nativefill

import "testing"

func ctl(id uintptr, l, t, r, b int32, text string) control {
	return control{id: id, rect: rect{Left: l, Top: t, Right: r, Bottom: b}, text: text}
}

func ids(cs []control) []uintptr {
	out := make([]uintptr, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.id)
	}
	return out
}

func only(t *testing.T, what string, cs []control, want uintptr) {
	t.Helper()
	if len(cs) != 1 || cs[0].id != want {
		t.Fatalf("%s = %v, want [%d]", what, ids(cs), want)
	}
}

func neverSecret(control) bool { return false }

// Forcepoint's "User Authentication" dialog (a WinForms app): the two edits
// share a class, neither carries ES_PASSWORD, and the password edit is listed
// before the username edit in the control tree. Only the label beside each box
// says which is which.
func TestForcepointDialogFieldsAreMappedByLabelNotTreeOrder(t *testing.T) {
	// Enumeration order: password edit first, then username edit.
	password := ctl(11, 120, 60, 300, 82, "")
	username := ctl(12, 120, 20, 300, 42, "smoi01")
	edits := []control{password, username}
	labels := []control{
		ctl(21, 10, 24, 100, 40, "User Name:"),
		ctl(22, 10, 64, 100, 80, "Password:"),
	}

	plain, secret := classifyFields(edits, labels, neverSecret)

	only(t, "plain", plain, 12)
	only(t, "secret", secret, 11)
}

// The mapping follows screen position: swapping where the boxes sit swaps the
// answer even though the tree order is unchanged.
func TestFieldMappingFollowsScreenPosition(t *testing.T) {
	first := ctl(1, 120, 60, 300, 82, "")  // listed first, drawn lower
	second := ctl(2, 120, 20, 300, 42, "") // listed second, drawn higher
	labels := []control{
		ctl(21, 10, 24, 100, 40, "Password:"),
		ctl(22, 10, 64, 100, 80, "User Name:"),
	}

	plain, secret := classifyFields([]control{first, second}, labels, neverSecret)

	only(t, "plain", plain, 1)
	only(t, "secret", secret, 2)
}

func TestLabelAboveTheFieldIsUsed(t *testing.T) {
	user := ctl(1, 10, 30, 200, 52, "")
	pass := ctl(2, 10, 100, 200, 122, "")
	labels := []control{
		ctl(21, 10, 10, 120, 28, "Username"),
		ctl(22, 10, 80, 120, 98, "Password"),
	}

	plain, secret := classifyFields([]control{user, pass}, labels, neverSecret)

	only(t, "plain", plain, 1)
	only(t, "secret", secret, 2)
}

// A control that reports itself as a password edit wins over its label.
func TestPasswordStyleWinsOverLabels(t *testing.T) {
	a := ctl(1, 120, 20, 300, 42, "")
	b := ctl(2, 120, 60, 300, 82, "")
	labels := []control{
		ctl(21, 10, 24, 100, 40, "User Name:"),
		ctl(22, 10, 64, 100, 80, "Password:"),
	}

	plain, secret := classifyFields([]control{a, b}, labels, func(c control) bool { return c.id == 2 })

	only(t, "plain", plain, 1)
	only(t, "secret", secret, 2)
}

// Two unlabelled boxes are a login form only when the window labels a password
// somewhere; otherwise they are some settings page's text fields.
func TestBarePairNeedsAPasswordLabel(t *testing.T) {
	top := ctl(1, 120, 20, 300, 42, "")
	bottom := ctl(2, 120, 60, 300, 82, "")

	plain, secret := classifyFields([]control{bottom, top}, []control{ctl(9, 400, 400, 500, 420, "Enter Password")}, neverSecret)
	only(t, "plain", plain, 1)
	only(t, "secret", secret, 2)

	plain, secret = classifyFields([]control{top, bottom}, nil, neverSecret)
	if len(secret) != 0 || len(plain) != 2 {
		t.Fatalf("with no password label nothing may be called a password: plain=%v secret=%v", ids(plain), ids(secret))
	}
}
