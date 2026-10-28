# Tobiro

Desktop access manager for DevOps engineers who work across multiple customer
networks. Part of the **biebie.net** product family.

Tobiro **does not implement VPN protocols**. It orchestrates the
officially installed vendor clients through provider adapters, and refuses to
guess at undocumented behaviour.

```text
Vue 3 UI
   ↓
Wails bindings          (no REST API in between)
   ↓
Go core
   ↓
VPN provider adapters
   ↓
Installed native VPN clients
```

## Stack

Wails v2 · Go 1.25 · Vue 3 · TypeScript · Vite · Pinia · Tailwind CSS v4

---

## The domain model

The three concepts that everything else hangs off are kept separate, so the app
can later grow from a VPN manager into a broader customer access workspace
(SSH, Kubernetes, databases, cloud accounts) without a redesign.

```text
VPN Profile  ──  the customer/project endpoint + provider
      │
      ├── Credentials[]   ── one profile, many login identities
      │
      └── Session         ── one live connection attempt
```

### Credential identity is a UUID, never a username

This is the rule the codebase is built around:

```text
SMOI         credentialId = 5f3a…   username = devops
Customer A   credentialId = 91b7…   username = devops
Customer B   credentialId = c204…   username = devops
```

All three are valid and distinct — and two credentials on the *same* profile may
also share a username. Nothing keys off `username`: not storage, not the secret
reference, not the "last used account" preference, not the session record.

`Label` is the human-readable identity shown in the UI ("Ming", "DevOps
Shared", "Backup Account").

`internal/vpn/manager_test.go` enforces this with
`TestDuplicateUsernamesAreDistinctIdentities`.

### Groups keep several employers apart

A profile carries a group name — usually the company the connection belongs to —
and a whole group can be kept out of the connection list:

```text
Acme Co     hidden     3 connections
Zeta Co                2 connections
Ungrouped              1 connection
```

Hiding is presentation only: nothing is deleted, a hidden connection still
connects, and the dashboard's **Show hidden** toggle reveals those groups for as
long as they are needed.

An SSH tunnel that **requires** a VPN in a hidden group stays off the dashboard
until hidden groups are revealed, even when the tunnel itself sits in another
group or **Ungrouped**. It is still listed nested under that VPN once you turn
**Show hidden** on.

**Ungrouped** has no hide control, because it is not a company anyone is done
with: it is where every connection without a group lands, so hiding it would
mean the next one arrives invisible behind the very section that would have
offered to reveal it. A flag stored there by an older build is ignored for the
same reason. To put a single connection away, archive it.

A group is a label, not an entity. Nothing is keyed by a group ID, so renaming a
group is an ordinary edit to the profiles that use it, and application data
records only the groups that differ from the default:

```json
"groups": {
  "Acme Co": { "name": "Acme Co", "hidden": true }
}
```

Emptying a group forgets its hidden flag, so a name typed again months later
cannot start out invisible. `internal/profile/service_test.go` enforces that.

### The archive is the one group that is always there

**Archived** is built in. It is offered even when it holds nothing, and it is
hidden until you say otherwise — the point is that there is always somewhere to
drop a connection you are not working with, without inventing a name for it
first. Because hidden is its default, revealing it is what gets recorded, and it
keeps that choice after its last connection moves out.

Right-click any card to move a connection between groups, the archive included.
Moving is deliberately not an edit: it changes where a connection is listed, not
how it connects, so `MoveConnection` works while the tunnel is up where
`UpdateConnection` asks you to disconnect first. When the destination is out of
sight the dashboard says where the card went rather than letting it vanish.

---

## Password security

Passwords are **never** written to JSON, YAML, SQLite, Pinia, localStorage, logs
or the repository.

| Platform | Store used |
| --- | --- |
| macOS | Keychain |
| Windows | Credential Manager |
| Linux | Secret Service / libsecret |

Application data holds only a reference derived from UUIDs:

```yaml
credential:
  id: 5f3a1c9e-…
  username: ming
  secretRef: biebie-access/<profileId>/<credentialId>
```

The frontend receives a `CredentialView`, which has no password field at all:

```json
{ "id": "5f3a1c9e-…", "label": "Ming", "username": "ming", "hasPassword": true }
```

A password is materialised in exactly one place — `CredentialService.Resolve`,
inside a single connect call — and handed straight to the adapter:

```text
Vue → Connect(profileId, credentialId) → Go core → OS credential store
                                                       ↓ password
                                                  VPN provider adapter
```

Defence in depth around that path:

- `Credential.Secret` is `json:"-"` so it cannot be serialised.
- `Credential.String()` masks the secret, so `%v` in a log line cannot leak it.
- The logger registers the live secret for the duration of the attempt and
  redacts it from every message, plus any `password=` / `otp=` style pair.
- The JSON store strips any `Secret` field found on load.

If the OS store is unavailable (headless Linux, CI), the app degrades to an
in-memory store for the session rather than writing secrets to disk, and says so
in Settings.

### MFA / OTP

One-time codes are transient. An adapter pauses authentication, Go emits
`auth:otp_required`, Vue shows the dialog, and the answer travels back over a
channel that lives only as long as the connect call. Nothing is persisted or
logged.

---

## Connection state machine

State is never a boolean:

```text
disconnected → connecting → authenticating → waiting_for_otp → connected
                    ↓             ↓                ↓              ↓
                  failed        failed          failed      disconnecting → disconnected
```

Transitions are declared in `internal/domain/session.go` and enforced by
`SessionManager`; an illegal move from an adapter is rejected rather than
silently applied.

---

## Provider support matrix

Adapters report what the installed client **officially** supports on the current
platform and version. Where a vendor documents no CLI, the adapter says so and
Tobiro launches the vendor GUI instead of inventing arguments.

| Provider | Detect | Version | Connect / disconnect | Status | Fallback |
| --- | --- | --- | --- | --- | --- |
| Forcepoint NGFW VPN Client | yes | macOS | no documented CLI | macOS | open native client |
| Ivanti Secure Access | yes | macOS | pending verification | macOS | open native client |
| FortiClient | yes | macOS | pending verification | macOS | open native client |
| Tailscale | yes | all | yes, documented CLI | yes | open native client |
| Surfshark VPN | yes | macOS | no documented CLI | macOS | open native client |
| macOS VPN (L2TP/IPsec, IKEv2) | yes | macOS | disconnect only, `scutil --nc` | macOS | open Network settings |
| Mock (development) | yes | — | yes, incl. OTP | yes | — |

"Pending verification" means a platform CLI was detected (`pulsesvc` /
`pulselauncher.exe` on Linux and Windows for Ivanti, `fortivpn` on Linux for
FortiClient, `surfshark-vpn` on Linux for Surfshark) but is not driven yet.
Enabling it is an isolated change inside the adapter: flip the capability flags
and implement the command. Nothing in the manager, services or UI needs to
change.

Detection on the current machine is shown in **Settings → VPN clients**.

### macOS VPN services, and why Connect is declined

The VPN services configured in **System Settings → Network → VPN** — L2TP over
IPsec, IKEv2, Cisco IPsec — are managed by macOS, which publishes more than any
vendor client here documents: `scutil --nc list|status|start|stop`.

Three of those four are used. **Connect is not**, and the reason is worth
recording, because it is not a missing feature.

L2TP over IPsec has two secrets kept in two different places. The user password
is referenced from the login keychain (`AuthPasswordEncryption : Keychain`) and
macOS reads it happily. The **IPSec shared secret** lives in the *System*
keychain, which macOS will not read on behalf of a user-level process — so
`scutil --nc start` fails with `The IPSec Shared Secret is missing` however
healthy the configuration is. System Settings succeeds because it runs somewhere
that may read it.

`scutil --nc start` does accept `--secret`, so this *could* be made to work — by
having Tobiro hold the shared secret and pass it back. It does not: taking
custody of a secret macOS is already keeping, in order to hand it to macOS, adds
a place for it to leak and buys one saved click.

So the split is by what needs a secret. Connecting stays in System Settings,
where the secret already is. Reporting the tunnel and stopping it need no secret,
so those are driven from here: `CanDisconnect` and `CanReadStatus` are true while
`CanConnect` is false, and Connect returns the same `UnsupportedOperationError`
with a native-UI fallback that the undocumented vendor clients use. The user gets
an explanation and a button to System Settings instead of a Connect that fails.

Two further consequences of macOS owning these services:

- **A profile names an existing service; Tobiro never creates or edits
  one.** That means writing privileged system network preferences, which is
  exactly the kind of guessing this codebase refuses to do.
- **Only services macOS owns are offered.** `scutil --nc list` also reports
  third-party NetworkExtension services, distinguished by a `VPN:<bundle id>`
  type. macOS cannot start those, and Surfshark and Tailscale already have their
  own adapters, so they are filtered out. Naming one anyway is refused with an
  explanation rather than a failed start.

The form does not ask for the name at all. `scutil --nc list` can enumerate what
is configured, so the connection form **offers the real services** with their
protocol and current state, and the user picks one:

```
VPN CAT      L2TP over IPsec        connected
Sweet Home   L2TP over IPsec        disconnected
VPN          L2TP over IPsec        disconnected
```

This is an optional adapter capability, `vpn.VendorProfileLister`, alongside
`LoginFiller`, `StartupController` and `NetworkDoctor`. No vendor here documents
a way to read its tunnel list, so macOS is the only implementation; adapters
without it fall back to a text field, and a name typed that way is still checked
against the machine:

```
macOS has no VPN service named "VPN Cat". Configured services: VPN CAT, Sweet Home, VPN.
```

### Connections without a gateway

Most providers are corporate gateways: the user supplies `host:port`, and one
gateway is reached with a different account per customer. Three are not, and the
domain says so rather than the UI guessing from a provider key:

- `RequiresEndpoint()` is false for **Tailscale**, **Surfshark** and **macOS
  VPN**, so the Add Connection form hides the gateway fields and shows the
  provider's own note.
- `ManagesOwnAccount()` is true for the same three, so no account list is
  offered: a Tailscale connection is this machine's node, a Surfshark connection
  is a subscription kept signed in inside the Surfshark app, and a macOS VPN
  service keeps its password in the Keychain.
- `VendorProfileField()` describes the vendor-side selector a provider connects
  by, so the form can label and require it per provider — "Tunnel name already
  configured in the vendor client" for FortiClient, "macOS VPN service" for
  macOS, **Tailscale account** (login email or nickname) for Tailscale. Where
  the adapter can enumerate the choices, the form offers them instead of asking
  for a name.

#### Tailscale and multiple tailnets

Tailscale keeps **one active tailnet per machine** ([fast user switching](https://tailscale.com/docs/features/client/fast-user-switching)).
Organisations can also run **multiple tailnets** under one identity provider
([multiple tailnets](https://tailscale.com/docs/features/multiple-tailnets)); joining
a new org tailnet still happens in the Tailscale app (or its tailnet selector URL),
not inside Tobiro.

For multi-customer work, add **one Biebie connection per tailnet/profile** and
pick the profile in the form — Tobiro lists whatever is already logged in
on this machine (`tailscale switch --list --json`). Each card stores a
**Tailscale account** value that resolves in order: profile **id**, **tailnet**,
**nickname**, then **login email** only when that login is unique on the machine.
Two tailnets under the same login (common with personal + org tailnets) must use
nickname or tailnet on each card, not the shared login alone.

**Connect** resolves the card to one switch profile, runs `tailscale switch <id>`
when that profile is not selected, then `tailscale up`. **Disconnect** runs
`tailscale down` only when the resolved profile is the active one. A card shows
**Connected** only when Tailscale is running and that profile is selected; other
cards stay disconnected and name the active profile.

Connections saved before this field existed may still have an empty account;
those behave like the old global Tailscale toggle until you edit them and choose
a profile.

These are plain provider properties, which is why adding Surfshark and macOS VPN
needed no new special case in the frontend.

### Reading status without a vendor API

None of the gateway vendors exposes tunnel state to third-party tools on macOS,
so `internal/vpn/tunnel` reads the machine's own network state instead. Two
signals must agree before a profile is reported connected:

1. **An established TCP connection to the profile's `host:port`.** VPN clients
   hold this control connection open for the life of the tunnel, and it is what
   attributes a tunnel to one specific profile.
2. **A point-to-point interface carrying a `/32` address.** That is how an
   assigned VPN address appears; utility tunnels used by DNS proxies and
   similar tools carry a wider prefix, so the host mask separates them.

Requiring both stops an ordinary HTTPS request to the same host from looking
like a live VPN. When several tunnels are up the assigned address is reported
as unknown rather than guessed.

The effect: you log in through the vendor window, and Tobiro picks the
tunnel up on its next poll — showing state, assigned IP, interface and uptime.

On macOS the probe keys off a **control socket owned by the vendor process**
(Forcepoint, Ivanti, FortiClient). That is what the vendor GUI itself uses as
"Connected". The `/32` tunnel address is filled in afterwards, when the client
assigns it — it is not required to flip the dashboard to Connected.

A Go-side watcher polls every 2 seconds so status does not depend on the
webview timer, and a tunnel is only marked disconnected after three consecutive
misses, so a single dropped netstat sample cannot flicker the UI.

**Surfshark and macOS VPN services are read a different way**, and not from
network state at all. Both are owned by macOS: a service configured in System
Settings obviously so, and Surfshark's because its client runs the tunnel as a
NetworkExtension. `scutil --nc` reports that state per service, alongside the
owning app's bundle identifier where there is one — and that identifier is what
attributes a reading to Surfshark rather than to some other tunnel.

The parsing lives in `internal/vpn/nc`, shared by both adapters so there is one
tested implementation of the `scutil` output rather than two.

Inferring it from interfaces was tried first and is wrong. Surfshark has no
gateway to hold a control socket against, its WireGuard tunnel is handed a `/16`
rather than a `/32`, and its tunnel extension **keeps running after a
disconnect** — so "the extension is alive and something owns the default route"
reports an unrelated L2TP or Tailscale session as if it were Surfshark's. The
regression test for this is a real `scutil --nc list` capture from a machine in
exactly that state.

An address is reported only when macOS publishes one for the service; some
NetworkExtension providers publish no `IPv4` block, and none is invented in that
case. Detection separately reports whether the system extension has been
**approved** in System Settings, since an unapproved extension is the usual
reason a healthy-looking install cannot connect at all.

Because Tobiro did not choose the identity for a tunnel raised in the
vendor window, such a session is labelled *Signed in via vendor client* rather
than showing an account, and no Disconnect button is offered for a client that
cannot be disconnected programmatically.

### Adding a provider

```go
registry.Register(ciscosecure.New())
```

That is the whole integration point. `main.go` is the only file that knows which
adapters exist; the manager is vendor-agnostic by construction.

Anything a client can do beyond connect, disconnect and status is an optional
interface the adapter may implement, so no capability leaks into the manager as a
provider-specific branch:

| Interface | For clients that can… |
| --- | --- |
| `LoginFiller` | type a stored account into the vendor GUI |
| `StartupController` | stop the vendor GUI launching at login |
| `NetworkDoctor` | explain full-tunnel versus split-tunnel effects |
| `VendorProfileLister` | enumerate the tunnels or services already configured |

---

## Process execution safety

- Commands are always argument arrays via `exec.CommandContext`. There is no
  `sh -c` path anywhere in the codebase.
- Host, port, vendor profile and executable path are validated before they can
  reach an exec call. Hosts are restricted to DNS/IP characters; executable
  paths must be absolute and free of shell metacharacters.
- Where a client reads a password, it is written to stdin rather than passed as
  an argument, so it never appears in the process list.
- Every vendor CLI call is bounded by a timeout.

Logs look like this, and are asserted on in tests:

```text
00:20:01 Connecting profile=SMOI provider=forcepoint endpoint=vpn.smoi.example:443 credential=5f3a1c9e-…
00:20:05 Authentication requested
00:20:08 Connected profile=SMOI interface=utun7 ip=172.30.0.113
```

---

## Repository layout

```text
biebie-access/
├── main.go                     composition root — wires services, registers adapters
├── app.go                      Wails binding surface: use-case APIs only
├── bridge.go                   UI event bridge + OTP prompter
├── views.go                    frontend-facing view types
├── service_peer.go             the local endpoint and biebie-access:// links
│
├── internal/
│   ├── domain/                 models, state machine, capabilities, validation
│   ├── store/                  JSON application data (no secrets)
│   ├── security/               SecureCredentialStore abstraction
│   │   └── keychain/           macOS / Windows / Linux implementation
│   ├── logging/                redacting activity log
│   ├── sysexec/                safe process execution
│   ├── profile/                ProfileService
│   ├── credential/             CredentialService
│   ├── session/                SessionManager (state machine enforcement)
│   ├── peer/                   Biebie Context Protocol endpoint and notifier
│   └── vpn/
│       ├── provider.go         VPNProviderAdapter interface
│       ├── registry.go         provider registry
│       ├── manager.go          vendor-agnostic orchestration
│       ├── mock/               simulated client (full flow incl. OTP)
│       ├── forcepoint/
│       ├── ivanti/
│       ├── forticlient/
│       ├── tailscale/
│       ├── surfshark/
│       ├── macosvpn/           VPN services macOS itself manages
│       └── nc/                 shared `scutil --nc` reader
│
├── cmd/endpoint-smoke/         serves the endpoint without the desktop app
│
└── frontend/src/
    ├── components/             ConnectionCard, CredentialSelector, CredentialForm, …
    ├── views/                  Dashboard, ConnectionDetail, Settings
    ├── stores/                 connections.ts, sessions.ts
    └── lib/                    state machine presentation helpers
```

Business rules live in `internal/`. `app.go` only delegates.

---

## Talking to Biebie Kube

Tobiro answers the Biebie Context Protocol on a socket owned by the
logged-in user, and registers `biebie-access://`. The contract itself is a
sibling checkout, `biebie-protocol/`, shared with Biebie Kube through a
`replace` directive — so changing the wire format breaks the build in both
applications rather than leaving a mismatch to be found at runtime.

Three things cross, and nothing else:

| Direction | What | Carries |
|---|---|---|
| Kube asks | `access.status`, `access.profiles` | profile ids, names, coarse state |
| Kube asks | `access.connect` | one connection reference |
| Access tells | `access.session.changed` | one profile id and its state |

`access.connect` raises this application's window at that connection. It does
not sign in. A peer asking for a tunnel is a claim about intent made by another
process; only the person at the keyboard can answer a password or a one-time
code, so the confirming click stays here. The window explains why it appeared.

A reference from a peer is an id **or** a connection name, resolved here by
`profile.Service.Resolve`: the id is a UUID nobody types on purpose, so the name
is what an engineer configuring the other application actually has to hand.
Connection names are kept unique for exactly this reason, and `Create` and
`Update` refuse a name another connection already answers to.

Only the id is stable, though, and every state change is announced under the id.
So `access.connect` replies with the id it resolved to, and a caller configured
with a name records it in place of the name — otherwise it would not recognise
its own customer network coming up.

A connection state change is announced outward as well as inward. Without it, a
Biebie Kube cluster that refused to connect because the customer network was
down would wait for someone to click again, having no way to know the wait was
over.

Both sides degrade to nothing. Biebie Kube not running costs a failed dial to a
socket that is not there; Tobiro not running leaves Biebie Kube offering
to launch it. Either application is fully usable with the other uninstalled.

To exercise this without launching two desktop applications:

```bash
go run ./cmd/endpoint-smoke                 # serves the real endpoint
go -C ../biebie-kube run ./cmd/access-smoke <profile-id>
```

---

## Appearance

Settings → Appearance offers Light, Dark, and System themes. Changes apply
immediately across connections, details, forms, and menus. The preference is
saved on this device under `biebie-access.theme`; System follows changes to the
operating system appearance and is the default when no preference is saved.

The application uses the purple Biebie mascot consistently in its header,
macOS bundle and About panel, and Windows icon. The source artwork is
`frontend/src/assets/images/logo.png`; `app-logo.png` is the smaller UI asset,
`build/appicon.png` is the 1024px packaging source, and
`build/windows/icon.ico` contains sizes from 16px to 256px.

## Development

```bash
wails dev      # live-reload development
wails build    # production bundle into build/bin
go test ./...  # domain, credential identity, secret-leakage and flow tests
```

Application data lives at the path shown in Settings — on macOS,
`~/Library/Application Support/biebie-access/data.json`.

### Trying the full flow without a VPN

Run `wails dev` to enable the **Mock VPN (development)** provider. It is omitted
from packaged builds, including `wails build -debug`, in both the connection
form and Settings → VPN clients. Existing saved mock profiles are preserved,
but their simulated adapter is available only in development.

Create a connection with this provider, add an account
with authentication set to **Username + Password + OTP**, and connect. The
dashboard walks through `connecting → authenticating → waiting_for_otp →
connected`, prompts for a code, and reports an assigned IP and duration.

Two deliberate hooks make the failure paths easy to exercise: the username
`fail` is rejected by the gateway, and the code `000000` is rejected as invalid.

### Checking an adapter against a real client

Adapters that read a live client carry opt-in tests, skipped by default so
`go test ./...` stays hermetic. They are the only way to catch a reading that
parses correctly but describes the wrong tunnel.

```bash
# Surfshark: run once in each state — the pair is what shows the reading
# follows Surfshark's own tunnel and not another VPN that happens to be up.
BIEBIE_TEST_SURFSHARK=connected    go test ./internal/vpn/surfshark/ -v
BIEBIE_TEST_SURFSHARK=disconnected go test ./internal/vpn/surfshark/ -v

# macOS VPN, read-only: detection, the offered services, resolution and status.
BIEBIE_TEST_MACOS_VPN="VPN CAT" go test ./internal/vpn/macosvpn/ -v

# macOS VPN disconnect. Needs a tunnel that is already up and leaves it down,
# so it takes its own opt-in.
BIEBIE_TEST_MACOS_VPN="VPN CAT" BIEBIE_TEST_MACOS_VPN_DISCONNECT=1 \
  go test ./internal/vpn/macosvpn/ -run Disconnect -v -timeout 2m
```

---

## Scope

**Phase 1 (this release)** — dashboard, connection CRUD, connection groups with
hidden-by-default, provider selection,
host/port config, multiple credentials per connection with UUID identity, secure
password storage, default and last-used credential, provider detection,
connect/disconnect/status abstraction, connection state machine, safe logging,
native client fallback, Ivanti Network Doctor on macOS (read-only: recovery
file + routing table, “This customer takes over all traffic”).

**Not yet implemented** — VM isolation, SSH tunnels, Kubernetes,
cloud accounts, database connections, browser proxy. Profiles, credentials,
sessions, providers and target resources are kept as separate domain concepts so
these can be added as peers of VPN rather than bolted onto it.
