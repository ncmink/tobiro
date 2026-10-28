package tailscale

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"tobiro/internal/domain"
	"tobiro/internal/sysexec"
)

// ActiveSwitch holds every tailscale switch profile on this machine.
type ActiveSwitch struct {
	Accounts []switchAccount
}

func (sw ActiveSwitch) SelectedProfile() (switchAccount, bool) {
	for _, a := range sw.Accounts {
		if a.Selected {
			return a, true
		}
	}
	return switchAccount{}, false
}

// ActiveLabel summarises the selected profile for status detail text.
func (sw ActiveSwitch) ActiveLabel(identity string) string {
	sel, ok := sw.SelectedProfile()
	if !ok {
		return strings.TrimSpace(identity)
	}
	return profileDisplayLabel(sel, identity)
}

func profileDisplayLabel(a switchAccount, identity string) string {
	id := strings.TrimSpace(identity)
	tailnet := strings.TrimSpace(a.Tailnet)
	nick := strings.TrimSpace(a.Nickname)
	account := strings.TrimSpace(a.Account)

	switch {
	case nick != "":
		if tailnet != "" && !strings.EqualFold(tailnet, nick) {
			return nick + " (" + tailnet + ")"
		}
		return nick
	case tailnet != "" && id != "" && !strings.EqualFold(tailnet, id):
		return tailnet + " (" + id + ")"
	case tailnet != "":
		return tailnet
	case id != "":
		return id
	case account != "":
		return account
	default:
		return a.ID
	}
}

type switchAccount struct {
	ID       string `json:"id"`
	Nickname string `json:"nickname"`
	Tailnet  string `json:"tailnet"`
	Account  string `json:"account"`
	Selected bool   `json:"selected"`
}

// switchTarget is the argument tailscale switch accepts: nickname if set, else account.
func (a switchAccount) switchTarget() string {
	if n := strings.TrimSpace(a.Nickname); n != "" {
		return n
	}
	return strings.TrimSpace(a.Account)
}

func parseSwitchListJSON(raw string) ([]switchAccount, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var accounts []switchAccount
	if err := json.Unmarshal([]byte(raw), &accounts); err != nil {
		return nil, fmt.Errorf("parse tailscale switch list: %w", err)
	}
	return accounts, nil
}

type resolveResult int

const (
	resolveNotFound resolveResult = iota
	resolveFound
	resolveAmbiguous
)

type resolvedProfile struct {
	Account switchAccount
	Result  resolveResult
}

// resolveProfile maps a connection's VendorProfile to one switch list entry.
// Order: id, tailnet, nickname, then account login only when unique.
func resolveProfile(accounts []switchAccount, vendor string) resolvedProfile {
	vendor = strings.TrimSpace(vendor)
	if vendor == "" {
		return resolvedProfile{Result: resolveNotFound}
	}

	if a, ok := uniqueMatch(accounts, func(a switchAccount) bool { return a.ID == vendor }); ok {
		return resolvedProfile{Account: a, Result: resolveFound}
	}

	if matches := filterAccounts(accounts, func(a switchAccount) bool {
		return strings.EqualFold(strings.TrimSpace(a.Tailnet), vendor)
	}); len(matches) == 1 {
		return resolvedProfile{Account: matches[0], Result: resolveFound}
	} else if len(matches) > 1 {
		return resolvedProfile{Result: resolveAmbiguous}
	}

	if matches := filterAccounts(accounts, func(a switchAccount) bool {
		return strings.EqualFold(strings.TrimSpace(a.Nickname), vendor)
	}); len(matches) == 1 {
		return resolvedProfile{Account: matches[0], Result: resolveFound}
	} else if len(matches) > 1 {
		return resolvedProfile{Result: resolveAmbiguous}
	}

	if matches := filterAccounts(accounts, func(a switchAccount) bool {
		return strings.EqualFold(strings.TrimSpace(a.Account), vendor)
	}); len(matches) == 1 {
		return resolvedProfile{Account: matches[0], Result: resolveFound}
	} else if len(matches) > 1 {
		return resolvedProfile{Result: resolveAmbiguous}
	}

	return resolvedProfile{Result: resolveNotFound}
}

func uniqueMatch(accounts []switchAccount, pred func(switchAccount) bool) (switchAccount, bool) {
	var found switchAccount
	n := 0
	for _, a := range accounts {
		if pred(a) {
			found = a
			n++
		}
	}
	if n == 1 {
		return found, true
	}
	return switchAccount{}, false
}

func filterAccounts(accounts []switchAccount, pred func(switchAccount) bool) []switchAccount {
	var out []switchAccount
	for _, a := range accounts {
		if pred(a) {
			out = append(out, a)
		}
	}
	return out
}

func optionNameForAccount(a switchAccount, accounts []switchAccount) string {
	nick := strings.TrimSpace(a.Nickname)
	if nick != "" && len(filterAccounts(accounts, func(x switchAccount) bool {
		return strings.EqualFold(strings.TrimSpace(x.Nickname), nick)
	})) == 1 {
		return nick
	}
	tailnet := strings.TrimSpace(a.Tailnet)
	if tailnet != "" && len(filterAccounts(accounts, func(x switchAccount) bool {
		return strings.EqualFold(strings.TrimSpace(x.Tailnet), tailnet)
	})) == 1 {
		return tailnet
	}
	return a.ID
}

func optionDetailForAccount(a switchAccount) string {
	tailnet := strings.TrimSpace(a.Tailnet)
	account := strings.TrimSpace(a.Account)
	switch {
	case tailnet != "" && account != "" && !strings.EqualFold(tailnet, account):
		return tailnet + " · " + account
	case tailnet != "":
		return tailnet
	case account != "":
		return account
	default:
		return a.ID
	}
}

func accountsToOptions(accounts []switchAccount) []domain.VendorProfileOption {
	out := make([]domain.VendorProfileOption, 0, len(accounts))
	for _, a := range accounts {
		name := optionNameForAccount(a, accounts)
		if name == "" {
			continue
		}
		out = append(out, domain.VendorProfileOption{
			Name:      name,
			Detail:    optionDetailForAccount(a),
			Connected: a.Selected,
		})
	}
	return out
}

// ListAccounts reports tailscale switch --list on this machine.
func ListAccounts(ctx context.Context, cli string) ([]domain.VendorProfileOption, error) {
	res, err := sysexec.Run(ctx, cli, "switch", "--list", "--json")
	if err != nil {
		combined := strings.TrimSpace(res.Combined())
		if combined == "" {
			return nil, err
		}
		return nil, fmt.Errorf("%s", combined)
	}
	accounts, err := parseSwitchListJSON(res.Stdout)
	if err != nil {
		return nil, err
	}
	return accountsToOptions(accounts), nil
}

// accountMatches reports whether two profile tokens are the same (legacy helper).
func accountMatches(activeIdentity, vendorProfile string) bool {
	active := strings.TrimSpace(activeIdentity)
	target := strings.TrimSpace(vendorProfile)
	if target == "" {
		return true
	}
	if active == "" {
		return false
	}
	if strings.Contains(target, "@") || strings.Contains(active, "@") {
		return strings.EqualFold(active, target)
	}
	return active == target
}

// profileChoices lists the names the connection form would offer, for error text.
func profileChoices(accounts []switchAccount) string {
	names := make([]string, 0, len(accounts))
	for _, a := range accounts {
		if n := optionNameForAccount(a, accounts); n != "" {
			names = append(names, n)
		}
	}
	return strings.Join(names, ", ")
}

// resolveConnectError reports a card whose Tailscale account cannot be mapped
// to exactly one profile. These are configuration problems the user fixes by
// editing the connection, not capabilities the client lacks, so they are plain
// errors rather than UnsupportedOperationError.
func resolveConnectError(vendor string, r resolvedProfile) error {
	switch r.Result {
	case resolveAmbiguous:
		return fmt.Errorf(
			"Tailscale account %q matches several profiles on this machine (same login). Edit this connection and pick a nickname or tailnet from tailscale switch --list",
			vendor,
		)
	case resolveNotFound:
		return fmt.Errorf(
			"Tailscale account %q is not on this machine. Open the Tailscale app, sign in or join that tailnet, then connect again",
			vendor,
		)
	default:
		return nil
	}
}

// switchTargetForProfile returns the trimmed vendor profile stored on the card.
func switchTargetForProfile(vendorProfile string) string {
	return strings.TrimSpace(vendorProfile)
}
