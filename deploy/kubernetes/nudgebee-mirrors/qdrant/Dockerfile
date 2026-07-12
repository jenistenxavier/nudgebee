# Patched ghcr mirror of the official Qdrant image (the vector DB deployed
# alongside nudgebee). We mirror to ghcr.io/nudgebee/* rather than pulling
# docker.io directly to avoid Docker Hub rate limits/pull issues.
#
# Re-mirror of docker.io/qdrant/qdrant + an apt upgrade layer to pull current
# Debian security patches (same qdrant version + base -- no behaviour change).
# Bump OS_PKG_EPOCH (ISO week) to force a weekly re-pull. The image runs as root
# at build, so no USER switch is needed for apt.
ARG OS_PKG_EPOCH=2026-W28
FROM docker.io/qdrant/qdrant:v1.18.2

# Re-declare in the build-stage scope (an ARG before FROM is only visible to FROM
# lines).
ARG OS_PKG_EPOCH

USER root
RUN echo "os-pkg-epoch: ${OS_PKG_EPOCH}" \
    && apt-get update \
    && apt-get upgrade -y \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*
