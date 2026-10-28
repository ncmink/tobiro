package domain

import (
	"errors"
	"regexp"
	"strings"
)

// UngroupedLabel is what the UI shows for connections that have no group.
const UngroupedLabel = "Ungrouped"

// ArchivedGroup is the built-in group for connections that are put away.
//
// It exists even when it is empty and it starts hidden, which is the whole
// point: there is always one place to drop a connection you are not working
// with, without having to decide on a name for where it went first.
const ArchivedGroup = "Archived"

// groupPattern keeps a group name printable and single-line. A group name is a
// display label only — it never reaches a command line — so the rule is about
// readability, not shell safety.
var groupPattern = regexp.MustCompile(`^[^\x00-\x1f\x7f]+$`)

// GroupSettings is the presentation state of one group of connections.
//
// A group is a label carried by the profile, not an entity of its own: nothing
// is keyed by a group ID, so renaming a group is just an edit to the profiles
// that use it. This record holds the only thing the group itself owns —
// whether the dashboard keeps it out of sight until asked. That is what makes
// working for several customers bearable: the connections for the customer you
// are not on today stay off screen without being deleted.
type GroupSettings struct {
	Name   string `json:"name"`
	Hidden bool   `json:"hidden"`
}

// GroupView is one group as the dashboard sees it.
type GroupView struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Hidden      bool   `json:"hidden"`
	Connections int    `json:"connections"`
}

// NormaliseGroup trims a user-supplied group name. An empty result means the
// profile is ungrouped.
//
// A name that matches the archive apart from its case becomes the archive
// itself, so typing "archived" into the form files the connection there instead
// of creating a second group that looks identical in the list.
func NormaliseGroup(name string) string {
	group := strings.TrimSpace(name)
	if strings.EqualFold(group, ArchivedGroup) {
		return ArchivedGroup
	}
	return group
}

// GroupHiddenByDefault reports the visibility a group has before anyone has
// chosen for it. Only the archive starts out of sight.
func GroupHiddenByDefault(name string) bool {
	return NormaliseGroup(name) == ArchivedGroup
}

// CanHideGroup reports whether a group can be kept out of the connection list.
//
// The ungrouped connections cannot. "Ungrouped" is not a company the engineer
// is done with for the day, it is where everything without a group lands, so
// hiding it would mean a connection arrives invisible and the section offering
// to reveal it is the one that just disappeared. A single connection goes to the
// archive instead.
func CanHideGroup(name string) bool { return NormaliseGroup(name) != "" }

// GroupLabel is the display name for a group name, including the ungrouped one.
func GroupLabel(name string) string {
	if NormaliseGroup(name) == "" {
		return UngroupedLabel
	}
	return NormaliseGroup(name)
}

// ValidateGroup rejects group names that cannot be shown as a single line.
func ValidateGroup(name string) error {
	group := NormaliseGroup(name)
	if group == "" {
		return nil
	}
	if len(group) > 64 {
		return errors.New("group name is too long (max 64 characters)")
	}
	if !groupPattern.MatchString(group) {
		return errors.New("group name contains invalid characters")
	}
	return nil
}
