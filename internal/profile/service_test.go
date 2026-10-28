package profile

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"tobiro/internal/domain"
	"tobiro/internal/store"
)

func newService(t *testing.T) (*Service, *store.Store) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "data.json"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return NewService(s), s
}

func create(t *testing.T, svc *Service, name, group string) domain.VPNProfile {
	t.Helper()
	p, err := svc.Create(domain.ProfileInput{
		Name:     name,
		Provider: domain.ProviderMock,
		Group:    group,
		Host:     "vpn.example",
		Port:     443,
	})
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	return p
}

func groupFor(t *testing.T, svc *Service, name string) domain.GroupView {
	t.Helper()
	for _, group := range svc.Groups() {
		if group.Name == name {
			return group
		}
	}
	t.Fatalf("no group %q in %+v", name, svc.Groups())
	return domain.GroupView{}
}

func named(groups []domain.GroupView) []domain.GroupView {
	out := make([]domain.GroupView, 0, len(groups))
	for _, group := range groups {
		if group.Name != domain.ArchivedGroup || group.Connections > 0 {
			out = append(out, group)
		}
	}
	return out
}

func TestGroupsCountConnectionsAndPutUngroupedLast(t *testing.T) {
	svc, _ := newService(t)
	create(t, svc, "SMOI", " Acme  ")
	create(t, svc, "Acme staging", "Acme")
	create(t, svc, "Home lab", "")
	create(t, svc, "Beta", "Zeta Co")

	groups := named(svc.Groups())
	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %d: %+v", len(groups), groups)
	}
	if groups[0].Name != "Acme" || groups[0].Connections != 2 {
		t.Fatalf("expected Acme with 2 connections first, got %+v", groups[0])
	}
	if groups[1].Name != "Zeta Co" {
		t.Fatalf("expected Zeta Co second, got %+v", groups[1])
	}
	if groups[2].Name != "" || groups[2].Label != domain.UngroupedLabel {
		t.Fatalf("expected ungrouped last labelled %q, got %+v", domain.UngroupedLabel, groups[2])
	}
}

func TestHidingAGroupHidesEveryConnectionInIt(t *testing.T) {
	svc, _ := newService(t)
	create(t, svc, "SMOI", "Acme")
	create(t, svc, "Acme staging", "Acme")
	create(t, svc, "Home lab", "")

	if err := svc.SetGroupHidden("Acme", true); err != nil {
		t.Fatalf("hide group: %v", err)
	}
	if !svc.GroupHidden("Acme") {
		t.Fatal("expected Acme to be hidden")
	}
	if svc.GroupHidden("") {
		t.Fatal("hiding one group must not hide the ungrouped connections")
	}

	for _, g := range svc.Groups() {
		if g.Name == "Acme" && !g.Hidden {
			t.Fatal("expected the group list to report Acme as hidden")
		}
	}
}

func TestShowingAGroupForgetsTheHiddenFlag(t *testing.T) {
	svc, dataStore := newService(t)
	create(t, svc, "SMOI", "Acme")

	if err := svc.SetGroupHidden("Acme", true); err != nil {
		t.Fatalf("hide group: %v", err)
	}
	if err := svc.SetGroupHidden("Acme", false); err != nil {
		t.Fatalf("show group: %v", err)
	}
	if svc.GroupHidden("Acme") {
		t.Fatal("expected Acme to be visible again")
	}

	dataStore.Read(func(d *store.Data) {
		if _, ok := d.Groups["Acme"]; ok {
			t.Fatal("a visible group should not be recorded in application data")
		}
	})
}

// A hidden flag left behind by an emptied group would silently hide the next
// connection that happens to reuse the name.
func TestMovingTheLastConnectionOutOfAGroupForgetsIt(t *testing.T) {
	svc, _ := newService(t)
	p := create(t, svc, "SMOI", "Acme")
	if err := svc.SetGroupHidden("Acme", true); err != nil {
		t.Fatalf("hide group: %v", err)
	}

	if _, err := svc.Update(p.ID, domain.ProfileInput{
		Name:     "SMOI",
		Provider: domain.ProviderMock,
		Group:    "Other Co",
		Host:     "vpn.example",
		Port:     443,
	}); err != nil {
		t.Fatalf("update profile: %v", err)
	}

	if svc.GroupHidden("Acme") {
		t.Fatal("expected the emptied group to lose its hidden flag")
	}
	create(t, svc, "Acme again", "Acme")
	if svc.GroupHidden("Acme") {
		t.Fatal("a reused group name must start visible")
	}
}

func TestDeletingTheLastConnectionInAGroupForgetsIt(t *testing.T) {
	svc, _ := newService(t)
	p := create(t, svc, "SMOI", "Acme")
	if err := svc.SetGroupHidden("Acme", true); err != nil {
		t.Fatalf("hide group: %v", err)
	}
	if err := svc.Delete(p.ID); err != nil {
		t.Fatalf("delete profile: %v", err)
	}
	if svc.GroupHidden("Acme") {
		t.Fatal("expected the deleted group to lose its hidden flag")
	}
}

func TestSetGroupHiddenRejectsAGroupNothingUses(t *testing.T) {
	svc, _ := newService(t)
	create(t, svc, "SMOI", "Acme")

	err := svc.SetGroupHidden("Nowhere", true)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// Hiding the ungrouped connections would hide the section that offers to
// reveal them, and every connection created without a group lands there.
func TestTheUngroupedConnectionsCannotBeHidden(t *testing.T) {
	svc, dataStore := newService(t)
	create(t, svc, "Home lab", "")

	if err := svc.SetGroupHidden("", true); err == nil {
		t.Fatal("expected hiding the ungrouped connections to be refused")
	}
	if svc.GroupHidden("") {
		t.Fatal("expected the ungrouped connections to stay visible")
	}

	// A flag written by an older build must not strand them either.
	if err := dataStore.Update(func(d *store.Data) error {
		d.Groups[""] = domain.GroupSettings{Hidden: true}
		return nil
	}); err != nil {
		t.Fatalf("write legacy flag: %v", err)
	}
	if svc.GroupHidden("") || groupFor(t, svc, "").Hidden {
		t.Fatal("expected a stored flag on the ungrouped connections to be ignored")
	}
}

func TestArchiveIsAlwaysOfferedAndStartsHidden(t *testing.T) {
	svc, dataStore := newService(t)
	create(t, svc, "SMOI", "Acme")

	archive := groupFor(t, svc, domain.ArchivedGroup)
	if !archive.Hidden || archive.Connections != 0 {
		t.Fatalf("expected an empty, hidden archive, got %+v", archive)
	}
	if !svc.GroupHidden(domain.ArchivedGroup) {
		t.Fatal("expected the archive to be hidden without anyone choosing")
	}
	dataStore.Read(func(d *store.Data) {
		if len(d.Groups) != 0 {
			t.Fatalf("a default needs no record on disk, got %+v", d.Groups)
		}
	})
}

// Hidden is the archive's default, so it is revealing it that has to be
// remembered — the opposite of every other group.
func TestRevealingTheArchiveIsRemembered(t *testing.T) {
	svc, dataStore := newService(t)

	if err := svc.SetGroupHidden(domain.ArchivedGroup, false); err != nil {
		t.Fatalf("reveal archive: %v", err)
	}
	if svc.GroupHidden(domain.ArchivedGroup) {
		t.Fatal("expected the archive to be visible")
	}
	dataStore.Read(func(d *store.Data) {
		if settings, ok := d.Groups[domain.ArchivedGroup]; !ok || settings.Hidden {
			t.Fatalf("expected a stored hidden=false for the archive, got %+v", d.Groups)
		}
	})

	if err := svc.SetGroupHidden(domain.ArchivedGroup, true); err != nil {
		t.Fatalf("hide archive: %v", err)
	}
	dataStore.Read(func(d *store.Data) {
		if _, ok := d.Groups[domain.ArchivedGroup]; ok {
			t.Fatalf("back at the default, the record should be gone, got %+v", d.Groups)
		}
	})
}

func TestRevealedArchiveSurvivesItsLastConnectionLeaving(t *testing.T) {
	svc, _ := newService(t)
	p := create(t, svc, "SMOI", domain.ArchivedGroup)
	if err := svc.SetGroupHidden(domain.ArchivedGroup, false); err != nil {
		t.Fatalf("reveal archive: %v", err)
	}

	if _, err := svc.SetGroup(p.ID, "Acme"); err != nil {
		t.Fatalf("move profile: %v", err)
	}
	if svc.GroupHidden(domain.ArchivedGroup) {
		t.Fatal("expected the revealed archive to stay revealed once emptied")
	}
}

func TestSetGroupMovesAConnectionAndAcceptsTheArchiveInAnyCase(t *testing.T) {
	svc, _ := newService(t)
	p := create(t, svc, "SMOI", "Acme")

	moved, err := svc.SetGroup(p.ID, "  archived ")
	if err != nil {
		t.Fatalf("move profile: %v", err)
	}
	if moved.Group != domain.ArchivedGroup {
		t.Fatalf("expected the archive, got %q", moved.Group)
	}
	if got := groupFor(t, svc, domain.ArchivedGroup); got.Connections != 1 || !got.Hidden {
		t.Fatalf("expected one hidden archived connection, got %+v", got)
	}

	// The group it left is empty now, so nothing should remember it.
	for _, group := range svc.Groups() {
		if group.Name == "Acme" {
			t.Fatalf("expected Acme to be gone, got %+v", group)
		}
	}

	cleared, err := svc.SetGroup(p.ID, "")
	if err != nil {
		t.Fatalf("clear group: %v", err)
	}
	if cleared.Group != "" {
		t.Fatalf("expected an ungrouped profile, got %q", cleared.Group)
	}
}

func TestSetGroupRejectsAnUnknownProfile(t *testing.T) {
	svc, _ := newService(t)
	if _, err := svc.SetGroup("nope", "Acme"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGroupNameIsTrimmedAndLengthChecked(t *testing.T) {
	svc, _ := newService(t)
	p := create(t, svc, "SMOI", "  Acme  ")
	if p.Group != "Acme" {
		t.Fatalf("expected the group name to be trimmed, got %q", p.Group)
	}

	_, err := svc.Create(domain.ProfileInput{
		Name:     "Too long",
		Provider: domain.ProviderMock,
		Group:    strings.Repeat("a", 65),
		Host:     "vpn.example",
		Port:     443,
	})
	if err == nil {
		t.Fatal("expected an over-long group name to be rejected")
	}
}

// Names are what another biebie.net application can actually refer to a
// connection by, so two connections answering to one would make that reference
// ambiguous. Case and surrounding space do not make a name distinct.
func TestAConnectionNameCannotBeClaimedTwice(t *testing.T) {
	svc, _ := newService(t)
	create(t, svc, "SMOI", "")

	for _, name := range []string{"SMOI", "smoi", "  SMOI  "} {
		_, err := svc.Create(domain.ProfileInput{
			Name: name, Provider: domain.ProviderMock, Host: "vpn.example", Port: 443,
		})
		if err == nil {
			t.Fatalf("a second connection called %q was allowed", name)
		}
	}

	if got := len(svc.List()); got != 1 {
		t.Fatalf("connections = %d, want the one that was allowed", got)
	}
}

func TestRenamingOntoAnotherNameIsRefusedButKeepingYourOwnIsNot(t *testing.T) {
	svc, _ := newService(t)
	first := create(t, svc, "SMOI", "")
	second := create(t, svc, "GovBKK", "")

	edit := domain.ProfileInput{
		Name: "smoi", Provider: domain.ProviderMock, Host: "vpn.example", Port: 443,
	}
	if _, err := svc.Update(second.ID, edit); err == nil {
		t.Fatal("renaming onto a name another connection holds must be refused")
	}

	// A connection is not in its own way: saving it unchanged has to work, or
	// every other edit to it becomes impossible.
	edit.Name = "SMOI"
	edit.Host = "vpn2.example"
	if _, err := svc.Update(first.ID, edit); err != nil {
		t.Fatalf("editing a connection while keeping its own name: %v", err)
	}
}

func TestResolveTakesAnIdentifierOrAName(t *testing.T) {
	svc, _ := newService(t)
	stored := create(t, svc, "vpn-cat", "Archived")

	for _, ref := range []string{stored.ID, "vpn-cat", "VPN-CAT", "  vpn-cat "} {
		found, err := svc.Resolve(ref)
		if err != nil {
			t.Fatalf("resolve %q: %v", ref, err)
		}
		if found.ID != stored.ID {
			t.Fatalf("resolve %q = %s, want %s", ref, found.ID, stored.ID)
		}
	}

	if _, err := svc.Resolve("no-such-connection"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
	if _, err := svc.Resolve(""); err == nil {
		t.Fatal("an empty reference must be refused")
	}
}

// An identifier must never be shadowed by a name that happens to equal it,
// because the identifier is the unambiguous one of the two.
func TestAnIdentifierWinsOverAConnectionNamedAfterIt(t *testing.T) {
	svc, _ := newService(t)
	target := create(t, svc, "real", "")
	create(t, svc, target.ID, "")

	found, err := svc.Resolve(target.ID)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if found.ID != target.ID {
		t.Fatalf("resolved to %q, want the connection holding that id", found.Name)
	}
}
