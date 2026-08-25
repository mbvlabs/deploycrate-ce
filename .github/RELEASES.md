# R2 release environments

Configure GitHub environments named `edge` and `stable`. Each needs:

- Variable `R2_BUCKET`: destination bucket name.
- Variable `R2_PUBLIC_URL`: public HTTPS bucket/custom-domain URL.
- Secret `R2_ENDPOINT`: R2 S3 API endpoint.
- Secrets `R2_ACCESS_KEY_ID` and `R2_SECRET_ACCESS_KEY`: bucket-scoped credentials.

Protect the `stable` environment. Successful `master` CI runs publish edge; `v*` tags publish stable. Artifacts are uploaded under immutable `releases/<version>/` paths before `manifest.json` is replaced.
