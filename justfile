set shell := ["bash", "-cu"]

# Build and publish an unsigned edge release to get-dev for migrating a running instance.
development-assets:
    scripts/publish-development-release.sh
