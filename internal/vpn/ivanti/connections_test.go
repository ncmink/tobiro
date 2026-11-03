package ivanti

import (
	"testing"

	"tobiro/internal/domain"
)

const sampleConnstore = `;
; /Library/Application Support/Pulse Secure/Pulse/connstore.dat
;

ive "3042b6816f72403491771de564284237" {
  connection-source: "user"
  friendly-name: "GovBKK Mild"
  uri: "1.1.254.50"
}

ive "e2bd0eabc2aa4527b5ae9dbe2f3daaa8" {
  connection-source: "user"
  friendly-name: "EGP"
  uri: "1.1.254.50"
}

machine "local" {
  guid: "584570BB78BD4BFBAC56F08A4B45D494"
}

userdata "3042b6816f72403491771de564284237" {
}
`

func TestParseConnstore(t *testing.T) {
	got := parseConnstore(sampleConnstore)
	if len(got) != 2 {
		t.Fatalf("got %d connections, want 2: %+v", len(got), got)
	}
	if got[0].Name != "GovBKK Mild" || got[0].URI != "1.1.254.50" {
		t.Errorf("first = %+v", got[0])
	}
	if got[1].Name != "EGP" || got[1].ID != "e2bd0eabc2aa4527b5ae9dbe2f3daaa8" {
		t.Errorf("second = %+v", got[1])
	}
}

func TestActiveConnectionName(t *testing.T) {
	cases := []struct {
		name string
		log  string
		want string
	}{
		{"empty", "", ""},
		{
			"connected",
			"03/11/26 12:39:49 PM [Connection] Connection establishing: 'EGP' now connecting\n",
			"EGP",
		},
		{
			"torn down after terminated",
			"03/11/26 12:39:49 PM [Connection] Connection establishing: 'EGP' now connecting\n" +
				"03/11/26 12:40:41 PM [Connection] Connection terminating: 'EGP' now disconnecting\n" +
				"03/11/26 12:40:43 PM [Connection] Connection terminated: 'EGP'\n",
			"",
		},
		{
			"still up while terminating",
			"03/11/26 12:39:49 PM [Connection] Connection establishing: 'EGP' now connecting\n" +
				"03/11/26 12:40:41 PM [Connection] Connection terminating: 'EGP' now disconnecting\n",
			"EGP",
		},
		{
			"switched",
			"03/11/26 12:39:49 PM [Connection] Connection establishing: 'EGP' now connecting\n" +
				"03/11/26 12:40:43 PM [Connection] Connection terminated: 'EGP'\n" +
				"03/11/26 12:42:08 PM [Connection] Connection establishing: 'GovBKK Mild' now connecting\n",
			"GovBKK Mild",
		},
	}
	for _, tc := range cases {
		if got := activeConnectionName(tc.log); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestScopeToConnection(t *testing.T) {
	up := &domain.VPNStatus{State: domain.StateConnected, Known: true, AssignedIP: "10.168.6.139"}

	if got := scopeToConnection(up, "EGP", "EGP"); got.State != domain.StateConnected {
		t.Errorf("bound card on active connection should stay connected, got %s", got.State)
	}
	if got := scopeToConnection(up, "GovBKK Mild", "EGP"); got.State != domain.StateDisconnected || got.AssignedIP != "" {
		t.Errorf("bound card on other connection should be disconnected, got %+v", got)
	}
	if got := scopeToConnection(up, "", "EGP"); got.State != domain.StateConnected {
		t.Errorf("unbound card keeps gateway status, got %s", got.State)
	}
	if got := scopeToConnection(up, "EGP", ""); got.State != domain.StateConnected {
		t.Errorf("unknown active connection keeps gateway status, got %s", got.State)
	}
}
