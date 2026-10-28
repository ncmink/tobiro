// Package profile implements VPN connection profile use cases.
package profile

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"tobiro/internal/domain"
	"tobiro/internal/store"
)

// Service manages VPN connection profiles.
type Service struct {
	store *store.Store
}

// NewService creates the profile service.
func NewService(s *store.Store) *Service { return &Service{store: s} }

// List returns all profiles, newest configuration first by name.
func (s *Service) List() []domain.VPNProfile {
	var out []domain.VPNProfile
	s.store.Read(func(d *store.Data) {
		out = make([]domain.VPNProfile, len(d.Profiles))
		copy(out, d.Profiles)
	})
	sort.SliceStable(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// Get returns one profile by ID.
func (s *Service) Get(id string) (domain.VPNProfile, error) {
	var (
		found domain.VPNProfile
		ok    bool
	)
	s.store.Read(func(d *store.Data) {
		for _, p := range d.Profiles {
			if p.ID == id {
				found, ok = p, true
				return
			}
		}
	})
	if !ok {
		return domain.VPNProfile{}, fmt.Errorf("profile %s: %w", id, domain.ErrNotFound)
	}
	return found, nil
}

// Resolve finds a profile by identifier or by name.
//
// It exists for references arriving from another biebie.net application, which
// carry whatever the engineer configured over there. An identifier is a UUID
// nobody types on purpose, so a name is what they actually have to hand — and
// names are unique here precisely so this can be unambiguous.
//
// Identifiers are tried first, so an exact one is never shadowed by a name that
// happens to match it. A store written before names were kept unique may still
// hold a pair, and saying so is better than picking one of them at random.
func (s *Service) Resolve(ref string) (domain.VPNProfile, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return domain.VPNProfile{}, fmt.Errorf("a connection id or name is required")
	}
	if found, err := s.Get(ref); err == nil {
		return found, nil
	}

	wanted := foldName(ref)
	var matches []domain.VPNProfile
	s.store.Read(func(d *store.Data) {
		for _, p := range d.Profiles {
			if foldName(p.Name) == wanted {
				matches = append(matches, p)
			}
		}
	})

	switch len(matches) {
	case 0:
		return domain.VPNProfile{}, fmt.Errorf("connection %s: %w", ref, domain.ErrNotFound)
	case 1:
		return matches[0], nil
	default:
		return domain.VPNProfile{}, fmt.Errorf(
			"more than one connection is called %q; rename one of them, or refer to it by id", ref)
	}
}

// Create validates and stores a new profile.
func (s *Service) Create(in domain.ProfileInput) (domain.VPNProfile, error) {
	if err := in.Validate(); err != nil {
		return domain.VPNProfile{}, err
	}
	in = in.Normalise()

	now := time.Now()
	p := domain.VPNProfile{
		ID:            uuid.NewString(),
		Name:          in.Name,
		Kind:          in.Kind,
		Provider:      in.Provider,
		Group:         domain.NormaliseGroup(in.Group),
		Requires:      in.Requires,
		Host:          in.Host,
		Port:          in.Port,
		VendorProfile: in.VendorProfile,
		ClientPath:    in.ClientPath,
		SSH:           in.SSH,
		Notes:         in.Notes,
		CredentialIDs: []string{},
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	err := s.store.Update(func(d *store.Data) error {
		if taken := nameTaken(d.Profiles, p.Name, ""); taken {
			return fmt.Errorf("a connection called %q already exists", p.Name)
		}
		if err := checkRequirement(d, p.ID, p.Requires); err != nil {
			return err
		}
		d.Profiles = append(d.Profiles, p)
		return nil
	})
	if err != nil {
		return domain.VPNProfile{}, err
	}
	return p, nil
}

// nameTaken reports whether a profile other than exceptID answers to this name.
//
// Names have to be unique because they are how the other biebie.net
// applications refer to a connection: a cluster in Biebie Kube configured with
// "vpn-cat" must name exactly one tunnel. Comparison ignores case and
// surrounding space, since "SMOI" and "smoi " standing for different customers
// would be a trap rather than a distinction.
//
// It is called inside a store transaction so two connections cannot claim the
// same name by being created at the same moment.
func nameTaken(profiles []domain.VPNProfile, name, exceptID string) bool {
	wanted := foldName(name)
	for _, p := range profiles {
		if p.ID != exceptID && foldName(p.Name) == wanted {
			return true
		}
	}
	return false
}

func foldName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// checkRequirement validates a prerequisite reference inside a transaction.
//
// A connection may only need one that exists, is not itself, and does not
// already depend on it. The last case is what stops a pair from waiting on each
// other: bringing either one up would otherwise recurse until something gave.
func checkRequirement(d *store.Data, id, requires string) error {
	if requires == "" {
		return nil
	}
	if requires == id {
		return errors.New("a connection cannot need itself")
	}

	target := findProfile(d, requires)
	if target == nil {
		return fmt.Errorf("the connection this one needs first: %w", domain.ErrNotFound)
	}

	// Walk the chain above the target. Arriving back at this profile means the
	// two would need each other, directly or through however many hops.
	step := target
	for hops := 0; step != nil && step.Requires != "" && hops <= len(d.Profiles); hops++ {
		if step.Requires == id {
			return fmt.Errorf("%q already needs this connection, so they cannot need each other", target.Name)
		}
		step = findProfile(d, step.Requires)
	}
	return nil
}

func findProfile(d *store.Data, id string) *domain.VPNProfile {
	for i := range d.Profiles {
		if d.Profiles[i].ID == id {
			return &d.Profiles[i]
		}
	}
	return nil
}

// Update validates and applies changes to an existing profile.
func (s *Service) Update(id string, in domain.ProfileInput) (domain.VPNProfile, error) {
	if err := in.Validate(); err != nil {
		return domain.VPNProfile{}, err
	}
	in = in.Normalise()

	var updated domain.VPNProfile
	err := s.store.Update(func(d *store.Data) error {
		for i := range d.Profiles {
			if d.Profiles[i].ID != id {
				continue
			}
			if nameTaken(d.Profiles, in.Name, id) {
				return fmt.Errorf("a connection called %q already exists", in.Name)
			}
			if err := checkRequirement(d, id, in.Requires); err != nil {
				return err
			}
			changingKind := d.Profiles[i].AccessKind() != in.Kind

			d.Profiles[i].Name = in.Name
			d.Profiles[i].Kind = in.Kind
			d.Profiles[i].Provider = in.Provider
			d.Profiles[i].Group = domain.NormaliseGroup(in.Group)
			d.Profiles[i].Requires = in.Requires
			d.Profiles[i].VendorProfile = in.VendorProfile
			d.Profiles[i].ClientPath = in.ClientPath
			d.Profiles[i].Notes = in.Notes

			switch {
			case in.Kind != domain.KindSSH:
				d.Profiles[i].Host = in.Host
				d.Profiles[i].Port = in.Port
				d.Profiles[i].SSH = domain.SSHSettings{}
			case changingKind:
				// The address a VPN was using is a gateway, not a bastion, and
				// carrying it over would leave a tunnel pointed somewhere it
				// was never meant to dial. It starts empty instead.
				d.Profiles[i].Host = ""
				d.Profiles[i].Port = domain.DefaultSSHPort
				d.Profiles[i].SSH = domain.SSHSettings{}
			}
			// An SSH tunnel that stays one keeps its server and port map: those
			// are set on the connection's own page, and this is the dialog that
			// does not ask about them.

			d.Profiles[i].UpdatedAt = time.Now()
			updated = d.Profiles[i]
			pruneGroups(d)
			return nil
		}
		return fmt.Errorf("profile %s: %w", id, domain.ErrNotFound)
	})
	if err != nil {
		return domain.VPNProfile{}, err
	}
	return updated, nil
}

// Delete removes a profile. Credential cleanup is the caller's responsibility
// (CredentialService owns secret removal).
//
// Anything that needed this connection first is released rather than left
// pointing at a gap, so deleting a VPN cannot turn the tunnels behind it into
// connections that refuse to start for a reason nobody can see.
func (s *Service) Delete(id string) error {
	return s.store.Update(func(d *store.Data) error {
		for i := range d.Profiles {
			if d.Profiles[i].ID == id {
				d.Profiles = append(d.Profiles[:i], d.Profiles[i+1:]...)
				delete(d.Preferences, id)
				releaseRequirement(d, id)
				pruneGroups(d)
				return nil
			}
		}
		return fmt.Errorf("profile %s: %w", id, domain.ErrNotFound)
	})
}

func releaseRequirement(d *store.Data, id string) {
	for i := range d.Profiles {
		if d.Profiles[i].Requires == id {
			d.Profiles[i].Requires = ""
			d.Profiles[i].UpdatedAt = time.Now()
		}
	}
}

// Dependents lists the connections that need this one up first.
//
// Order follows the stored order, which is the order they were created in; the
// callers either tear all of them down or count them, so nothing depends on it.
func (s *Service) Dependents(id string) []domain.VPNProfile {
	if id == "" {
		return nil
	}
	var out []domain.VPNProfile
	s.store.Read(func(d *store.Data) {
		for _, p := range d.Profiles {
			if p.Requires == id {
				out = append(out, p)
			}
		}
	})
	return out
}

// Groups lists every group in use with its connection count and whether the
// dashboard hides it by default. Named groups come first, alphabetically;
// ungrouped connections are reported last under an empty name.
func (s *Service) Groups() []domain.GroupView {
	counts := map[string]int{}
	chosen := map[string]bool{}
	s.store.Read(func(d *store.Data) {
		for _, p := range d.Profiles {
			counts[p.Group]++
		}
		for name, settings := range d.Groups {
			chosen[name] = settings.Hidden
		}
	})

	// The archive is always offered, empty or not: it is the destination the
	// "put this away" action needs, and an empty group costs the dashboard
	// nothing because a section with no connections is not drawn.
	if _, ok := counts[domain.ArchivedGroup]; !ok {
		counts[domain.ArchivedGroup] = 0
	}

	out := make([]domain.GroupView, 0, len(counts))
	for name, count := range counts {
		out = append(out, domain.GroupView{
			Name:        name,
			Label:       domain.GroupLabel(name),
			Hidden:      hiddenFor(chosen, name),
			Connections: count,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if groupRank(out[i].Name) != groupRank(out[j].Name) {
			return groupRank(out[i].Name) < groupRank(out[j].Name)
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// groupRank keeps the two sections that are not a company out of the alphabet:
// the ungrouped connections sit after the named groups, and the archive after
// those, so putting something away never pushes the groups in use down the page.
func groupRank(name string) int {
	switch name {
	case domain.ArchivedGroup:
		return 2
	case "":
		return 1
	default:
		return 0
	}
}

// hiddenFor resolves a group's visibility: what the user chose, otherwise the
// group's default. A group that cannot be hidden at all is never reported
// hidden, so a flag stored by an older build cannot strand its connections.
func hiddenFor(chosen map[string]bool, name string) bool {
	if !domain.CanHideGroup(name) {
		return false
	}
	if hidden, ok := chosen[name]; ok {
		return hidden
	}
	return domain.GroupHiddenByDefault(name)
}

// GroupHidden reports whether a group is kept off the connection list until the
// user asks for hidden connections.
func (s *Service) GroupHidden(name string) bool {
	group := domain.NormaliseGroup(name)
	chosen := map[string]bool{}
	s.store.Read(func(d *store.Data) {
		if settings, ok := d.Groups[group]; ok {
			chosen[group] = settings.Hidden
		}
	})
	return hiddenFor(chosen, group)
}

// SetGroupHidden hides or reveals a group.
//
// A choice that matches the group's default is recorded by deleting the record
// rather than storing it, so the document only ever describes groups that
// differ — which is also what lets a revealed archive survive a restart while
// an ordinary group left alone stores nothing at all.
func (s *Service) SetGroupHidden(name string, hidden bool) error {
	group := domain.NormaliseGroup(name)
	if !domain.CanHideGroup(group) {
		return fmt.Errorf("%s is not a group that can be hidden", domain.UngroupedLabel)
	}
	return s.store.Update(func(d *store.Data) error {
		if group != domain.ArchivedGroup && !groupInUse(d, group) {
			return fmt.Errorf("group %q: %w", domain.GroupLabel(group), domain.ErrNotFound)
		}
		if hidden == domain.GroupHiddenByDefault(group) {
			delete(d.Groups, group)
			return nil
		}
		if d.Groups == nil {
			d.Groups = map[string]domain.GroupSettings{}
		}
		d.Groups[group] = domain.GroupSettings{Name: group, Hidden: hidden}
		return nil
	})
}

// SetGroup files a profile under a different group.
//
// It is separate from Update because moving a connection between groups changes
// where it is listed, not how it connects: there is no endpoint to re-validate
// and, in the layer above, no reason to make the engineer disconnect first.
func (s *Service) SetGroup(id string, group string) (domain.VPNProfile, error) {
	name := domain.NormaliseGroup(group)
	if err := domain.ValidateGroup(name); err != nil {
		return domain.VPNProfile{}, err
	}

	var updated domain.VPNProfile
	err := s.store.Update(func(d *store.Data) error {
		for i := range d.Profiles {
			if d.Profiles[i].ID != id {
				continue
			}
			d.Profiles[i].Group = name
			d.Profiles[i].UpdatedAt = time.Now()
			updated = d.Profiles[i]
			pruneGroups(d)
			return nil
		}
		return fmt.Errorf("profile %s: %w", id, domain.ErrNotFound)
	})
	if err != nil {
		return domain.VPNProfile{}, err
	}
	return updated, nil
}

// SetRequirement records the connection that has to be up first, or clears it
// when requires is empty.
//
// It is separate from Update for the same reason SetGroup is: the order things
// have to be reached in is not part of how any one of them connects. Import
// needs it too — a file names the prerequisite, so the reference can only be
// resolved once every connection in that file exists.
func (s *Service) SetRequirement(id, requires string) (domain.VPNProfile, error) {
	requires = strings.TrimSpace(requires)

	var updated domain.VPNProfile
	err := s.store.Update(func(d *store.Data) error {
		for i := range d.Profiles {
			if d.Profiles[i].ID != id {
				continue
			}
			if err := checkRequirement(d, id, requires); err != nil {
				return err
			}
			d.Profiles[i].Requires = requires
			d.Profiles[i].UpdatedAt = time.Now()
			updated = d.Profiles[i]
			return nil
		}
		return fmt.Errorf("profile %s: %w", id, domain.ErrNotFound)
	})
	if err != nil {
		return domain.VPNProfile{}, err
	}
	return updated, nil
}

// SetTunnel replaces the SSH half of a connection: the server it dials, the
// login, and the ports it lends.
//
// It is separate from Update because it is edited somewhere else — on the
// connection's own page, beside the ports the tunnel currently has open —
// and because a tunnel is allowed to be saved unfinished. A connection that is
// not an SSH tunnel has no SSH half to set, and says so rather than quietly
// growing one.
func (s *Service) SetTunnel(id string, in domain.TunnelInput) (domain.VPNProfile, error) {
	if err := in.Validate(); err != nil {
		return domain.VPNProfile{}, err
	}
	in = in.Normalise()

	var updated domain.VPNProfile
	err := s.store.Update(func(d *store.Data) error {
		for i := range d.Profiles {
			if d.Profiles[i].ID != id {
				continue
			}
			if !d.Profiles[i].IsSSH() {
				return fmt.Errorf("%q is not an SSH tunnel", d.Profiles[i].Name)
			}
			d.Profiles[i].Host = in.Host
			d.Profiles[i].Port = in.Port
			d.Profiles[i].SSH = in.SSH
			d.Profiles[i].UpdatedAt = time.Now()
			updated = d.Profiles[i]
			return nil
		}
		return fmt.Errorf("profile %s: %w", id, domain.ErrNotFound)
	})
	if err != nil {
		return domain.VPNProfile{}, err
	}
	return updated, nil
}

func groupInUse(d *store.Data, group string) bool {
	for _, p := range d.Profiles {
		if p.Group == group {
			return true
		}
	}
	return false
}

// pruneGroups drops settings for groups no profile uses any more, so a group
// that was renamed or emptied cannot leave a hidden flag behind that silently
// applies when the same name is typed again months later.
//
// The archive is exempt: it is a fixture of the UI rather than a name someone
// happened to type, so a revealed archive stays revealed after its last
// connection moves out.
func pruneGroups(d *store.Data) {
	if len(d.Groups) == 0 {
		return
	}
	for name := range d.Groups {
		if name == domain.ArchivedGroup || groupInUse(d, name) {
			continue
		}
		delete(d.Groups, name)
	}
}

// Preferences returns remembered UI choices for a profile.
func (s *Service) Preferences(profileID string) domain.ProfilePreferences {
	prefs := domain.ProfilePreferences{ProfileID: profileID}
	s.store.Read(func(d *store.Data) {
		if p, ok := d.Preferences[profileID]; ok {
			prefs = p
		}
	})
	return prefs
}

// RememberCredential records the last credential UUID used on a profile.
//
// Only the UUID is stored. Remembering a username would be ambiguous the moment
// two credentials share one, which the domain explicitly permits.
func (s *Service) RememberCredential(profileID, credentialID string) error {
	return s.store.Update(func(d *store.Data) error {
		if d.Preferences == nil {
			d.Preferences = map[string]domain.ProfilePreferences{}
		}
		d.Preferences[profileID] = domain.ProfilePreferences{
			ProfileID:        profileID,
			LastCredentialID: credentialID,
		}
		return nil
	})
}
