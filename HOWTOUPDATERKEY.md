# Tobiro updater signing key

Tobiro runs on **Wails v3** with the in-app updater ([guide](https://wails.io/guides/updater/)), same model as biebie-kube: GitHub Releases ship platform `.zip` files, sibling `.zip.sig`, and `SHA256SUMS`. The app pins **`updater.key.pub`** at build time; CI signs with the private key.

## Generate a keypair (once)

```bash
cd tobiro
wails3 updater genkey -out updater.key
```

| File | Commit? |
| --- | --- |
| `updater.key` | **Never** — GitHub secret `UPDATER_SIGNING_KEY` |
| `updater.key.pub` | **Yes** — embedded in `update.go` |

## GitHub secret

Full PEM of `updater.key`:

```bash
pbcopy < updater.key
```

## Sign release zips in CI

```bash
go run ./cmd/sign-release Tobiro-v0.0.1-darwin-universal.zip
# or: wails3 updater sign -key updater.key ...
```

Upload `.zip` + `.zip.sig` to the release.

## Dev / build

```bash
wails3 generate bindings -ts
cd frontend && npm install && npm run build
wails3 dev   # after `wails3 update build-assets` if you add the Taskfile layout
```

See [v2 → v3 migration](https://wails.io/migration/v2-to-v3/).
