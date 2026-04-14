# ─── Soroban Runner Container ─────────────────────────────────────
# This container stays alive and receives `docker exec` commands
# from the backend to compile Soroban contracts.
#
# NOTE: First build will take 15-20 minutes as it compiles stellar-cli
# from source. Subsequent builds will be cached by Docker.

FROM rust:1.94-slim-bookworm

# Install dependency dasar (tetap dibutuhkan untuk workflow kamu)
RUN apt-get update && apt-get install -y \
    curl \
    ca-certificates \
    build-essential \
    pkg-config \
    libssl-dev \
    libudev-dev \
    libusb-1.0-0-dev \
    libhidapi-dev \
    libdbus-1-dev \
    && rm -rf /var/lib/apt/lists/*

# Add WASM target (ini tetap penting buat Soroban contract)
RUN rustup target add wasm32v1-none

# ✅ Install stellar-cli pre-built binary
RUN ARCH=$(dpkg --print-architecture) && \
    if [ "$ARCH" = "amd64" ]; then \
      STELLAR_ARCH="x86_64-unknown-linux-gnu"; \
    elif [ "$ARCH" = "arm64" ]; then \
      STELLAR_ARCH="aarch64-unknown-linux-gnu"; \
    else \
      echo "Unsupported architecture: $ARCH" && exit 1; \
    fi && \
    echo "Downloading stellar-cli for $STELLAR_ARCH..." && \
    curl -fSL -o /tmp/stellar-cli.tar.gz \
      "https://github.com/stellar/stellar-cli/releases/download/v25.2.0/stellar-cli-25.2.0-${STELLAR_ARCH}.tar.gz" && \
    tar -xzf /tmp/stellar-cli.tar.gz -C /tmp && \
    find /tmp -name "stellar" -type f -exec mv {} /usr/local/bin/stellar \; && \
    chmod +x /usr/local/bin/stellar && \
    rm -rf /tmp/stellar-cli.tar.gz

# ✅ Install sccache for global build caching
RUN ARCH=$(dpkg --print-architecture) && \
    if [ "$ARCH" = "amd64" ]; then \
      SCCACHE_ARCH="x86_64-unknown-linux-musl"; \
    elif [ "$ARCH" = "arm64" ]; then \
      SCCACHE_ARCH="aarch64-unknown-linux-musl"; \
    else \
      echo "Unsupported architecture: $ARCH" && exit 1; \
    fi && \
    curl -fSL -o /tmp/sccache.tar.gz \
      "https://github.com/mozilla/sccache/releases/download/v0.10.0/sccache-v0.10.0-${SCCACHE_ARCH}.tar.gz" && \
    tar -xzf /tmp/sccache.tar.gz -C /tmp && \
    mv /tmp/sccache-v0.10.0-${SCCACHE_ARCH}/sccache /usr/local/bin/sccache && \
    chmod +x /usr/local/bin/sccache && \
    rm -rf /tmp/sccache.tar.gz /tmp/sccache-v0.10.0-${SCCACHE_ARCH}

# Set sccache and cargo environment variables
ENV RUSTC_WRAPPER=sccache
ENV SCCACHE_DIR=/app/sccache
ENV SCCACHE_BASE_DIR=/app/workspaces
ENV CARGO_HOME=/app/cargo
ENV CARGO_HOME=/app/cargo
ENV SCCACHE_IDLE_TIMEOUT=0
ENV SCCACHE_SERVER_UDS=/tmp/sccache.socket

# Workspace and Cache
RUN mkdir -p /app/workspaces /app/sccache /app/cargo && \
    chmod 777 /app/sccache /app/cargo
WORKDIR /app/workspaces

# Start sccache server and keep container alive
CMD ["sh", "-c", "sccache --start-server && tail -f /dev/null"]
