# Releasing Tobiro

A release is a git tag. Pushing `vX.Y.Z` runs `.github/workflows/release.yml`,
which builds macOS (.dmg + signed updater zip) and Windows (NSIS + signed zip),
then opens a **draft** GitHub release.

Publish the draft only after you check the assets. Until then, `/releases/latest`
and the in-app updater ignore it.

First release example: **v0.0.1**.

## Before the first CI run

Add these **GitHub repository secrets** on [ncmink/tobiro](https://github.com/ncmink/tobiro):

| Secret | Purpose |
| --- | --- |
| `APPLE_DEVELOPER_CERTIFICATE_P12_BASE64` | Developer ID `.p12` (same as biebie-kube if you reuse the cert) |
| `APPLE_DEVELOPER_CERTIFICATE_PASSWORD` | p12 password |
| `APPLE_NOTARIZATION_KEY_P8_BASE64` | App Store Connect API `.p8` (base64) |
| `APPLE_API_KEY_ID` | Key ID |
| `APPLE_API_ISSUER_ID` | Issuer ID |
| `UPDATER_SIGNING_KEY` | Full PEM text of `updater.key` (pair of committed `updater.key.pub`) |
| `PROTOCOL_TOKEN` | PAT to clone private `ncmink/biebie-protocol` |
| `ES_USERNAME` | SSL.com account username (eSigner Authenticode signing of `Tobiro.exe` + installer) |
| `ES_PASSWORD` | SSL.com account password |
| `CREDENTIAL_ID` | eSigner credential ID (order page → **Show my signing credentials**) |
| `ES_TOTP_SECRET` | Base32 secret from the eSigner OTP QR code (`secret=...`), not a 6-digit code |

Each release spends two eSigner signings (app exe + installer).

## eSigner signing quota (SSL.com)

Tobiro’s Windows job calls eSigner twice per successful release (Authenticode on
`Tobiro.exe` and on the NSIS installer). Test runs and re-runs of the Windows job
also consume signings.

**Where to check usage**

1. Log in at [ssl.com](https://www.ssl.com/) → **Orders** (or open order **co-021lejtg987**).
2. Open the issued **Personal ID Code Signing** order (status **eSigner active**).
3. Scroll to **END ENTITY CERTIFICATES** → **Show Details**.
4. Read **Signings This Month** (total signings on this certificate for the current
   month). On **Current eSigner Plan**, confirm tier (**Tier 1 Annual** = **240
   signings / year** with rollover while the subscription stays active).

**Rough budget:** `used ≈ 2 × (number of release Windows jobs that completed both
sign steps)`. Compare **Signings This Month** (and annual allowance) before kicking
off an extra `workflow_dispatch` test.

SSL.com docs: [How to view eSigner signature usage](https://www.ssl.com/guide/esigner-pricing-for-code-signing/).

**Protocol dependency:** Tobiro’s peer RPC sets `AccessProfile.Kind` (`vpn` / `ssh`).
That field must exist on **`ncmink/biebie-protocol` `master`** before release CI can
compile. If CI fails with `unknown field Kind`, push the protocol repo first (see
below), then re-run the release workflow.

```bash
cd ../biebie-protocol   # sibling of tobiro in the monorepo checkout
git add context/access.go
git commit -m "Add AccessProfile.Kind for VPN vs SSH in peer profile lists"
git push origin master
```

See `HOWTOUPDATERKEY.md` for generating `updater.key`.

## Short version (v0.0.1)

```bash
cd tobiro

# 1. Align version strings (already 0.0.1 for first ship)
#    build/config.yml → info.version
#    app.go → appVersion
#    wails.json → info.productVersion

wails3 task common:update:build-assets
# Restore CFBundleURLName in build/darwin/Info*.plist if the generator reset it:
#   <string>net.tobiro.access</string>  (or keep wails.com.tobiro if you prefer)

go test ./...
(cd frontend && npm run build)

# 2. Commit migration + release workflow
git add -A
git commit -m "Prepare v0.0.1 release pipeline"

git push origin master   # or your default branch
git tag v0.0.1
git push origin v0.0.1

# 3. Watch CI, then publish draft
gh run watch "$(gh run list --workflow=release.yml --limit=1 --json databaseId -q '.[0].databaseId')"
gh release view v0.0.1 --json assets -q '.assets[].name'
gh release edit v0.0.1 --draft=false
```

## Expected release assets

```text
Tobiro-v0.0.1-macos-universal.dmg
Tobiro-v0.0.1-darwin-universal.zip
Tobiro-v0.0.1-darwin-universal.zip.sig
Tobiro-v0.0.1-windows-amd64-installer.exe
Tobiro-v0.0.1-windows-amd64.zip
Tobiro-v0.0.1-windows-amd64.zip.sig
SHA256SUMS
```

The Wails updater uses the **`.zip` + `.sig`** files and **`SHA256SUMS`**. The
`.dmg` / `.exe` are for fresh installs.

## Manual dispatch (without a tag)

```bash
gh workflow run release.yml -f version=v0.0.1
```

Builds the ref you dispatch from; tag name on the draft comes from the input.

## Local dev (not a release)

```bash
wails3 task dev
```
