# TSGW - Tailscale HTTPS Load Balancer

A Go application that acts as an HTTPS load balancer for remote endpoints accessible via Tailscale network.

[![Go Version](https://img.shields.io/badge/go-1.25.1+-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

## Quick Start

1. **Install**
   ```bash
   go install github.com/AndreZiviani/tsgw@latest
   ```

2. **Setup OAuth Client** - Follow the detailed steps in [OAuth Setup](#oauth-setup) below

3. **Configure and Run**
   ```bash
   ./tsgw \
     --tailscale-domain "your-domain.ts.net" \
     --oauth-client-id "tskey-client-xxxxxxxxxxxxxx" \
     --oauth-client-secret "tskey-secret-xxxxxxxxxxxxxx" \
     --route "app=http://app.internal:8080" \
     --route "api=https://api.internal:3000"
   ```

4. **Access**
   - `https://app.your-domain.ts.net` → `http://app.internal:8080`
   - `https://api.your-domain.ts.net` → `https://api.internal:3000`

## Features

- **🔐 Automatic Authentication**: OAuth 2.0 with token refresh
- **🏗️ Isolated Architecture**: One Tailscale node per route with dedicated Echo instance
- **🔒 Automatic HTTPS**: Tailscale-managed certificates
- **🎯 Host-Based Routing**: Route by Host header
- **⚡ Optimized Proxy**: Pre-configured proxy setup (no per-request overhead)
- **📊 Observability**: Optional OpenTelemetry support
- **🐳 Container Ready**: Docker and Kubernetes support
- **⚙️ CLI-First Config**: Environment variables and CLI flags only

## OAuth Setup

TSGW requires OAuth client credentials to manage Tailscale devices via the API. Follow these steps to create the necessary tag and OAuth client:

### Step 1: Create Tag in ACL Policy

1. Open your [Tailnet Policy File](https://login.tailscale.com/admin/acls/file) in the Tailscale admin console
2. Add the following tags to the `tagOwners` section:

```json
{
  "tagOwners": {
    "tag:tsgw": [],
    // ... your other tags
  }
}
```

3. **Save** the policy file

### Step 2: Create OAuth Client

1. Go to [OAuth Clients](https://login.tailscale.com/admin/settings/oauth) in the Tailscale admin console
2. Click **"Generate OAuth Client"**
3. Configure the client:
   - **Name**: `TSGW Load Balancer` (or any descriptive name)
   - **Scopes**: Select both:
     - ✅ **Devices** (to create and manage Tailscale machines)
     - ✅ **Auth Keys** (to generate authentication keys)
   - **Tags**: Enter `tag:tsgw`
4. Click **"Generate Client"**
5. **Copy and save** the Client ID and Client Secret immediately (the secret won't be shown again)

### Step 3: Test OAuth Setup

Verify your OAuth client works:

```bash
# Test API access with your credentials
curl -u "tskey-client-xxxxx:tskey-secret-xxxxx" \
     "https://api.tailscale.com/api/v2/tailnet/-/devices"
```

If successful, you'll see a JSON response with your tailnet's devices.

## Configuration

### Basic Configuration File

```yaml
## Configuration

TSGW uses a CLI-first configuration approach with environment variable support. No config files are supported.

### Required Configuration

```bash
# Required: Tailscale domain
--tailscale-domain "your-domain.ts.net" \
# Required: OAuth credentials
--oauth-client-id "tskey-client-xxxxxxxxxxxxxx" \
--oauth-client-secret "tskey-secret-xxxxxxxxxxxxxx" \
# Required: At least one route
--route "app=http://app.internal:8080"
```

### Environment Variables

```bash
# Required
export TSGW_TAILSCALE_DOMAIN="your-domain.ts.net"
export TSGW_OAUTH_CLIENT_ID="tskey-client-xxxxxxxxxxxxxx"
export TSGW_OAUTH_CLIENT_SECRET="tskey-secret-xxxxxxxxxxxxxx"

# Routes - Option 1: Single environment variable with space-separated routes
export TSGW_ROUTES="app=http://app.internal:8080 api=https://api.internal:3000 web=http://web.internal:8080"

# Routes - Option 2: Individual environment variables (overrides TSGW_ROUTES if both are set)
export TSGW_ROUTE_APP="http://app.internal:8080"
export TSGW_ROUTE_API="https://api.internal:3000"
export TSGW_ROUTE_WEB="http://web.internal:8080"

# Optional
export TSGW_HOSTNAME="tsgw"                    # Base hostname for nodes
export TSGW_PORT=443                          # Listen port
export TSGW_LOG_LEVEL="info"                  # trace, debug, info, warn, error
export TSGW_LOG_FORMAT="console"              # console or json
export TSGW_SKIP_TLS_VERIFY="false"           # Skip backend TLS verification
export TSGW_LISTEN_ADDRESS=""                 # Optional regular network listener
export TSGW_TSNET_DIR="./tsnet"               # Tailscale state directory
export TSGW_FORCE_CLEANUP="false"             # Force cleanup of existing state

# OpenTelemetry (optional)
export TSGW_OTEL_ENABLED="false"
export TSGW_OTEL_SERVICE_NAME="tsgw"
export TSGW_OTEL_ENDPOINT="localhost:4317"
export TSGW_OTEL_PROTOCOL="grpc"
export TSGW_OTEL_INSECURE="false"
```

### CLI Flags

```bash
./tsgw \
  --tailscale-domain "your-domain.ts.net" \
  --oauth-client-id "tskey-client-xxxxxxxxxxxxxx" \
  --oauth-client-secret "tskey-secret-xxxxxxxxxxxxxx" \
  --route "app=http://app.internal:8080" \
  --route "api=https://api.internal:3000" \
  --route "web=http://web.internal:8080" \
  --log-level "info" \
  --tsnet-dir "./tsnet"
```

### Route Configuration

Routes are specified in `name=backend_url` format:

```bash
# Examples
--route "app=http://app.internal:8080"
--route "api=https://api.internal:3000"
--route "web=http://web.internal:8080"
--route "secure-app=https://secure-app.internal:8443"
```

Each route creates:
- A dedicated Tailscale node: `app.your-domain.ts.net`
- A dedicated Echo instance for that route
- Pre-configured proxy to the backend URL

**Priority**: Environment Variables > CLI Flags
```

### Environment Variables

```bash
# Core
export TSGW_TAILSCALE_DOMAIN="your-domain.ts.net"
export TSGW_OAUTH_CLIENT_ID="tskey-client-xxx"
export TSGW_OAUTH_CLIENT_SECRET="tskey-secret-xxx"
export TSGW_TSNET_DIR="/persistent/path"  # ⚠️ CRITICAL

# Routes
export TSGW_ROUTE_APP="http://app.internal:8080"
export TSGW_ROUTE_API="https://api.internal:3000"

# Optional
export TSGW_LOG_LEVEL="info"
export TSGW_OTEL_ENABLED="true"
```

### CLI Flags

```bash
./tsgw \
       --tailscale-domain "your-domain.ts.net" \
       --oauth-client-id "xxx" \
       --oauth-client-secret "xxx" \
       --tsnet-dir "/persistent/path" \
       --force-cleanup \
       --route "app=http://app.internal:8080" \
       --route "api=https://api.internal:3000"
```

**Priority**: Environment Variables > CLI Flags

## Storage

### ⚠️ **CRITICAL: Persistent Storage Required**

The `tsnet_dir` contains Tailscale machine state and **MUST be persistent**. Loss results in:
- Lost machine registrations
- Certificate renewal failures  
- Service disruption

**Always use persistent storage in production.**

## Deployment

### Docker

```bash
# Build
docker build -t tsgw .

# Run with persistent storage
docker run \
  -e TSGW_TAILSCALE_DOMAIN="your-domain.ts.net" \
  -e TSGW_OAUTH_CLIENT_ID="tskey-client-xxxxxxxxxxxxxx" \
  -e TSGW_OAUTH_CLIENT_SECRET="tskey-secret-xxxxxxxxxxxxxx" \
  -e TSGW_ROUTES="app=http://app.internal:8080 api=https://api.internal:3000" \
  -e TSGW_TSNET_DIR="/app/tsnet" \
  -v tsgw_data:/app/tsnet \
  tsgw
```

### Kubernetes

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: tsgw
spec:
  replicas: 1
  selector:
    matchLabels:
      app: tsgw
  template:
    metadata:
      labels:
        app: tsgw
    spec:
      containers:
      - name: tsgw
        image: tsgw:latest
        env:
        - name: TSGW_TAILSCALE_DOMAIN
          value: "your-domain.ts.net"
        - name: TSGW_OAUTH_CLIENT_ID
          valueFrom:
            secretKeyRef:
              name: tsgw-oauth
              key: client-id
        - name: TSGW_OAUTH_CLIENT_SECRET
          valueFrom:
            secretKeyRef:
              name: tsgw-oauth
              key: client-secret
        - name: TSGW_TSNET_DIR
          value: "/data/tsgw"
        # Routes - Option 1: Single env var with space-separated routes
        - name: TSGW_ROUTES
          value: "app=http://app.internal:8080 api=https://api.internal:3000"
        # Routes - Option 2: Individual route env vars (alternative)
        # - name: TSGW_ROUTE_APP
        #   value: "http://app.internal:8080"
        # - name: TSGW_ROUTE_API
        #   value: "https://api.internal:3000"
        volumeMounts:
        - name: tsgw-storage
          mountPath: /data/tsgw
      volumes:
      - name: tsgw-storage
        persistentVolumeClaim:
          claimName: tsgw-pvc
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: tsgw-pvc
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
```

### Command Line

```bash
# Basic usage
./tsgw \
  --tailscale-domain "your-domain.ts.net" \
  --oauth-client-id "tskey-client-xxxxxxxxxxxxxx" \
  --oauth-client-secret "tskey-secret-xxxxxxxxxxxxxx" \
  --route "app=http://app.internal:8080"

# With multiple routes and options
./tsgw \
  --tailscale-domain "your-domain.ts.net" \
  --oauth-client-id "tskey-client-xxxxxxxxxxxxxx" \
  --oauth-client-secret "tskey-secret-xxxxxxxxxxxxxx" \
  --route "app=http://app.internal:8080" \
  --route "api=https://api.internal:3000" \
  --route "web=http://web.internal:8080" \
  --log-level "debug" \
  --otel-enabled \
  --otel-endpoint "localhost:4317"
```

## Troubleshooting

### Common Issues

**Machines removed from Tailscale console but cached locally**
```bash
# Force cleanup of invalid state files
./tsgw --force-cleanup --log-level debug

# Or set environment variable
export TSGW_FORCE_CLEANUP=true
./tsgw
```

**Machines not appearing in Tailscale admin**
```bash
# Check OAuth credentials with debug logging
./tsgw \
  --tailscale-domain "your-domain.ts.net" \
  --oauth-client-id "tskey-client-xxxxxxxxxxxxxx" \
  --oauth-client-secret "tskey-secret-xxxxxxxxxxxxxx" \
  --route "test=http://httpbin.org" \
  --log-level "debug"

# Verify API access
curl -H "Authorization: Bearer $(tailscale auth)" \
     https://api.tailscale.com/api/v2/tailnet/-/devices
```

**Connection failures**
```bash
# Check tsnet directory permissions
ls -la ./tsnet/

# Verify backend connectivity
curl -v http://your-backend:8080/health

# Check certificates
openssl s_client -connect app.your-domain.ts.net:443
```

**High memory usage**
```bash
# Check for certificate rotation issues
./tsgw \
  --tailscale-domain "your-domain.ts.net" \
  --oauth-client-id "tskey-client-xxxxxxxxxxxxxx" \
  --oauth-client-secret "tskey-secret-xxxxxxxxxxxxxx" \
  --route "test=http://httpbin.org" \
  --log-level "debug" 2>&1 | grep -i cert

# Monitor with observability
./tsgw \
  --tailscale-domain "your-domain.ts.net" \
  --oauth-client-id "tskey-client-xxxxxxxxxxxxxx" \
  --oauth-client-secret "tskey-secret-xxxxxxxxxxxxxx" \
  --route "test=http://httpbin.org" \
  --otel-enabled \
  --otel-endpoint "localhost:4317"
```

### Logs

```bash
# Enable debug logging
./tsgw \
  --tailscale-domain "your-domain.ts.net" \
  --oauth-client-id "tskey-client-xxxxxxxxxxxxxx" \
  --oauth-client-secret "tskey-secret-xxxxxxxxxxxxxx" \
  --route "test=http://httpbin.org" \
  --log-level "debug"

# JSON format for production
./tsgw \
  --tailscale-domain "your-domain.ts.net" \
  --oauth-client-id "tskey-client-xxxxxxxxxxxxxx" \
  --oauth-client-secret "tskey-secret-xxxxxxxxxxxxxx" \
  --route "test=http://httpbin.org" \
  --log-level "info" \
  --log-format "json"

# Check specific route
./tsgw \
  --tailscale-domain "your-domain.ts.net" \
  --oauth-client-id "tskey-client-xxxxxxxxxxxxxx" \
  --oauth-client-secret "tskey-secret-xxxxxxxxxxxxxx" \
  --route "test=http://httpbin.org" \
  --log-level "trace"
```

## Requirements

- Go 1.25.1+
- Tailscale account with OAuth client
- Persistent storage for production

## License

MIT License - see [LICENSE](LICENSE) file.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

For detailed development setup, see the original README or documentation.
