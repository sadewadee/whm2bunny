# whm2bunny

[![Test](https://github.com/mordenhost/whm2bunny/actions/workflows/test.yml/badge.svg)](https://github.com/mordenhost/whm2bunny/actions/workflows/test.yml)
[![Release](https://github.com/mordenhost/whm2bunny/actions/workflows/release.yml/badge.svg)](https://github.com/mordenhost/whm2bunny/actions/workflows/release.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/mordenhost/whm2bunny)](https://goreportcard.com/report/github.com/mordenhost/whm2bunny)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

**Automated DNS & CDN provisioning system for [mordenhost.com](https://mordenhost.com)**

A production-grade Go daemon that automatically provisions **BunnyDNS Zones** and **BunnyCDN Pull Zones** when new domains are added to WHM/cPanel. Designed specifically for [mordenhost.com](https://mordenhost.com) hosting infrastructure.

---

## Overview

**whm2bunny** bridges your WHM/cPanel server with Bunny.net services, enabling fully automated domain provisioning:

- When a new cPanel account is created → DNS zone + CDN automatically configured
- When an addon domain is added → DNS records + CDN pull zone created
- When a subdomain is created → CDN-enabled automatically
- When an account is terminated → All resources cleaned up

This eliminates manual DNS configuration and CDN setup, reducing provisioning time from ~15 minutes to seconds.

---

## Key Features

| Feature | Description |
|---------|-------------|
| **Auto DNS Provisioning** | Creates BunnyDNS zones with A, CNAME, MX, TXT (SPF/DMARC) records |
| **CDN Integration** | Auto-provisions BunnyCDN pull zones (Asia+Oceania region) |
| **Custom Nameservers** | Uses `ns1.mordenhost.com` and `ns2.mordenhost.com` |
| **WHM/cPanel Hooks** | Seamless integration via standard WHM script hooks |
| **HMAC Security** | All webhooks verified with HMAC-SHA256 signatures |
| **Telegram Notifications** | Real-time alerts for provisioning events |
| **Auto-Recovery** | Failed provisions automatically retry with exponential backoff |
| **SSL Monitoring** | Verifies SSL certificate issuance after CDN setup |
| **Daily Summaries** | Bandwidth statistics delivered to Telegram |
| **Input Validation** | Domain validation with DNS checks (RFC 1035 compliant) |
| **State Persistence** | Survives crashes and restarts with state recovery |

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              WHM/cPanel Server                              │
│                                                                             │
│  ┌──────────────┐     ┌──────────────────┐     ┌────────────────────────┐  │
│  │ New Account  │     │  WHM Hook Script │     │   HMAC-SHA256 Sign     │  │
│  │   Created    │────▶│  (whm_hook.py)   │────▶│   Webhook Payload      │  │
│  └──────────────┘     └──────────────────┘     └────────────────────────┘  │
│                                                          │                  │
└──────────────────────────────────────────────────────────│──────────────────┘
                                                           │
                                                           ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                            whm2bunny Server                                 │
│                                                                             │
│  ┌─────────────────┐                                                       │
│  │ POST /hook      │◀────── Webhook with HMAC signature                    │
│  │ (Verification)  │                                                       │
│  └────────┬────────┘                                                       │
│           │                                                                 │
│           ▼                                                                 │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                    PROVISIONING PIPELINE                             │   │
│  │                                                                      │   │
│  │  Step 1: Create DNS Zone (BunnyDNS)                                 │   │
│  │     └── Creates zone with custom SOA email                          │   │
│  │                                                                      │   │
│  │  Step 2: Add DNS Records                                             │   │
│  │     ├── A record: @ → ORIGIN_IP (your WHM server)                   │   │
│  │     ├── CNAME: www → @                                              │   │
│  │     ├── MX: mail.domain.com (priority 10)                           │   │
│  │     ├── TXT: @ → "v=spf1 a mx -all" (SPF)                           │   │
│  │     └── TXT: _dmarc → "v=DMARC1; p=none" (DMARC)                    │   │
│  │                                                                      │   │
│  │  Step 3: Create Pull Zone (BunnyCDN)                                │   │
│  │     ├── Name: morden-example-com                                    │   │
│  │     ├── Origin: ORIGIN_IP                                           │   │
│  │     ├── Region: Asia+Oceania only                                   │   │
│  │     ├── Origin Shield: Singapore (SG)                               │   │
│  │     ├── Features: AutoSSL, Brotli compression                       │   │
│  │     └── Add hostname: domain.com                                    │   │
│  │                                                                      │   │
│  │  Step 4: Sync CDN CNAME                                              │   │
│  │     └── CNAME: cdn → {pullzone}.bunnycdn.com                        │   │
│  │                                                                      │   │
│  │  Step 5: SSL Certificate Check                                       │   │
│  │     └── Verify SSL issuance (async, 10s delay)                      │   │
│  │                                                                      │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│           │                                                                 │
│           ▼                                                                 │
│  ┌─────────────────┐     ┌─────────────────┐     ┌─────────────────────┐  │
│  │  State Manager  │     │    Scheduler    │     │ Telegram Notifier   │  │
│  │ (persistence)   │     │ (daily summary) │     │ (notifications)     │  │
│  └─────────────────┘     └─────────────────┘     └─────────────────────┘  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Supported Events

| Event | Trigger | Action |
|-------|---------|--------|
| `account_created` | WHM creates new cPanel account | Full provision (DNS + CDN) |
| `addon_created` | User adds addon domain | Full provision (DNS + CDN) |
| `subdomain_created` | User creates subdomain | CDN provision + DNS CNAME (reuses parent zone) |
| `account_deleted` | WHM terminates account | Deprovision (cleanup DNS + CDN) |

---

## Quick Start

### Download Binary

```bash
# Download latest release
curl -sSL https://github.com/mordenhost/whm2bunny/releases/latest/download/whm2bunny-linux-amd64.tar.gz | tar xz

# Make executable
chmod +x whm2bunny

# Generate default config
./whm2bunny config generate config.yaml

# Edit config with your values
vim config.yaml

# Run
./whm2bunny serve
```

### Docker

```bash
docker run -d \
  --name whm2bunny \
  -p 9090:9090 \
  -v /var/lib/whm2bunny:/var/lib/whm2bunny \
  -e BUNNY_API_KEY=your-key \
  -e ORIGIN_IP=your-ip \
  -e WHM_HOOK_SECRET=your-secret \
  -e TELEGRAM_BOT_TOKEN=your-bot-token \
  -e TELEGRAM_CHAT_ID=your-chat-id \
  mordenhost/whm2bunny:latest
```

### Docker Compose

```bash
# Clone repository
git clone https://github.com/mordenhost/whm2bunny.git
cd whm2bunny

# Copy and edit config
cp config.yaml.example config.yaml
vim config.yaml

# Start
docker-compose up -d
```

---

## Configuration

### Environment Variables

| Variable | Required | Description | Default |
|----------|----------|-------------|---------|
| `BUNNY_API_KEY` | Yes | Bunny.net API key | - |
| `ORIGIN_IP` | Yes | IP of WHM/cPanel origin server | - |
| `WHM_HOOK_SECRET` | Yes | HMAC secret for webhook verification | - |
| `SERVER_PORT` | No | HTTP server port | `9090` |
| `TELEGRAM_BOT_TOKEN` | No | Telegram bot token | - |
| `TELEGRAM_CHAT_ID` | No | Telegram chat ID | - |
| `STATE_FILE` | No | Path to state file | `/var/lib/whm2bunny/state.json` |
| `ORIGIN_SHIELD_REGION` | No | Origin shield region | `SG` |
| `SOA_EMAIL` | No | SOA record email | `hostmaster@mordenhost.com` |

### Config File (config.yaml)

```yaml
server:
  port: 9090
  host: "0.0.0.0"

bunny:
  api_key: "${BUNNY_API_KEY}"
  base_url: "https://api.bunny.net"

dns:
  nameserver1: "ns1.mordenhost.com"
  nameserver2: "ns2.mordenhost.com"
  soa_email: "hostmaster@mordenhost.com"

cdn:
  origin_shield_region: "SG"
  regions: [asia]

origin:
  ip: "${ORIGIN_IP}"

webhook:
  secret: "${WHM_HOOK_SECRET}"

telegram:
  enabled: true
  bot_token: "${TELEGRAM_BOT_TOKEN}"
  chat_id: "${TELEGRAM_CHAT_ID}"
  summary:
    enabled: true
    schedule: "0 9 * * *"      # Daily at 9 AM UTC
    weekly_schedule: "0 9 * * 1" # Weekly on Monday

logging:
  level: "info"
  format: "json"
```

---

## WHM/cPanel Integration

### Step 1: Install Hook Script on WHM Server

```bash
# Create directory
mkdir -p /usr/local/cpanel/whm2bunny

# Download hook script
curl -sSL https://raw.githubusercontent.com/mordenhost/whm2bunny/main/scripts/whm_hook.py \
  -o /usr/local/cpanel/whm2bunny/whm_hook.py

# Make executable
chmod +x /usr/local/cpanel/whm2bunny/whm_hook.py

# Create config
mkdir -p /etc/whm2bunny
cat > /etc/whm2bunny/config.json << 'EOF'
{
  "webhook_url": "http://your-whm2bunny-server:9090/hook",
  "secret": "your-webhook-secret-here",
  "timeout": 30,
  "max_retries": 3,
  "retry_delay": 2,
  "debug": false
}
EOF

# Create log directory
mkdir -p /var/log/whm2bunny
chmod 755 /var/log/whm2bunny
```

### Step 2: Register Hooks in WHM

**Option A: Via WHM Web Interface**

1. Login to WHM as root
2. Navigate to: **Home > Script Hooks > Add Script Hook**
3. Add hooks for each event:

| Event Type | Script Path | Evaluator |
|------------|-------------|-----------|
| Creating an Account (Post) | `/usr/local/cpanel/whm2bunny/whm_hook.py createacct` | `/usr/local/cpanel/3rdparty/bin/python3` |
| Adding an Addon Domain (Post) | `/usr/local/cpanel/whm2bunny/whm_hook.py addaddondomain` | `/usr/local/cpanel/3rdparty/bin/python3` |
| Parking a Subdomain (Post) | `/usr/local/cpanel/whm2bunny/whm_hook.py parksubdomain` | `/usr/local/cpanel/3rdparty/bin/python3` |
| Terminating an Account (Post) | `/usr/local/cpanel/whm2bunny/whm_hook.py killacct` | `/usr/local/cpanel/3rdparty/bin/python3` |

**Option B: Via Command Line**

```bash
# Create account hook
/usr/local/cpanel/bin/manage_hooks add scripthook \
  --category Whostmgr --event Create --stage post \
  --script /usr/local/cpanel/whm2bunny/whm_hook.py \
  --exectype script --manual 1 --arg createacct

# Addon domain hook
/usr/local/cpanel/bin/manage_hooks add scripthook \
  --category Whostmgr --event AddonDomain --stage post \
  --script /usr/local/cpanel/whm2bunny/whm_hook.py \
  --exectype script --manual 1 --arg addaddondomain

# Subdomain hook
/usr/local/cpanel/bin/manage_hooks add scripthook \
  --category Whostmgr --event Parksubdomain --stage post \
  --script /usr/local/cpanel/whm2bunny/whm_hook.py \
  --exectype script --manual 1 --arg parksubdomain

# Account termination hook
/usr/local/cpanel/bin/manage_hooks add scripthook \
  --category Whostmgr --event Killacct --stage post \
  --script /usr/local/cpanel/whm2bunny/whm_hook.py \
  --exectype script --manual 1 --arg killacct
```

### Step 3: Verify & Test

```bash
# Verify hooks are registered
/usr/local/cpanel/bin/manage_hooks list | grep whm_hook

# Test manually
echo '{"domain":"test.example.com","user":"testuser"}' | \
  /usr/local/cpanel/whm2bunny/whm_hook.py createacct

# Check logs
tail -f /var/log/whm2bunny/hook.log
```

---

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/hook` | WHM webhook receiver (HMAC protected) |
| `GET` | `/health` | Health check |
| `GET` | `/ready` | Readiness check |
| `GET` | `/ping` | Heartbeat (for load balancers) |

### Health Check

```bash
curl http://localhost:9090/health
# {"status": "healthy", "uptime": "2h30m", "version": "1.0.0"}
```

### Readiness Check

```bash
curl http://localhost:9090/ready
# {"ready": true, "checks": {"bunny": "ok", "telegram": "ok", "state": "ok"}}
```

### Debug Endpoints (enabled with `DEBUG=true`)

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/debug/pending` | List pending provisions |
| `GET` | `/debug/last-error` | Last 10 errors |
| `POST` | `/debug/retry/{id}` | Retry failed provision |
| `GET` | `/debug/state` | All provision states |

---

## Telegram Bot Commands

| Command | Description |
|---------|-------------|
| `/start` | Register and show help |
| `/status` | Show provisioning status |
| `/stats` | Show bandwidth statistics |
| `/purge <domain>` | Purge CDN cache for domain |
| `/purge_all` | Purge all CDN caches |
| `/list` | List provisioned domains |
| `/retry <domain>` | Retry failed provisioning |
| `/help` | Show available commands |

---

## Auto-Recovery

When whm2bunny restarts, it automatically recovers pending/failed provisions:

1. **State Loading** - Reads state from `/var/lib/whm2bunny/state.json`
2. **Backoff Delay** - Waits 5 seconds after server starts
3. **Recovery Loop** - Processes each pending/failed domain with 2-4 second backoff
4. **Retry Limit** - Skips domains with 5+ retry attempts

```
Server Start → Wait 5s → Load Pending States → Recovery Loop
                                              │
                                              ├── Domain 1 → Wait 2s → Provision
                                              ├── Domain 2 → Wait 3s → Provision
                                              └── Domain 3 → Wait 4s → Provision
```

---

## Custom Nameservers

Configure glue records at your domain registrar:

| Nameserver | Points To |
|------------|-----------|
| `ns1.mordenhost.com` | `coco.bunny.net` |
| `ns2.mordenhost.com` | `kiki.bunny.net` |

**Setup at Registrar:**
1. Go to domain registrar DNS settings
2. Add glue records:
   - `ns1.mordenhost.com` → `91.121.54.114` (coco.bunny.net IP)
   - `ns2.mordenhost.com` → `91.121.54.115` (kiki.bunny.net IP)
3. Change domain nameservers to `ns1.mordenhost.com` and `ns2.mordenhost.com`

---

## Troubleshooting

### Webhook Not Received

```bash
# Check if whm2bunny is running
curl http://localhost:9090/health

# Check firewall
sudo ufw allow 9090/tcp

# Check hook logs on WHM server
tail -f /var/log/whm2bunny/hook.log
```

### Signature Verification Failed

```bash
# Ensure secret matches in both configs
# /etc/whm2bunny/config.json (WHM server)
# config.yaml or WHM_HOOK_SECRET env (whm2bunny server)

# Check for whitespace in secrets
echo -n "your-secret" | md5sum
```

### DNS Zone Not Created

```bash
# Verify Bunny API key
curl -H "AccessKey: YOUR_API_KEY" https://api.bunny.net/dnszone

# Check logs
docker logs whm2bunny | grep -i error
```

### SSL Certificate Pending

SSL certificates are issued automatically by BunnyCDN. This can take 1-5 minutes.

```bash
# Check SSL status manually
curl -H "AccessKey: YOUR_API_KEY" \
  https://api.bunny.net/pullzone/{ZONE_ID}/certificates
```

### Recovery Not Working

```bash
# Check state file
cat /var/lib/whm2bunny/state.json | jq .

# Manually trigger recovery via debug endpoint
curl -X POST http://localhost:9090/debug/retry/{state_id}
```

---

## Development

### Prerequisites

- Go 1.21+
- Make
- Docker (optional)

### Build

```bash
# Clone
git clone https://github.com/mordenhost/whm2bunny.git
cd whm2bunny

# Build
go build -o whm2bunny ./cmd/whm2bunny

# Test
go test ./...

# Run with debug
DEBUG=true ./whm2bunny serve
```

### Make Targets

| Command | Description |
|---------|-------------|
| `make build` | Build binary |
| `make test` | Run tests |
| `make test-coverage` | HTML coverage report |
| `make lint` | Run linters |
| `make ci` | Run CI checks locally |
| `make docker-build` | Build Docker image |
| `make release` | Create release artifacts |

---

## Project Structure

```
whm2bunny/
├── cmd/whm2bunny/              # CLI entry point
│   ├── main.go                 # Main entry
│   └── commands/               # Cobra commands (serve, config, version)
│       └── serve.go            # HTTP server setup, recovery, scheduler
│
├── internal/
│   ├── bunny/                  # Bunny.net API client
│   │   ├── client.go           # Base HTTP client with retry
│   │   ├── dns.go              # DNS zone/records API
│   │   ├── cdn.go              # Pull zone API
│   │   └── stats.go            # Bandwidth statistics
│   │
│   ├── provisioner/            # Provisioning orchestration
│   │   ├── provision.go        # Main provisioner, recovery, SSL check
│   │   ├── domain.go           # Domain provisioning steps
│   │   ├── subdomain.go        # Subdomain provisioning
│   │   └── deprovision.go      # Cleanup logic
│   │
│   ├── webhook/                # WHM webhook handling
│   │   └── handler.go          # HMAC verification, routing
│   │
│   ├── validator/              # Input validation
│   │   └── validator.go        # Domain, subdomain, DNS checks
│   │
│   ├── notifier/               # Telegram notifications
│   │   └── telegram.go         # Bot commands, notifications
│   │
│   ├── scheduler/              # Background jobs
│   │   └── summary.go          # Daily/weekly summaries
│   │
│   ├── state/                  # State persistence
│   │   ├── manager.go          # State CRUD operations
│   │   └── snapshot.go         # Bandwidth snapshots
│   │
│   └── retry/                  # Retry logic
│       └── retry.go            # Exponential backoff
│
├��─ config/                     # Configuration loading
├── scripts/                    # Installation scripts
│   ├── whm_hook.py             # WHM hook script
│   └── hook_config.json.example
│
└── docs/                       # Documentation
```

---

## Security

- **HMAC-SHA256** signature verification for all webhooks
- No credentials in config files (environment variables supported)
- TLS for all external communications
- Non-root container user
- Input validation with DNS checks
- Context timeout for all API calls

---

## About mordenhost.com

[mordenhost.com](https://mordenhost.com) is a web hosting provider offering reliable cPanel-based hosting solutions. This tool was developed to automate DNS and CDN provisioning for our infrastructure, ensuring fast content delivery through BunnyCDN's Asia-Pacific edge network.

---

## License

MIT License - see [LICENSE](LICENSE) for details.

---

## Support

- [GitHub Issues](https://github.com/mordenhost/whm2bunny/issues)
- [mordenhost.com](https://mordenhost.com)
