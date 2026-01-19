# ADR-007: Cloudflare Tunnel and iOS Signing Reference Implementations

## Status

**Reference** - Analysis of external codebases for future implementation

## Context

We have cloned three reference implementations into `.src/` that demonstrate useful patterns for:
1. **Cloudflare WARP/Tunnel integration** - Programmatic tunnel management
2. **iOS app signing** - Self-hosted signing infrastructure
3. **Builder/Worker pattern** - Distributed job execution

| Repo | Author | Purpose | Location |
|------|--------|---------|----------|
| **wgcf** | ViRb3 | Cloudflare WARP configuration generator | `.src/wgcf/` |
| **SignTools** | SignTools | iOS app signing web service | `.src/SignTools/` |
| **SignTools-Builder** | SignTools | macOS signing worker | `.src/SignTools-Builder/` |

---

## wgcf - Cloudflare WARP CLI

### What It Does

Unofficial CLI for Cloudflare WARP that can:
- Register new WARP accounts via Cloudflare API
- Generate WireGuard configuration profiles
- Manage WARP+ license keys
- Check device/account status

### Architecture

```
wgcf/
├── cmd/
│   ├── register/     # Account registration
│   ├── generate/     # WireGuard profile generation
│   ├── update/       # License key updates
│   ├── status/       # Account status
│   └── trace/        # Verify WARP connectivity
├── cloudflare/
│   ├── api.go        # Cloudflare WARP API client
│   └── util.go
├── config/
│   └── config.go     # TOML config file management
├── openapi/          # Auto-generated from OpenAPI spec
└── wireguard/        # WireGuard key generation
```

### Key Patterns

**1. OpenAPI Client Generation**

```go
// Generated from openapi-spec.yml
apiClient := openapi.NewAPIClient(&openapi.Configuration{
    DefaultHeader: DefaultHeaders,
    Servers: []openapi.ServerConfiguration{{URL: ApiUrl}},
    HTTPClient: &httpClient,
})

// Type-safe API calls
result, _, err := apiClient.DefaultAPI.
    Register(nil, ApiVersion).
    RegisterRequest(openapi.RegisterRequest{...}).
    Execute()
```

**2. TLS Configuration for Cloudflare API**

```go
// Must match app's TLS config or API returns 403/1020
DefaultTransport = &http.Transport{
    TLSClientConfig: &tls.Config{
        MinVersion: tls.VersionTLS12,
        MaxVersion: tls.VersionTLS12,  // Force TLS 1.2
    },
    ForceAttemptHTTP2: false,  // Disable HTTP/2
}
```

**3. TOML Config Persistence**

```go
// wgcf-account.toml stores device credentials
type Context struct {
    DeviceId   string
    AccessToken string
    LicenseKey  string
    PrivateKey  string
}
```

### Relevance to xplat

- **Cloudflare API patterns** - Same HTTP client setup for CF Workers API
- **WireGuard profile generation** - Could integrate with xplat VPN/tunnel features
- **OpenAPI codegen** - Pattern for generating type-safe API clients

---

## SignTools - iOS Signing Service

### What It Does

Self-hosted platform to sign iOS apps without a computer:
- Web interface for uploading unsigned IPAs
- Distributes signing jobs to builders (macOS machines)
- OTA installation directly to iOS devices
- Supports both developer accounts and provisioning profiles

### Architecture

```
SignTools/
├── main.go                 # Entry point, server setup
├── src/
│   ├── config/             # YAML config parsing
│   ├── storage/            # File-based app/profile storage
│   │   ├── storage.go      # Resolver initialization
│   │   ├── app_resolver.go
│   │   ├── profile_resolver.go
│   │   ├── job_resolver.go
│   │   └── upload_resolver.go
│   ├── tunnel/             # Tunnel provider abstraction
│   │   ├── generic.go      # Provider interface
│   │   ├── cloudflare.go   # cloudflared metrics scraping
│   │   └── ngrok.go        # ngrok API client
│   ├── builders/           # Builder integrations
│   │   ├── shared.go       # Builder interface
│   │   ├── selfhosted.go   # SignTools-Builder client
│   │   └── semaphore.go    # Semaphore CI client
│   └── assets/             # Embedded templates
└── signer-cfg.yml          # Config file
```

### Key Patterns

**1. Tunnel Provider Interface**

```go
// src/tunnel/generic.go
type Provider interface {
    getPublicUrl(timeout time.Duration) (string, error)
}

func GetPublicUrl(provider Provider, timeout time.Duration) (string, error) {
    timer := time.After(timeout)
    for len(timer) < 1 {
        url, err := provider.getPublicUrl(timeout)
        if err == nil {
            return url, nil
        }
        time.Sleep(100 * time.Millisecond)
    }
    return "", err
}
```

**2. Cloudflare Tunnel URL Discovery**

```go
// src/tunnel/cloudflare.go
// Scrapes cloudflared metrics endpoint to get public URL
var publicUrlRegex = regexp.MustCompile(
    `cloudflared_tunnel_user_hostnames_counts{userHostname="(.+)"}`)

func (c *Cloudflare) getPublicUrl(timeout time.Duration) (string, error) {
    url := fmt.Sprintf("http://%s/metrics", c.Host)
    if err := util.WaitForServer(url, timeout); err != nil {
        return "", err
    }
    response, _ := sling.New().Get(url).ReceiveBody()
    data, _ := io.ReadAll(response.Body)
    if matches := publicUrlRegex.FindStringSubmatch(string(data)); len(matches) > 0 {
        return matches[1], nil  // e.g., "https://xxx.trycloudflare.com"
    }
    return "", ErrTunnelNotFound
}
```

**3. ngrok API Client**

```go
// src/tunnel/ngrok.go
func (n *Ngrok) getPublicUrl(timeout time.Duration) (string, error) {
    ngrokUrl := fmt.Sprintf("http://%s/api/tunnels", n.Host)
    var tunnels Tunnels
    sling.New().Get(ngrokUrl).ReceiveSuccess(&tunnels)
    for _, tunnel := range tunnels.Tunnels {
        if strings.EqualFold(tunnel.Proto, n.Proto) {
            return tunnel.PublicURL, nil
        }
    }
    return "", ErrTunnelNotFound
}
```

**4. Resolver Pattern for Storage**

```go
// src/storage/storage.go
var Apps = newAppResolver()
var Profiles = newProfileResolver()
var Jobs = newJobResolver()
var Uploads = newUploadResolver()

func Load() {
    appsPath = filepath.Join(config.Current.SaveDir, "apps")
    // Create required directories
    for _, path := range requiredPaths {
        os.MkdirAll(path, os.ModePerm)
    }
    // Refresh each resolver
    Apps.refresh()
    Profiles.refresh()
    Uploads.refresh()
}
```

### Relevance to xplat

- **Tunnel abstraction** - Same pattern for synccf tunnel management
- **Cloudflared metrics scraping** - Get tunnel URL without manual config
- **Builder coordination** - Pattern for distributed job execution
- **File-based storage** - Simple resolver pattern for local state

---

## SignTools-Builder - macOS Signing Worker

### What It Does

Lightweight HTTP server that runs on macOS to:
- Accept signing jobs from SignTools service
- Execute signing scripts with secrets
- Queue and serialize job execution

### Architecture

Single-file implementation with Echo web framework:

```go
// Endpoints
GET  /status   -> Job queue status
POST /trigger  -> Queue new signing job (auth required)
POST /secrets  -> Update signing secrets (auth required)

// Job execution
jobChan := make(chan bool, 1000)   // Job queue
workerChan := make(chan bool, 1)   // Single worker
```

### Key Patterns

**1. Simple Job Queue**

```go
jobChan := make(chan bool, 1000)   // Pending jobs
workerChan := make(chan bool, 1)   // Active jobs (max 1)

go func() {
    for {
        <-jobChan            // Wait for job
        workerChan <- true   // Mark worker busy

        // Execute job with timeout
        ctx, cancel := context.WithTimeout(context.Background(), timeout)
        cmd := exec.CommandContext(ctx, entrypoint)
        cmd.Env = append(os.Environ(), secrets...)
        output, err := cmd.CombinedOutput()

        cancel()
        <-workerChan         // Mark worker free
    }
}()
```

**2. Atomic Secret Storage**

```go
var secrets atomic.Value

// POST /secrets - Update secrets atomically
func secretsHandler(c echo.Context) error {
    params, _ := url.ParseQuery(string(bodyBytes))
    newSecrets := map[string]string{}
    for key, val := range params {
        newSecrets[key] = val[0]
    }
    secrets.Store(newSecrets)  // Atomic swap
    return c.NoContent(200)
}

// Usage in job
signEnv := os.Environ()
for key, val := range secrets.Load().(map[string]string) {
    signEnv = append(signEnv, key+"="+val)
}
```

**3. Key-Based Auth Middleware**

```go
keyAuth := middleware.KeyAuth(func(s string, c echo.Context) (bool, error) {
    return s == *authKey, nil
})

e.POST("/trigger", triggerHandler, keyAuth)
e.POST("/secrets", secretsHandler, keyAuth)
```

### Relevance to xplat

- **Job queue pattern** - Simple channel-based queue for background tasks
- **Secret injection** - Pass secrets to subprocesses via environment
- **Key auth** - Simple bearer token authentication

---

## Summary: Useful Patterns for xplat

### Tunnel Management

| Pattern | Source | How to Use |
|---------|--------|------------|
| Provider interface | SignTools | Abstract tunnel backends (cloudflared, ngrok) |
| Cloudflared metrics scraping | SignTools | Auto-discover tunnel URL without config |
| ngrok API | SignTools | Query ngrok for public URL |

### Cloudflare API

| Pattern | Source | How to Use |
|---------|--------|------------|
| TLS 1.2 enforcement | wgcf | Required for some CF APIs |
| OpenAPI codegen | wgcf | Generate type-safe clients |
| API client wrapper | wgcf | Centralize auth token handling |

### Job Execution

| Pattern | Source | How to Use |
|---------|--------|------------|
| Channel-based queue | SignTools-Builder | Simple job queue with backpressure |
| Context timeout | SignTools-Builder | Enforce job time limits |
| Atomic secrets | SignTools-Builder | Thread-safe secret updates |
| Key auth | SignTools-Builder | Simple API authentication |

### Storage

| Pattern | Source | How to Use |
|---------|--------|------------|
| Resolver pattern | SignTools | Organize file-based storage |
| TOML config | wgcf | Persist device/account credentials |

---

## Potential Integration Points

### 1. Tunnel Provider for synccf

Add cloudflared URL discovery to `internal/synccf/`:

```go
// Auto-discover tunnel URL from cloudflared metrics
type CloudflaredTunnel struct {
    MetricsHost string  // e.g., "localhost:51881"
}

func (c *CloudflaredTunnel) GetPublicURL() (string, error) {
    // Scrape metrics for userHostname
}
```

### 2. Job Queue for Background Tasks

Use SignTools-Builder pattern for xplat background jobs:

```go
type JobQueue struct {
    pending chan Job
    active  chan struct{}
}

func (q *JobQueue) Submit(job Job) error {
    select {
    case q.pending <- job:
        return nil
    default:
        return ErrQueueFull
    }
}
```

### 3. OpenAPI Client for CF APIs

Generate type-safe Cloudflare API client from OpenAPI spec (like wgcf does for WARP API).

---

## References

- wgcf source: `.src/wgcf/`
- SignTools source: `.src/SignTools/`
- SignTools-Builder source: `.src/SignTools-Builder/`
- cloudflared: https://github.com/cloudflare/cloudflared
- ngrok API: https://ngrok.com/docs/api/
