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

3. **Configure**
   ```yaml
   # config.yaml
   tailscale_domain: "your-domain.ts.net"
   oauth:
     client_id: "tskey-client-xxxxxxxxxxxxxx"
     client_secret: "tskey-secret-xxxxxxxxxxxxxx"
   routes:
     "app": "http://app.internal:8080"
     "api": "https://api.internal:3000"
   ```

4. **Run**
   ```bash
   ./tsgw
   ```

5. **Access**
   - `https://app.your-domain.ts.net` → `http://app.internal:8080`
   - `https://api.your-domain.ts.net` → `https://api.internal:3000`

## Features

- **🔐 Automatic Authentication**: OAuth 2.0 with token refresh
- **🏗️ Multi-Node Architecture**: One Tailscale node per route for isolation
- **🔒 Automatic HTTPS**: Tailscale-managed certificates
- **🎯 Host-Based Routing**: Route by Host header
- **📊 Observability**: Optional OpenTelemetry support
- **🐳 Container Ready**: Docker and Kubernetes support
- **⚙️ Flexible Config**: CLI, env vars, and config files

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
# config.yaml
hostname: "tsgw"                    # Base hostname for nodes
tailscale_domain: "example.ts.net"  # Your Tailscale domain
port: 443                          # Listen port
tsnet_dir: "./tsnet"               # ⚠️ MUST BE PERSISTENT

# OAuth (required)
oauth:
  client_id: "tskey-client-xxxxxxxxxxxxxx"
  client_secret: "tskey-secret-xxxxxxxxxxxxxx"

# Routes (required)
routes:
  "app1": "http://app1.internal:8080"
  "api": "https://api.internal:3000"
  "web": "http://web.internal:8080"

# Optional features
log_level: "info"        # trace, debug, info, warn, error
log_format: "console"    # console or json
skip_tls_verify: false   # Skip backend TLS verification

# OpenTelemetry (optional)
opentelemetry:
  enabled: false
  endpoint: "localhost:4317"
  service_name: "tsgw"
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
./tsgw --config config.yaml \
       --tailscale-domain "your-domain.ts.net" \
       --oauth-client-id "xxx" \
       --oauth-client-secret "xxx" \
       --tsnet-dir "/persistent/path" \
       --route "app=http://app.internal:8080" \
       --route "api=https://api.internal:3000"
```

**Priority**: Environment Variables > CLI Flags > Config File

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
docker run -v tsgw_data:/app/tsnet \
  -e TSGW_TAILSCALE_DOMAIN="your-domain.ts.net" \
  -e TSGW_OAUTH_CLIENT_ID="xxx" \
  -e TSGW_OAUTH_CLIENT_SECRET="xxx" \
  -e TSGW_TSNET_DIR="/app/tsnet" \
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

## Troubleshooting

### Common Issues

**Machines not appearing in Tailscale admin**
```bash
# Check OAuth credentials
./tsgw --log-level debug

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
./tsgw --log-level debug 2>&1 | grep -i cert

# Monitor with observability
./tsgw --otel-enabled --otel-endpoint localhost:4317
```

### Logs

```bash
# Enable debug logging
export TSGW_LOG_LEVEL=debug

# JSON format for production
export TSGW_LOG_FORMAT=json

# Check specific route
./tsgw --log-level trace --route "test=http://httpbin.org"
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