# R2 release environments

Configure GitHub environments named `edge` and `stable`. Each needs:

- Variable `R2_BUCKET`: destination bucket name.
- Variable `R2_PUBLIC_URL`: public HTTPS bucket/custom-domain URL.
- Secret `R2_ENDPOINT`: R2 S3 API endpoint.
- Secrets `R2_ACCESS_KEY_ID` and `R2_SECRET_ACCESS_KEY`: bucket-scoped credentials.

The credentials need object read/write access and permission to list the bucket. Protect the `stable` environment and restrict stable tags to trusted maintainers.

Successful `master` CI runs publish edge; `v*` tags publish stable only when the tagged commit belongs to `master`. Both publishing jobs need `id-token: write` so Cosign can obtain a short-lived Fulcio certificate from GitHub's OIDC identity. No signing key or signing secret is stored in GitHub.

The workflows sign and immediately verify `manifest.json`, both architecture-specific bootstrap binaries, and both application binaries. Verification pins the certificate issuer to `https://token.actions.githubusercontent.com` and the certificate identity to the exact publishing workflow reference. Sigstore bundles use the `.sigstore.json` suffix and are published beside their authenticated files.

Artifacts are uploaded under immutable `releases/<version>/` paths before the channel's `manifest.json.sigstore.json` and `manifest.json` are replaced. The manifest is uploaded last, so clients cannot observe a new release before its artifacts and signature bundles exist. Stable publication refuses to overwrite an existing version path.

Installers bootstrap Cosign 3.0.6 from the official Sigstore GitHub release and verify its pinned AMD64 or ARM64 SHA-256 digest before using it. The application and bootstrap use the same pinned verifier; if the installed copy is missing, they download a temporary copy and verify that digest. Updating Cosign requires changing the version and both official release digests in `scripts/install-channel.sh` and `internal/releaseauth/verifier.go` together.
