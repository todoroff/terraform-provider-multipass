## Releasing `terraform-provider-multipass`

This project uses **semantic versioning** (`vMAJOR.MINOR.PATCH`) and **GoReleaser + GitHub Actions** to build and publish release artifacts.

### 1. Pre-release checklist

- [ ] All CI checks passing on the `main` branch.
- [ ] `go test ./...` passes locally on your dev machine.
- [ ] CHANGELOG or GitHub Release notes drafted (at least a short summary).

### 2. Choose the next version

Decide whether the release is:

- **PATCH** (`v0.1.1`) – bug fixes, no breaking changes.
- **MINOR** (`v0.2.0`) – new features, backwards compatible.
- **MAJOR** (`v1.0.0`) – breaking changes.

Update any version references if needed (e.g., example `required_providers` blocks once published).

### 3. Tag the release

From the `main` branch:

```bash
git pull origin main
git tag v1.0.0
git push origin v1.0.0
```

> Replace `v1.0.0` with the chosen version.

### 4. GitHub Actions + GoReleaser

Pushing a tag matching `v*.*.*` triggers the `release` workflow:

- Runs tests via the GoReleaser `before` hook.
- Cross-compiles the provider for `linux`, `darwin`, and `windows` on `amd64` and `arm64`.
- Uploads zipped binaries and checksums to the GitHub Release that corresponds to the tag.

You can monitor progress under **GitHub → Actions → Release**.

### 5. Optional: run GoReleaser locally

To test the release process without publishing:

```bash
goreleaser release --snapshot --clean
```

This writes artifacts into `dist/` using the current version information, but does not publish to GitHub.

### 6. Terraform / OpenTofu Registry

Once registry integration is configured:

- Point the registry at the GitHub Releases this workflow produces.
- Keep the provider `source` (`todoroff/multipass`) and published versions in sync with your tags.

### 7. Post-release

- Announce the release (Release notes, README badges, etc.).
- Open follow-up issues for any bugs or deferred items discovered during release.


