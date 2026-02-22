# whm2bunny

[![Test](https://github.com/mordenhost/whm2bunny/actions/workflows/test.yml/badge.svg)](https://github.com/mordenhost/whm2bunny/actions/workflows/test.yml)
[![Release](https://github.com/mordenhost/whm2bunny/actions/workflows/release.yml/badge.svg)](https://github.com/mordenhost/whm2bunny/actions/workflows/release.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/mordenhost/whm2bunny)](https://goreportcard.com/report/github.com/mordenhost/whm2bunny)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A Go daemon that auto-provisions **BunnyDNS Zone + BunnyCDN Pull Zone** when a new domain is added to WHM/cPanel. It runs as an HTTP server receiving webhooks from WHM hooks.

## Features

- **Automatic DNS Provisioning** - Creates BunnyDNS zones with custom nameservers
- **CDN Integration** - Auto-provisions BunnyCDN pull zones (Asia+Oceania region)
- **WHM/cPanel Integration** - Seamless webhook integration with cPanel hooks
- **Telegram Notifications** - Real-time alerts for provisioning events
- **Crash Recovery** - State persistence with automatic resume
- **Daily/Weekly Summaries** - Bandwidth statistics via Telegram
- **Bot Commands** - `/purge`, `/stats`, `/status`, and more

## Quick Start

### Binary Installation

```bash
# Download latest release
curl -sSL https://releases.mordenhost.com/whm2bunny/latest/whm2bunny-linux-amd64 -o whm2bunny
chmod +x whm2bunny

# Generate config
./whm2bunny config generate config.yaml
```

### Docker

```bash
docker run -d \
  --name whm2bunny \
  -p 9090:9090 \
  -e BUNNY_API_KEY=your-key \
  -e REVERSE_PROXY_IP=your-ip \
  -e WHM_HOOK_SECRET=your-secret \
  mordenhost/whm2bunny:latest
```

### Docker Compose

```bash
# Clone repository
git clone https://github.com/mordenhost/whm2bunny.git
cd whm2bunny

# Copy and edit config
cp config.yaml.example config.yaml

# Set environment variables
export BUNNY_API_KEY="your-api-key"
export REVERSE_PROXY_IP="your-server-ip"
export WHM_HOOK_SECRET="your-webhook-secret"

# Start
docker-compose up -d
```

## Configuration

### Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `BUNNY_API_KEY` | Yes | Bunny.net API key |
| `REVERSE_PROXY_IP` | Yes | IP of Nginx/Caddy reverse proxy |
| `WHM_HOOK_SECRET` | Yes | HMAC secret for webhook verification |
| `TELEGRAM_BOT_TOKEN` | No | Telegram bot token |
| `TELEGRAM_CHAT_ID` | No | Telegram chat ID |

### Config File

```yaml
# config.yaml
server:
  port: 9090
  host: "127.0.0.1"

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
  reverse_proxy_ip: "${REVERSE_PROXY_IP}"

webhook:
  secret: "${WHM_HOOK_SECRET}"

telegram:
  enabled: true
  bot_token: "${TELEGRAM_BOT_TOKEN}"
  chat_id: "${TELEGRAM_CHAT_ID}"

logging:
  level: "info"
  format: "json"
```

## WHM/cPanel Integration

### Install Hook Handler

```bash
# Run installer
curl -sSL https://get.mordenhost.com/whm2bunny | bash

# Or manually
./scripts/install.sh
```

### Register Hooks

```bash
# Register hooks with cPanel
/usr/local/cpanel/bin/manage_hooks add module Whm2bunnyHook

# Verify registration
/usr/local/cpanel/bin/manage_hooks list | grep Whm2bunny
```

### Supported Events

| Event | Action |
|-------|--------|
| Account Created | Full provision (DNS + CDN) |
| Addon Domain Added | Full provision (DNS + CDN) |
| Subdomain Created | CDN provision + DNS CNAME |
| Account Deleted | Deprovision (cleanup) |

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| POST | `/hook` | WHM webhook receiver |
| GET | `/health` | Health check |
| GET | `/ready` | Readiness check |

### Health Check

```bash
curl http://localhost:9090/health
# {"status": "healthy", "uptime": "2h30m"}
```

### Readiness Check

```bash
curl http://localhost:9090/ready
# {"ready": true, "checks": {"bunny": "ok", "telegram": "ok"}}
```

## Telegram Bot Commands

| Command | Description |
|---------|-------------|
| `/start` | Register and show help |
| `/status` | Show provisioning status |
| `/stats` | Show bandwidth statistics |
| `/purge <domain>` | Purge CDN cache |
| `/purge_all` | Purge all CDN caches |
| `/list` | List provisioned domains |
| `/retry <domain>` | Retry failed provisioning |
| `/help` | Show available commands |

## Provisioning Flow

```
WHM Hook → POST /hook → whm2bunny
                    │
                    ├── Step 1: Create DNS Zone
                    ├── Step 2: Add DNS Records
                    │   ├── A record: @ → origin IP
                    │   ├── CNAME: www → @
                    │   ├── MX: mail.domain.com
                    │   └── TXT: SPF record
                    ├── Step 3: Create Pull Zone (CDN)
                    └── Step 4: Add CNAME: cdn → pullzone
                    │
                    └── Telegram Notification
```

## Custom Nameservers

Configure glue records at your domain registrar:

| Nameserver | Points To |
|------------|-----------|
| ns1.mordenhost.com | coco.bunny.net |
| ns2.mordenhost.com | kiki.bunny.net |

## Development

### Prerequisites

- Go 1.21+
- Make

### Build

```bash
# Clone
git clone https://github.com/mordenhost/whm2bunny.git
cd whm2bunny

# Build
make build

# Test
make test

# Run
./whm2bunny serve
```

### Make Targets

```bash
make build        # Build binary
make test         # Run tests
make test-coverage # HTML coverage report
make lint         # Run linters
make ci           # Run CI checks locally
make docker-build # Build Docker image
make release      # Create release artifacts
```

## Architecture

```
whm2bunny/
├── cmd/whm2bunny/          # CLI entry point
│   ├── main.go
│   └── commands/           # Cobra commands
├── internal/
│   ├── bunny/              # Bunny.net API client
│   ├── provisioner/        # Provisioning logic
│   ├── webhook/            # WHM webhook handler
│   ├── notifier/           # Telegram notifications
│   ├── scheduler/          # Cron jobs
│   ├── state/              # State persistence
│   ├── retry/              # Retry logic
│   └── validator/          # Input validation
├── config/                 # Configuration
├── scripts/                # Installation scripts
└── docs/                   # Documentation
```

## Documentation

- [Testing Documentation](docs/TESTING.md)
- [CI/CD Documentation](docs/CI-CD.md)

## Security

- HMAC-SHA256 signature verification for all webhooks
- No credentials in config files (environment variables only)
- TLS for all external communications
- Non-root container user

## License

MIT License - see [LICENSE](LICENSE) for details.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make changes with tests
4. Run `make ci` to verify
5. Submit a pull request

## Support

- [GitHub Issues](https://github.com/mordenhost/whm2bunny/issues)
- [Documentation](docs/)
