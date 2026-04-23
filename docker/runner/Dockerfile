# ─── Soroban & Frontend Runner Container ──────────────────────────
# This container stays alive and receives `docker exec` commands
# from the backend to compile Soroban contracts and run Node.js apps.

FROM rust:1.94-slim-bookworm

# Install basic dependencies + Node.js setup
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
    gnupg \
    procps \
    && curl -fsSL https://deb.nodesource.com/setup_20.x | bash - \
    && apt-get install -y nodejs \
    && corepack enable && corepack prepare pnpm@latest --activate \
    && rm -rf /var/lib/apt/lists/*

# Add WASM target for Soroban
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

# Set environment variables for caching and ports
ENV RUSTC_WRAPPER=sccache
ENV SCCACHE_DIR=/app/sccache
ENV SCCACHE_BASE_DIR=/app/workspaces
ENV CARGO_HOME=/app/cargo
ENV SCCACHE_IDLE_TIMEOUT=0
ENV SCCACHE_SERVER_UDS=/tmp/sccache.socket

# PNPM & Node Configuration
ENV PNPM_HOME=/usr/local/share/pnpm
ENV PATH="$PNPM_HOME:$PATH"
RUN pnpm config set store-dir /app/pnpm-store

# Workspace and Cache
RUN mkdir -p /app/workspaces /app/sccache /app/cargo /app/pnpm-store && \
    chmod 777 /app/sccache /app/cargo /app/pnpm-store

# Workspace and Cache
RUN mkdir -p /app/workspaces /app/sccache /app/cargo /app/pnpm-store && \
    chmod 777 /app/sccache /app/cargo /app/pnpm-store

# ─── Shared Node Modules (Hoisting) ───
# We pre-install common dependencies at the parent level (/app).
# Node.js resolution will walk up from /app/workspaces/:session/:name 
# and find these in /app/node_modules.
WORKDIR /app
RUN pnpm add react react-dom vite @vitejs/plugin-react tailwindcss postcss autoprefixer lucide-react @stellar/stellar-sdk

WORKDIR /app/workspaces

# Start sccache server and keep container alive
CMD ["sh", "-c", "sccache --start-server && tail -f /dev/null"]
