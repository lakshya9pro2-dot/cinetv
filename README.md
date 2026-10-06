# Vidara 3-Tier Multi-Source Movie/TV Streaming API (Go)

A high-performance backend API written in Go that resolves standard **TMDB IDs** (for both Movies and TV Shows) into signed, playable streaming URLs across a unified **3-tier fallback architecture**.

This project converts the legacy Python implementation (`app.py`) into idiomatic, concurrent Go with zero third-party dependencies, integrating:
- **Tier 1: Vidara & Streamtape** (Local in-memory index + live upstream resolution)
- **Tier 2: VidFast Extractor API** (`/extract?url=...&timeout=20`)
- **Tier 3: CineTV / Filmin** (Cryptographic signing, token generation, AES/3DES decryption, and VOD extraction)

---

## Table of Contents
1. [Architecture & Fallback Order](#1-architecture--fallback-order)
2. [Quickstart & Usage](#2-quickstart--usage)
3. [API Documentation](#3-api-documentation)
4. [Streaming Headers (`Referer` & `Origin`)](#4-streaming-headers-referer--origin)
5. [Configuration & Environment Variables](#5-configuration--environment-variables)
6. [Codebase Guide for AI & Developers](#6-codebase-guide-for-ai--developers)
7. [Testing](#7-testing)

---

## 1. Architecture & Fallback Order

The client only provides standard TMDB IDs (e.g. `550` or `262838/1/2`). The server queries tiers in strict priority:

```text
Incoming Client Request (/api/movie/{tmdb_id} or /api/tv/{tmdb_id}/{season}/{episode})
   │
   ▼
Tier 1: Vidara
   │
   ├── Found & Resolved? ──► Return HTTP 200 (tier: 1, source: "vidara")
   │
   └── Not found / unavailable
          │
          ▼
      Tier 2: VidFast Extractor
          │
          ├── Found & Valid URL? ──► Return HTTP 200 (tier: 2, source: "vidfast-extractor")
          │
          └── Failed / timeout
                 │
                 ▼
             Tier 3: CineTV (Filmin)
                 │
                 ├── Found & Signed URL? ──► Return HTTP 200 (tier: 3, source: "cinetv")
                 │
                 └── Not found
                        │
                        ▼
                   Return Clean HTTP 404 {"success": false, "error": "source_not_found"}
```

### Strict Short-Circuit Rules
- **Tier 1 succeeds** $\rightarrow$ Tier 2 and Tier 3 are **never called**.
- **Tier 2 succeeds** $\rightarrow$ Tier 3 is **never called**.
- **Tier isolation**: Failures, network timeouts, or invalid JSON on one tier never crash the server and automatically cascade to the next tier.

---

## 2. Quickstart & Usage

### Prerequisites
- **Go 1.22+** (tested on Go 1.26.0)

### Build
Compile the native binary with zero external dependencies:
```bash
go build -o vidara-server .
```

### Run
```bash
# Run on default port 8080
./vidara-server

# Or run directly with Go
go run .

# Or specify custom environment variables
PORT=8080 EXTRACTOR_URL=http://192.168.1.2:8080 ./vidara-server
```

---

## 3. API Documentation

### A. Movie Endpoint
```http
GET /api/movie/{tmdb_id}
```
Resolves a movie using TMDB movie ID across Tier 1 $\rightarrow$ Tier 2 $\rightarrow$ Tier 3.

#### Example: Tier 1 Resolution (Local Dataset)
```bash
curl -s http://localhost:8080/api/movie/1167307
```
**Response (HTTP 200):**
```json
{
  "success": true,
  "tmdb_id": 1167307,
  "type": "movie",
  "tier": 1,
  "source": "vidara",
  "title": "Obsession",
  "url": "https://s7-s1001604.97bf1.com/hls/I68vNTnIoWTmgFsypD3Xk51Sfhorw8FB/master.m3u8?token=...",
  "headers": {
    "Origin": "https://vidara.to",
    "Referer": "https://vidara.to/"
  },
  "play": {
    "va": "mzfdFOguU87x",
    "st": "6a44824dcd175"
  }
}
```

#### Example: Tier 2 Resolution (VidFast Extractor)
```bash
curl -s http://localhost:8080/api/movie/126560
```
**Response (HTTP 200):**
```json
{
  "success": true,
  "tmdb_id": 126560,
  "type": "movie",
  "tier": 2,
  "source": "vidfast-extractor",
  "url": "https://moon.zenoak.top/vd/.../master.m3u8",
  "headers": {
    "Origin": "https://vidfast.vc",
    "Referer": "https://vidfast.vc/"
  }
}
```

#### Example: Tier 3 Resolution (CineTV / Filmin)
```bash
curl -s http://localhost:8080/api/movie/550
```
**Response (HTTP 200):**
```json
{
  "success": true,
  "tmdb_id": 550,
  "type": "movie",
  "tier": 3,
  "source": "cinetv",
  "name": "Fight Club",
  "title": "Fight Club",
  "url": "http://758d5c9d4a019e.e6r4r1.com/vod/1/2025/04/03/604be407c1fe/1135529346.mp4?wsSecret=...&wsTime=...",
  "headers": {
    "Origin": "https://filmin.ajfysu.com",
    "Referer": "https://filmin.ajfysu.com/"
  }
}
```

#### Example: Complete Failure
```bash
curl -s http://localhost:8080/api/movie/999999999
```
**Response (HTTP 404):**
```json
{
  "success": false,
  "tmdb_id": 999999999,
  "error": "source_not_found"
}
```

---

### B. TV Show Endpoint
```http
GET /api/tv/{tmdb_id}/{season_number}/{episode_number}
```
Resolves a specific TV episode with exact season and episode matching.

#### Example: Tier 1 TV Resolution
```bash
curl -s http://localhost:8080/api/tv/262838/1/2
```
**Response (HTTP 200):**
```json
{
  "success": true,
  "tmdb_id": 262838,
  "type": "tv",
  "season": 1,
  "episode": 2,
  "tier": 1,
  "source": "vidara",
  "title": "India's Got Latent",
  "headers": {
    "Origin": "https://vidara.to",
    "Referer": "https://vidara.to/"
  },
  "play": {
    "va": "6a4482e9ae150",
    "st": "6a44824dcd175"
  }
}
```

#### Example: Tier 3 TV Fallback (CineTV)
```bash
curl -s http://localhost:8080/api/tv/1399/1/6
```
**Response (HTTP 200):**
```json
{
  "success": true,
  "tmdb_id": 1399,
  "type": "tv",
  "tier": 3,
  "source": "cinetv",
  "name": "Game of Thrones",
  "title": "Game of Thrones",
  "season": 1,
  "episode": 6,
  "url": "http://7590e5287be439.e6r4r1.com/vod/1/2026/06/18/60565ddb818f/534473569.mp4?wsSecret=...&wsTime=...",
  "headers": {
    "Origin": "https://filmin.ajfysu.com",
    "Referer": "https://filmin.ajfysu.com/"
  }
}
```

---

### C. Health Check
```http
GET /health
```
**Response (HTTP 200):**
```json
{
  "status": "ok"
}
```

---

### D. Preserved Legacy Vidara (`app.py`) Endpoints

| Endpoint | Methods | Description |
|---|---|---|
| `/api/resolve` | `GET`, `POST` | Dual parallel resolver for `va` and `st` IDs with fallback. |
| `/api/extract` | `GET`, `POST` | Universal stream extractor for Vidara/Streamtape URLs. |
| `/api/proxy/stream` | `GET`, `HEAD` | Media stream chunk proxy supporting HTTP byte `Range` requests and CORS. |
| `/api/vidara` | `GET` | Resolves default Vidara stream. |
| `/api/streamtape` | `GET` | Resolves default Streamtape stream. |
| `/playlist.m3u8` | `GET` | Master HLS playlist proxy. |
| `/player.html`, `/player`, `/` | `GET` | HTML video player interface. |

---

## 4. Streaming Headers (`Referer` & `Origin`)

To prevent CDN hotlink protection and CORS blocking in modern video players (such as Hls.js, ExoPlayer, VLC, or HTML5 `<video>`), every resolved stream provides the required playback headers:

| Tier | Source | `Referer` Header | `Origin` Header |
|---|---|---|---|
| **Tier 1** | Vidara | `https://vidara.to/` | `https://vidara.to` |
| **Tier 2** | VidFast Extractor | `https://vidfast.vc/` | `https://vidfast.vc` |
| **Tier 3** | CineTV / Filmin | `https://filmin.ajfysu.com/` | `https://filmin.ajfysu.com` |

---

## 5. Configuration & Environment Variables

All settings can be configured via environment variables with safe defaults:

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | Port for the HTTP server to listen on. |
| `EXTRACTOR_URL` | `http://192.168.1.2:8080` | Base URL of the Tier 2 VidFast Extractor service. |
| `VIDFAST_BASE_URL` | `https://vidfast.vc` | Target URL prefix used for VidFast movie/tv requests. |
| `VIDARA_BASE_URL` | `https://vidara.to` | Base URL of the Vidara upstream streaming API. |
| `STREAMTAPE_BASE_URL` | `https://streamtape.com` | Base URL of Streamtape upstream. |
| `FILMIN_BASE_URL` | `https://filmin.ajfysu.com` | Upstream CineTV / Filmin API base URL. |
| `TMDB_KEY` | `e6333b32409e02a4a6eba6fb7ff866bb` | TMDB API v3 key for title and ID lookups. |
| `DATA_FILE` | `data/vidara.json` | Path to the local Vidara JSON dataset. |
| `REQUEST_TIMEOUT_SECONDS` | `10` | Default timeout duration for external network requests. |

---

## 6. Codebase Guide for AI & Developers

This section explains the internals so any AI model or developer can navigate, maintain, and extend the codebase.

```text
.
├── config.go            # Runtime configuration loader
├── data/
│   └── vidara.json      # Clean, normalized Tier 1 dataset
├── data.json            # Original dataset (with auto-repairing parser)
├── go.mod               # Pure Go module definition (zero dependencies)
├── handlers/
│   ├── api.go           # HTTP handlers, validation, CORS, and proxy routes
│   └── api_test.go      # Integration test suite for HTTP API endpoints
├── main.go              # Application bootstrap and router binding
├── models/
│   └── models.go        # Unified response models and data transfer objects
├── resolver/
│   ├── resolver.go      # Central 3-tier fallback orchestrator
│   └── resolver_test.go # Fallback order unit test suite
├── tiers/
│   ├── tier1_vidara.go  # Tier 1 in-memory store, JSON parser, and stream extractors
│   ├── tier1_vidara_test.go # Tier 1 indexing and parsing tests
│   ├── tier2_extractor.go # Tier 2 VidFast extractor client
│   ├── tier2_extractor_test.go # Tier 2 URL encoding and timeout tests
│   └── tier3_cinetv.go  # Tier 3 Filmin cryptosystem and stream signer
└── README.md
```

### Module Breakdown

#### 1. [`models/models.go`](file:///home/linux/Desktop/file/models/models.go)
Defines shared structs used across all tiers:
- `ResolutionResult`: The standardized response object. Contains `success`, `tmdb_id`, `type`, `tier`, `source`, optional `title`/`name`, `season`, `episode`, `url`, `headers`, `play`, and `error`.
- `Playback`: Holds `"va"` and `"st"` file codes.
- `ExtractorResponse`: Unified model for extraction results from Vidara, Streamtape, or VidFast.
- `AudioTrack`: Dynamic HLS multi-track audio descriptors extracted from `.m3u8`.

#### 2. [`tiers/tier1_vidara.go`](file:///home/linux/Desktop/file/tiers/tier1_vidara.go)
Handles Tier 1 resolution:
- **In-Memory Indexing**:
  - `movieIndex map[int]MovieRecord`: Instant $O(1)$ lookup for movies by TMDB ID.
  - `tvIndex map[int]map[int]map[int]TVEpisodeRecord`: Instant $O(1)$ lookup by TMDB ID $\rightarrow$ Season $\rightarrow$ Episode.
- **Resilient Dataset Parser**:
  - Automatically loads and sanitizes both valid JSON (`data/vidara.json`) and malformed legacy structures (`data.json`).
  - Repairs syntax errors such as `"play": "va": "...", "st": "..."`, trailing quotes, duplicate season chains, and trailing commas.
- **Stream Extraction**:
  - `ExtractVidara`: POSTs to `https://vidara.to/api/stream` with `{"filecode": "...", "device": "web"}`. If `.m3u8` is returned, it parses `#EXT-X-MEDIA:TYPE=AUDIO` tracks in real time.
  - `ExtractStreamTape`: Scrapes Streamtape HTML, extracts script tokens, evaluates chained `.substring()` operations with a pure Go interpreter, follows HTTP 302 redirects to find the direct TapeContent CDN URL, and sets up a proxy route.
  - `tryResolveStream`: Asynchronously resolves playable stream URLs with an 8-second bounded timeout.

#### 3. [`tiers/tier2_extractor.go`](file:///home/linux/Desktop/file/tiers/tier2_extractor.go)
Handles Tier 2 resolution via external extractor service:
- Constructs target URLs:
  - Movie: `https://vidfast.vc/movie/{tmdb_id}`
  - TV: `https://vidfast.vc/tv/{tmdb_id}/{season}/{episode}`
- Encodes query parameters safely using `net/url`:
  - Request: `GET {EXTRACTOR_URL}/extract?url=<encoded_target>&timeout=20`
- Response validation:
  - Checks `success == true` (or `status == "success"`) and `url != ""` $\rightarrow$ returns Tier 2 success.
  - On network error, extractor `success == false`, or timeout $\rightarrow$ returns `nil, nil` to smoothly cascade to Tier 3.

#### 4. [`tiers/tier3_cinetv.go`](file:///home/linux/Desktop/file/tiers/tier3_cinetv.go)
Reimplements CineTV / Filmin provider functionality in Go:
- **Device Emulation**: Generates random 16-byte Android device IDs and emulates genuine device hardware fingerprints (`brandModels`).
- **Cryptographic Engine**:
  - **Triple-DES (3DES-CBC)**: Decrypts `SECRET_KEY_ENCRYPTED` using `DES_KEY` (`dsawdf634eebGFHITR5UT9kS0`) and `DES_IV` (`32456738`) to generate request signatures.
  - **AES-128-CBC**: Decrypts Filmin API responses with `AES_KEY_STR` (`0123456789123456`) and `AES_IV_STR` (`2015030120123456`). If response bytes start with `0x1F, 0x8B`, it automatically decompresses with GZIP.
  - **MD5 Tokenizer**: Generates dynamic P2P token signatures: `MD5(P2P_SALT + deviceID + vodID + timestamp)`.
- **TMDB & VOD Resolution**:
  - Resolves TMDB ID to title and release year.
  - Searches Filmin `/api/search/result`.
  - Queries `/api/vod/info_new` and filters `vod_collection` by episode number.
  - Generates signed streaming URLs with `wsSecret` and `wsTime` expiration tokens.

#### 5. [`resolver/resolver.go`](file:///home/linux/Desktop/file/resolver/resolver.go)
Implements the central fallback logic using Go interfaces (`TierResolver`):
```go
type TierResolver interface {
    ResolveMovie(ctx context.Context, tmdbID int) (*models.ResolutionResult, error)
    ResolveTV(ctx context.Context, tmdbID, season, episode int) (*models.ResolutionResult, error)
}
```
Coordinates `tier1.Resolve` $\rightarrow$ `tier2.Resolve` $\rightarrow$ `tier3.Resolve`, logging each step and short-circuiting on the first success.

#### 6. [`handlers/api.go`](file:///home/linux/Desktop/file/handlers/api.go)
HTTP routing and validation layer:
- Validates `tmdb_id > 0`, `season > 0`, `episode > 0`, returning `400 Bad Request` on failure.
- Injects permissive CORS headers (`Access-Control-Allow-Origin: *`, `Expose-Headers`).
- Handles stream proxying with HTTP `Range` headers for seeking in video players.

---

## 7. Testing

The project has comprehensive unit and integration test coverage:

```bash
go test -v ./...
```

### What the Tests Verify:
1. **Fallback Priority** ([`resolver/resolver_test.go`](file:///home/linux/Desktop/file/resolver/resolver_test.go)):
   - Proves Tier 1 success never invokes Tier 2 or Tier 3.
   - Proves Tier 2 success never invokes Tier 3.
   - Proves Tier 3 executes when Tiers 1 and 2 fail.
   - Proves all three failing returns a structured 404 response.
2. **Tier 1 Indexing & Parsing** ([`tiers/tier1_vidara_test.go`](file:///home/linux/Desktop/file/tiers/tier1_vidara_test.go)):
   - Tests loading both clean and malformed JSON datasets.
   - Verifies exact season and episode matching (guarantees wrong episodes are never served).
   - Tests JavaScript token evaluation and ID cleanup.
3. **Tier 2 Extractor Encoding** ([`tiers/tier2_extractor_test.go`](file:///home/linux/Desktop/file/tiers/tier2_extractor_test.go)):
   - Verifies dynamic URL generation for Movie and TV.
   - Validates that `&timeout=20` and URL encoding are sent on every request.
4. **HTTP Endpoints & Validation** ([`handlers/api_test.go`](file:///home/linux/Desktop/file/handlers/api_test.go)):
   - Verifies `/health`, `/api/movie/{id}`, and `/api/tv/{id}/{s}/{e}` across all status codes (`200`, `400`, `404`).
