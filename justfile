set shell := ["bash", "-cu"]

# Build upload-ready Linux AMD64 development binaries.
development-assets:
    #!/usr/bin/env bash
    set -euo pipefail

    version="development-$(git describe --always --dirty)"
    output_root="dist/deploycrate-ce-development"
    cli_output="${output_root}/dc-ce-cli"
    app_output="${output_root}/dc-ce-app"

    install -d "${cli_output}" "${app_output}"

    GOARCH=amd64 andurel build --version "${version}"
    mv deploycrate-ce "${app_output}/deploycrate-ce"

    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
        -trimpath \
        -ldflags "-s -w -X main.version=${version}" \
        -o "${cli_output}/deploycrate" \
        ./cmd/deploycrate

    (
        cd "${cli_output}"
        sha256sum deploycrate > deploycrate.sha256
    )
    (
        cd "${app_output}"
        sha256sum deploycrate-ce > deploycrate-ce.sha256
    )
    install -m 0644 scripts/install-development.sh "${output_root}/install.sh"

    printf 'Development assets generated in %s\n' "${output_root}"
