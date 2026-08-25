set shell := ["bash", "-cu"]

# Build the same edge artifacts produced by GitHub Actions, without publishing them.
development-assets:
    scripts/build-release.sh "edge-$(git rev-parse --short=12 HEAD)" edge "https://get-dev.deploycrate.com"

# Build a local stable release snapshot without publishing it.
stable-assets version:
    scripts/build-release.sh "{{version}}" stable "https://get.deploycrate.com"
