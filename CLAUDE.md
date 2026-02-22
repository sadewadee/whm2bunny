# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**whm2bunny** is a Go daemon that auto-provisions BunnyDNS Zone + BunnyCDN Pull Zone when a new domain is added to WHM/cPanel. It runs as an HTTP server receiving webhooks from WHM hooks.

## Build & Run

```bash
# Build
go build -o whm2bunny ./cmd/whm2bunny

# Run
./whm2bunny

# Run with Docker
docker-compose up -d
```

## Architecture

```
WHM/cPanel Hook → POST /hook → whm2bunny HTTP Server
                                      ├── Step 1: Create DNS Zone (BunnyDNS)
                                      ├── Step 2: Add DNS Records (A, CNAME, MX, TXT)
                                      ├── Step 3: Create Pull Zone (BunnyCDN - Asia+Oceania only)
                                      └── Step 4: Sync CDN CNAME to DNS
```

## Key Configuration (env vars)

| Variable | Description |
|----------|-------------|
| `BUNNY_API_KEY` | Bunny.net API key |
| `ORIGIN_IP` | IP of WHM/cPanel origin server |
| `ORIGIN_SHIELD_REGION` | Default: `SG` (Singapore) |
| `WHM_HOOK_SECRET` | HMAC secret for webhook verification |
| `SERVER_PORT` | Default: `9090` |
| `SOA_EMAIL` | Default: `hostmaster@mordenhost.com` |

## Custom Nameservers

- `ns1.mordenhost.com` → `coco.bunny.net`
- `ns2.mordenhost.com` → `kiki.bunny.net`

Glue records must be configured at the domain registrar.

## Bunny API Auth

```
Header: AccessKey: <BUNNY_API_KEY>
Base URL: https://api.bunny.net
```

## DNS Record Type Mapping

| Type Int | Record |
|----------|--------|
| 0 | A |
| 1 | AAAA |
| 2 | CNAME |
| 3 | TXT |
| 4 | MX |

## CDN Region

Only Asia + Oceania enabled via `EnableGeoZoneASIA: true`.
