FROM debian:trixie-slim AS python-base
LABEL org.nudgebee.image.authors="dev@nudgebee.com"
LABEL maintainer="nudgebee"

ARG TARGETARCH
ENV PYTHONDONTWRITEBYTECODE=1
ENV PYTHONUNBUFFERED=1
ENV CONDA_DIR="/opt/conda"
ENV PATH="${CONDA_DIR}/bin:${CONDA_DIR}/envs/myenv/bin:$PATH"

# Install system dependencies, Miniforge, and setup environment.
# `apt-get upgrade` patches the base Debian packages to the latest trixie
# security releases — without it the image ships whatever debian:trixie-slim
# had when the tag was cut (which had drifted far behind: openssl, libgnutls,
# libpcre2, ... all with fixes available).
# DEBIAN_FRONTEND=noninteractive keeps apt from opening debconf prompts (which
# `upgrade` can trigger for reconfigured packages) and hanging the non-TTY CI
# build. Scoped to this RUN via `export` so it does not persist into the image.
RUN export DEBIAN_FRONTEND=noninteractive && \
    apt-get update && apt-get upgrade -y && apt-get install -y --no-install-recommends \
    bash build-essential bzip2 ca-certificates curl && \
    rm -rf /var/lib/apt/lists/* && \
    # Install Miniforge
    if [ "$TARGETARCH" = "arm64" ]; then \
        curl -fsSL "https://github.com/conda-forge/miniforge/releases/latest/download/Miniforge3-Linux-aarch64.sh" -o miniforge.sh; \
    else \
        curl -fsSL "https://github.com/conda-forge/miniforge/releases/latest/download/Miniforge3-Linux-x86_64.sh" -o miniforge.sh; \
    fi && \
    bash miniforge.sh -b -p "${CONDA_DIR}" && \
    rm miniforge.sh && \
    # Setup environment and install packages
    "${CONDA_DIR}/bin/conda" update -n base -c conda-forge conda -y && \
    "${CONDA_DIR}/bin/conda" create -n myenv python=3.11.8 -c conda-forge -y && \
    "${CONDA_DIR}/bin/conda" install -n myenv -c conda-forge pulp -y && \
    "${CONDA_DIR}/bin/conda" run -n myenv pip install uv && \
    "${CONDA_DIR}/bin/conda" run -n myenv pip uninstall -y importlib-metadata || true && \
    "${CONDA_DIR}/bin/conda" clean --yes --all && \
    rm -rf /root/.cache
