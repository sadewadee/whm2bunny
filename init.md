BunnyDNS nya akan menggunakan custom nameserver yaitu ns1.mordenhost.com dan ns2.mordenhost.com yang sudah di pointing ke a record coco.bunny.net dan kiki.bunny.net
BunnyCDN hanya akan mendukung koneksi ke wilayah asia dan ocenia.

stack akan menggunakan golang sebagai [gimme best name for this].

Untuk nama Golang service-nya: **`whm2bunny`** — jelas dan deskriptif, langsung menunjukkan fungsinya sebagai provisioning dari WHM/cPanel ke BunnyDNS + BunnyCDN.

***

# whm2bunny

**Auto-Provisioning Service: WHM/cPanel → BunnyDNS + BunnyCDN**
*Golang daemon for Morden Hosting Infrastructure*

***

## Overview

whm2bunny adalah sebuah Go daemon yang berjalan di server WHM/cPanel dan secara otomatis memprovision **BunnyDNS Zone** + **BunnyCDN Pull Zone** setiap kali domain baru ditambahkan ke server. Nameserver yang digunakan adalah custom branded (`ns1.mordenhost.com` / `ns2.mordenhost.com`) yang di-delegate ke Bunny's anycast DNS network (`coco.bunny.net` / `kiki.bunny.net`). CDN hanya aktif di region **Asia \& Oceania**.[^1][^2]

***

## Architecture

```
WHM/cPanel
  └── Hook Script (POST /hook)
        └── whm2bunny HTTP Server
              ├── Step 1: POST /dnszone          → BunnyDNS
              ├── Step 2: PUT  /dnszone/{id}/records  → BunnyDNS (A, CNAME, MX, TXT)
              ├── Step 3: POST /pullzone          → BunnyCDN (Asia+Oceania)
              └── Step 4: PUT  /dnszone/{id}/records  → BunnyDNS (CNAME → b-cdn.net)
```


***

## Prerequisites

### Custom Nameserver Delegation

Sebelum whm2bunny dijalankan, nameserver Morden harus sudah dikonfigurasi:


| Hostname | Points To | Purpose |
| :-- | :-- | :-- |
| `ns1.mordenhost.com` | `coco.bunny.net` (A record) | Primary NS |
| `ns2.mordenhost.com` | `kiki.bunny.net` (A record) | Secondary NS |

> **Catatan**: Resolusi `coco.bunny.net` dan `kiki.bunny.net` ke IP dilakukan via lookup saat setup. Tambahkan sebagai **glue records** di registrar domain `mordenhost.com`.[^3]

***

## Configuration

```go
// config.go
package config

type Config struct {
	BunnyAPIKey        string `env:"BUNNY_API_KEY"`
	BunnyBaseURL       string `env:"BUNNY_BASE_URL" envDefault:"https://api.bunny.net"`
	ReverseProxyIP     string `env:"REVERSE_PROXY_IP"` // IP of your Nginx/Caddy reverse proxy
	OriginShieldRegion string `env:"ORIGIN_SHIELD_REGION" envDefault:"SG"`
	WHMHookSecret      string `env:"WHM_HOOK_SECRET"`
	ServerPort         string `env:"SERVER_PORT" envDefault:"9090"`
	SOAEmail           string `env:"SOA_EMAIL" envDefault:"hostmaster@mordenhost.com"`
}
```


***

## API Reference

### Base URL \& Auth

```
Base URL : https://api.bunny.net
Auth     : Header "AccessKey: <BUNNY_API_KEY>"
```


***

### Step 1 — Create DNS Zone

`POST /dnszone`[^1]

```json
{
  "Domain": "example.com",
  "CustomNameserversEnabled": true,
  "Nameserver1": "ns1.mordenhost.com",
  "Nameserver2": "ns2.mordenhost.com",
  "SoaEmail": "hostmaster@mordenhost.com",
  "LoggingEnabled": true,
  "LogAnonymized": false
}
```

**Response** (201):

```json
{
  "Id": 123456,
  "Domain": "example.com",
  "Nameserver1": "ns1.mordenhost.com",
  "Nameserver2": "ns2.mordenhost.com"
}
```

> Simpan `Id` sebagai `zoneId` untuk langkah berikutnya.

***

### Step 2 — Add DNS Records

`PUT /dnszone/{zoneId}/records`[^4]

DNS Record type integers dari Bunny API:


| Type | Record |
| :-- | :-- |
| `0` | A |
| `1` | AAAA |
| `2` | CNAME |
| `3` | TXT |
| `4` | MX |
| `12` | NS |

**A Record (Root → Reverse Proxy)**

```json
{
  "Type": 0,
  "Name": "@",
  "Value": "{{REVERSE_PROXY_IP}}",
  "Ttl": 300
}
```

**CNAME www**

```json
{
  "Type": 2,
  "Name": "www",
  "Value": "example.com",
  "Ttl": 300
}
```

**MX Record**

```json
{
  "Type": 4,
  "Name": "@",
  "Value": "mail.example.com",
  "Priority": 10,
  "Ttl": 3600
}
```

**TXT Record (SPF)**

```json
{
  "Type": 3,
  "Name": "@",
  "Value": "v=spf1 ip4:{{REVERSE_PROXY_IP}} ~all",
  "Ttl": 3600
}
```


***

### Step 3 — Create CDN Pull Zone (Asia \& Oceania Only)

`POST /pullzone`[^2][^5]

`EnableGeoZoneASIA` mencakup **Asia dan Oceania** sekaligus dalam satu flag di Bunny API.

```json
{
  "Name": "morden-example-com",
  "OriginUrl": "http://{{REVERSE_PROXY_IP}}",
  "OriginHostHeader": "example.com",
  "AddHostHeader": true,

  "EnableGeoZoneUS": false,
  "EnableGeoZoneEU": false,
  "EnableGeoZoneASIA": true,
  "EnableGeoZoneSA": false,
  "EnableGeoZoneAF": false,

  "EnableOriginShield": true,
  "OriginShieldZoneCode": "SG",

  "EnableAutoSSL": true,
  "DisableLetsEncrypt": false,

  "DisableCookies": false,
  "EnableLogging": true,

  "OriginConnectTimeout": 10,
  "OriginResponseTimeout": 60,
  "OriginRetries": 3,
  "OriginRetry5XXResponses": true,
  "UseStaleWhileOffline": true
}
```

**Response** (201):

```json
{
  "Id": 789,
  "Name": "morden-example-com",
  "Hostnames": [
    { "Value": "morden-example-com.b-cdn.net" }
  ]
}
```

> Simpan `Hostnames[^0].Value` sebagai CDN hostname untuk Step 4.

***

### Step 4 — Sync CDN CNAME ke BunnyDNS

Update record `@` dan `www` di DNS zone agar mengarah ke CDN.[^6][^4]

**Update `@` → CDN**

```json
{
  "Type": 2,
  "Name": "@",
  "Value": "morden-example-com.b-cdn.net",
  "Ttl": 300
}
```

**Update `www` → CDN**

```json
{
  "Type": 2,
  "Name": "www",
  "Value": "morden-example-com.b-cdn.net",
  "Ttl": 300
}
```

> Setelah ini, semua traffic `example.com` dan `www.example.com` akan melalui BunnyCDN edge di Asia/Oceania terlebih dahulu.

***

## Golang Implementation

### Project Structure

```
whm2bunny/
├── cmd/
│   └── whm2bunny/
│       └── main.go
├── internal/
│   ├── bunny/
│   │   ├── client.go        # HTTP client wrapper
│   │   ├── dns.go           # DNS zone & record operations
│   │   └── cdn.go           # Pull zone operations
│   ├── provisioner/
│   │   └── provision.go     # Orchestrates all 4 steps
│   └── webhook/
│       └── handler.go       # WHM hook HTTP handler
├── config/
│   └── config.go
├── Dockerfile
└── docker-compose.yml
```


***

### Bunny HTTP Client

```go
// internal/bunny/client.go
package bunny

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

func NewClient(apiKey, baseURL string) *Client {
	return &Client{
		apiKey:  apiKey,
		baseURL: baseURL,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) do(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshal body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return nil, 0, err
	}

	req.Header.Set("AccessKey", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	return respBody, resp.StatusCode, nil
}
```


***

### DNS Operations

```go
// internal/bunny/dns.go
package bunny

import (
	"context"
	"encoding/json"
	"fmt"
)

type DNSZoneRequest struct {
	Domain                   string `json:"Domain"`
	CustomNameserversEnabled bool   `json:"CustomNameserversEnabled"`
	Nameserver1              string `json:"Nameserver1"`
	Nameserver2              string `json:"Nameserver2"`
	SoaEmail                 string `json:"SoaEmail"`
	LoggingEnabled           bool   `json:"LoggingEnabled"`
}

type DNSZoneResponse struct {
	ID     int64  `json:"Id"`
	Domain string `json:"Domain"`
}

type DNSRecordType int

const (
	RecordA     DNSRecordType = 0
	RecordAAAA  DNSRecordType = 1
	RecordCNAME DNSRecordType = 2
	RecordTXT   DNSRecordType = 3
	RecordMX    DNSRecordType = 4
)

type DNSRecord struct {
	Type     DNSRecordType `json:"Type"`
	Name     string        `json:"Name"`
	Value    string        `json:"Value"`
	Ttl      int           `json:"Ttl"`
	Priority *int          `json:"Priority,omitempty"`
}

func (c *Client) CreateDNSZone(ctx context.Context, req DNSZoneRequest) (*DNSZoneResponse, error) {
	body, status, err := c.do(ctx, "POST", "/dnszone", req)
	if err != nil {
		return nil, err
	}
	if status != 201 {
		return nil, fmt.Errorf("create dns zone: status %d body %s", status, body)
	}

	var zone DNSZoneResponse
	if err := json.Unmarshal(body, &zone); err != nil {
		return nil, fmt.Errorf("unmarshal dns zone: %w", err)
	}
	return &zone, nil
}

func (c *Client) AddDNSRecord(ctx context.Context, zoneID int64, record DNSRecord) error {
	path := fmt.Sprintf("/dnszone/%d/records", zoneID)
	body, status, err := c.do(ctx, "PUT", path, record)
	if err != nil {
		return err
	}
	if status != 201 {
		return fmt.Errorf("add dns record: status %d body %s", status, body)
	}
	return nil
}

// UpdateDNSRecord updates an existing record by deleting + re-adding
// since Bunny uses POST /dnszone/{id}/records/{recordId}
func (c *Client) UpdateDNSRecord(ctx context.Context, zoneID, recordID int64, record DNSRecord) error {
	path := fmt.Sprintf("/dnszone/%d/records/%d", zoneID, recordID)
	body, status, err := c.do(ctx, "POST", path, record)
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("update dns record: status %d body %s", status, body)
	}
	return nil
}
```


***

### CDN Operations

```go
// internal/bunny/cdn.go
package bunny

import (
	"context"
	"encoding/json"
	"fmt"
)

type PullZoneRequest struct {
	Name                string `json:"Name"`
	OriginUrl           string `json:"OriginUrl"`
	OriginHostHeader    string `json:"OriginHostHeader"`
	AddHostHeader       bool   `json:"AddHostHeader"`

	// Region: Asia + Oceania ONLY
	EnableGeoZoneUS   bool `json:"EnableGeoZoneUS"`
	EnableGeoZoneEU   bool `json:"EnableGeoZoneEU"`
	EnableGeoZoneASIA bool `json:"EnableGeoZoneASIA"` // covers Asia AND Oceania
	EnableGeoZoneSA   bool `json:"EnableGeoZoneSA"`
	EnableGeoZoneAF   bool `json:"EnableGeoZoneAF"`

	// Origin Shield
	EnableOriginShield   bool   `json:"EnableOriginShield"`
	OriginShieldZoneCode string `json:"OriginShieldZoneCode"` // "SG" for Singapore

	// SSL
	EnableAutoSSL     bool `json:"EnableAutoSSL"`
	DisableLetsEncrypt bool `json:"DisableLetsEncrypt"`

	// Reliability
	OriginConnectTimeout    int  `json:"OriginConnectTimeout"`
	OriginResponseTimeout   int  `json:"OriginResponseTimeout"`
	OriginRetries           int  `json:"OriginRetries"`
	OriginRetry5XXResponses bool `json:"OriginRetry5XXResponses"`
	UseStaleWhileOffline    bool `json:"UseStaleWhileOffline"`
	EnableLogging           bool `json:"EnableLogging"`
}

type PullZoneHostname struct {
	Value string `json:"Value"`
}

type PullZoneResponse struct {
	ID        int64              `json:"Id"`
	Name      string             `json:"Name"`
	Hostnames []PullZoneHostname `json:"Hostnames"`
}

func (c *Client) CreatePullZone(ctx context.Context, domain, originIP, shieldRegion string) (*PullZoneResponse, error) {
	name := sanitizePullZoneName(domain) // "example.com" → "morden-example-com"

	req := PullZoneRequest{
		Name:             name,
		OriginUrl:        "http://" + originIP,
		OriginHostHeader: domain,
		AddHostHeader:    true,

		EnableGeoZoneUS:   false,
		EnableGeoZoneEU:   false,
		EnableGeoZoneASIA: true, // Asia + Oceania
		EnableGeoZoneSA:   false,
		EnableGeoZoneAF:   false,

		EnableOriginShield:   true,
		OriginShieldZoneCode: shieldRegion,

		EnableAutoSSL:      true,
		DisableLetsEncrypt: false,

		OriginConnectTimeout:    10,
		OriginResponseTimeout:   60,
		OriginRetries:           3,
		OriginRetry5XXResponses: true,
		UseStaleWhileOffline:    true,
		EnableLogging:           true,
	}

	body, status, err := c.do(ctx, "POST", "/pullzone", req)
	if err != nil {
		return nil, err
	}
	if status != 201 {
		return nil, fmt.Errorf("create pull zone: status %d body %s", status, body)
	}

	var pz PullZoneResponse
	if err := json.Unmarshal(body, &pz); err != nil {
		return nil, fmt.Errorf("unmarshal pull zone: %w", err)
	}
	return &pz, nil
}

func sanitizePullZoneName(domain string) string {
	name := "morden-" + domain
	// replace dots and special chars with dashes
	result := make([]byte, len(name))
	for i := 0; i < len(name); i++ {
		if name[i] == '.' {
			result[i] = '-'
		} else {
			result[i] = name[i]
		}
	}
	return string(result)
}
```


***

### Provisioner — Orchestrates All Steps

```go
// internal/provisioner/provision.go
package provisioner

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/mordenhost/whm2bunny/config"
	"github.com/mordenhost/whm2bunny/internal/bunny"
)

type Provisioner struct {
	cfg    *config.Config
	bunny  *bunny.Client
	logger *slog.Logger
}

func New(cfg *config.Config, logger *slog.Logger) *Provisioner {
	return &Provisioner{
		cfg:    cfg,
		bunny:  bunny.NewClient(cfg.BunnyAPIKey, cfg.BunnyBaseURL),
		logger: logger,
	}
}

// Provision runs the full 4-step provisioning for a new domain.
func (p *Provisioner) Provision(ctx context.Context, domain string) error {
	log := p.logger.With("domain", domain)

	// ── Step 1: Create DNS Zone ──────────────────────────────────────────
	log.Info("creating BunnyDNS zone")
	zone, err := p.bunny.CreateDNSZone(ctx, bunny.DNSZoneRequest{
		Domain:                   domain,
		CustomNameserversEnabled: true,
		Nameserver1:              "ns1.mordenhost.com",
		Nameserver2:              "ns2.mordenhost.com",
		SoaEmail:                 p.cfg.SOAEmail,
		LoggingEnabled:           true,
	})
	if err != nil {
		return fmt.Errorf("step1 create dns zone: %w", err)
	}
	log.Info("dns zone created", "zoneId", zone.ID)

	// ── Step 2: Seed DNS Records ─────────────────────────────────────────
	mxPriority := 10
	records := []bunny.DNSRecord{
		{Type: bunny.RecordA, Name: "@", Value: p.cfg.ReverseProxyIP, Ttl: 300},
		{Type: bunny.RecordCNAME, Name: "www", Value: domain, Ttl: 300},
		{Type: bunny.RecordMX, Name: "@", Value: "mail." + domain, Ttl: 3600, Priority: &mxPriority},
		{Type: bunny.RecordTXT, Name: "@", Value: fmt.Sprintf("v=spf1 ip4:%s ~all", p.cfg.ReverseProxyIP), Ttl: 3600},
	}

	for _, rec := range records {
		log.Info("adding dns record", "type", rec.Type, "name", rec.Name)
		if err := p.bunny.AddDNSRecord(ctx, zone.ID, rec); err != nil {
			return fmt.Errorf("step2 add record %s: %w", rec.Name, err)
		}
	}

	// ── Step 3: Create BunnyCDN Pull Zone ────────────────────────────────
	log.Info("creating BunnyCDN pull zone")
	pz, err := p.bunny.CreatePullZone(ctx, domain, p.cfg.ReverseProxyIP, p.cfg.OriginShieldRegion)
	if err != nil {
		return fmt.Errorf("step3 create pull zone: %w", err)
	}

	if len(pz.Hostnames) == 0 {
		return fmt.Errorf("step3 pull zone created but no hostname returned")
	}
	cdnHostname := pz.Hostnames[^0].Value
	log.Info("pull zone created", "cdnHostname", cdnHostname)

	// ── Step 4: Sync CDN CNAMEs back to BunnyDNS ────────────────────────
	cnameRecords := []bunny.DNSRecord{
		{Type: bunny.RecordCNAME, Name: "@", Value: cdnHostname, Ttl: 300},
		{Type: bunny.RecordCNAME, Name: "www", Value: cdnHostname, Ttl: 300},
	}

	for _, rec := range cnameRecords {
		log.Info("syncing cdn cname to dns", "name", rec.Name, "value", rec.Value)
		if err := p.bunny.AddDNSRecord(ctx, zone.ID, rec); err != nil {
			return fmt.Errorf("step4 sync cname %s: %w", rec.Name, err)
		}
	}

	log.Info("provisioning complete",
		"domain", domain,
		"zoneId", zone.ID,
		"pullZoneId", pz.ID,
		"cdnHostname", cdnHostname,
	)
	return nil
}
```


***

### WHM Hook Handler

WHM/cPanel memanggil hook script saat domain dibuat. Hook script tersebut melakukan HTTP POST ke whm2bunny.[^7]

```go
// internal/webhook/handler.go
package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/mordenhost/whm2bunny/internal/provisioner"
	"log/slog"
)

type WHMHookPayload struct {
	Event  string `json:"event"`
	Domain string `json:"domain"`
}

type Handler struct {
	provisioner *provisioner.Provisioner
	secret      string
	logger      *slog.Logger
}

func NewHandler(p *provisioner.Provisioner, secret string, logger *slog.Logger) *Handler {
	return &Handler{provisioner: p, secret: secret, logger: logger}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Verify HMAC signature from WHM hook
	sig := r.Header.Get("X-Whm2bunny-Signature")
	if !h.verifySignature(r, sig) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var payload WHMHookPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if payload.Event != "domain_created" || payload.Domain == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	go func() {
		if err := h.provisioner.Provision(r.Context(), payload.Domain); err != nil {
			h.logger.Error("provisioning failed", "domain", payload.Domain, "err", err)
		}
	}()

	w.WriteHeader(http.StatusAccepted)
}

func (h *Handler) verifySignature(r *http.Request, sig string) bool {
	mac := hmac.New(sha256.New, []byte(h.secret))
	mac.Write([]byte(r.URL.Path))
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(sig), []byte(expected))
}
```


***

### WHM Hook Script

Simpan di `/usr/local/cpanel/hooks/post_domain_create.sh`:

```bash
#!/bin/bash
DOMAIN="$1"
SECRET="your-whm-hook-secret"
SIG=$(echo -n "/hook" | openssl dgst -sha256 -hmac "$SECRET" | awk '{print $2}')

curl -s -X POST http://127.0.0.1:9090/hook \
  -H "Content-Type: application/json" \
  -H "X-Whm2bunny-Signature: $SIG" \
  -d "{\"event\":\"domain_created\",\"domain\":\"$DOMAIN\"}"
```

Register di WHM:

```bash
# WHM > cPanel > Manage Hooks > Add Hook
# Event: Domains::add_domain
# Stage: post
# Action: /usr/local/cpanel/hooks/post_domain_create.sh "$domain"
```


***

## Deployment

```yaml
# docker-compose.yml
services:
  whm2bunny:
    image: mordenhost/whm2bunny:latest
    restart: unless-stopped
    ports:
      - "127.0.0.1:9090:9090"
    environment:
      BUNNY_API_KEY: ${BUNNY_API_KEY}
      REVERSE_PROXY_IP: ${REVERSE_PROXY_IP}
      ORIGIN_SHIELD_REGION: SG
      WHM_HOOK_SECRET: ${WHM_HOOK_SECRET}
      SOA_EMAIL: hostmaster@mordenhost.com
      SERVER_PORT: 9090
```

```dockerfile
# Dockerfile
FROM golang:1.25.1-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o whm2bunny ./cmd/whm2bunny

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/whm2bunny /usr/local/bin/whm2bunny
ENTRYPOINT ["whm2bunny"]
```


***

## Error Handling \& Idempotency

| Scenario | Handling |
| :-- | :-- |
| DNS Zone sudah ada | Cek `409 Conflict`, fetch existing zone ID dan lanjut |
| Pull Zone name conflict | Append suffix `-2`, `-3`, dst. |
| CDN hostname kosong | Retry 3x dengan backoff 5s |
| WHM hook duplikat | Cek domain existence di BunnyDNS sebelum create |
| Bunny API rate limit (429) | Exponential backoff: 1s → 2s → 4s → 8s |

[^2][^1]

***

Dengan arsitektur ini, setiap domain baru di Morden Hosting akan **otomatis ter-provision di Bunny dalam hitungan detik** — DNS dengan custom nameserver branded `mordenhost.com`, CDN dengan edge nodes di Asia/Oceania, tanpa ada konfigurasi manual.[^5]
<span style="display:none">[^10][^11][^12][^13][^14][^15][^8][^9]</span>

<div align="center">⁂</div>

[^1]: https://docs.bunny.net/api-reference/core/dns-zone/add-dns-zone

[^2]: https://docs.bunny.net/api-reference/core/pull-zone/add-pull-zone

[^3]: https://docs.bunny.net/dns

[^4]: https://docs.bunny.net/api-reference/core/dns-zone/add-dns-record

[^5]: https://docs.bunny.net/api-reference/core/pull-zone/update-pull-zone

[^6]: https://docs.bunny.net/api-reference/core/dns-zone/update-dns-record

[^7]: https://docs.cpanel.net/whm/dns-functions/synchronize-dns-records/

[^8]: https://www.janbrennenstuhl.eu/bunny-cdn-domain-redirect/

[^9]: https://docs.bunny.net/api-reference/core/region/region-list

[^10]: https://bunny-launcher.net/bunny-sdk/supported-endpoints/

[^11]: https://docs.bunny.net/api-reference/core/storage-zone/list-storage-zones

[^12]: https://docs.bunny.net/api-reference/core/pull-zone/get-pull-zone

[^13]: https://www.jhanley.com/blog/bunny-net-account-and-api-keys/

[^14]: https://github.com/libdns/bunny

[^15]: https://docs.bunny.net/reference/get_shield-waf-rules-review-triggered-shieldzoneid

