package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"vidara-api/models"
	"vidara-api/resolver"
	"vidara-api/tiers"
)

const (
	StreamtapeDefaultURL = "https://streamtape.com/v/0A2vDYQz3wIbPQ6/"
	VidaraDefaultURL     = "https://vidara.to/v/d932127894f1"
)

// APIHandler handles HTTP requests for the multi-tier streaming API.
type APIHandler struct {
	resolver   *resolver.CentralResolver
	vidaraTier *tiers.VidaraTier
	httpClient *http.Client
}

// NewAPIHandler creates a new APIHandler instance.
func NewAPIHandler(res *resolver.CentralResolver, vidaraTier *tiers.VidaraTier, timeout time.Duration) *APIHandler {
	return &APIHandler{
		resolver:   res,
		vidaraTier: vidaraTier,
		httpClient: &http.Client{Timeout: timeout},
	}
}

// AddCORSHeaders sets standard permissive CORS headers on the response.
func AddCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, HEAD")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Range, Content-Length, Accept-Ranges")
}

// ResponseWriterWrapper captures status code for HTTP request logging.
type ResponseWriterWrapper struct {
	http.ResponseWriter
	StatusCode int
}

func (rw *ResponseWriterWrapper) WriteHeader(code int) {
	rw.StatusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// LoggingMiddleware logs all incoming HTTP requests with method, path, status, duration, and client IP.
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &ResponseWriterWrapper{ResponseWriter: w, StatusCode: http.StatusOK}
		next.ServeHTTP(rw, r)
		duration := time.Since(start)
		log.Printf("[http] %d %s %s (%s) from %s", rw.StatusCode, r.Method, r.URL.RequestURI(), duration, r.RemoteAddr)
	})
}

// HandleHealth handles GET /health.
func (h *APIHandler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	AddCORSHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	log.Printf("[api] /health requested from %s", r.RemoteAddr)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
	})
}

// HandleMovie handles GET /api/movie/{tmdb_id}.
func (h *APIHandler) HandleMovie(w http.ResponseWriter, r *http.Request) {
	AddCORSHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	rawID := strings.TrimPrefix(r.URL.Path, "/api/movie/")
	rawID = strings.Trim(rawID, "/")
	log.Printf("[api] incoming movie request: id='%s' from %s", rawID, r.RemoteAddr)

	tmdbID, err := strconv.Atoi(rawID)
	if err != nil || tmdbID <= 0 {
		log.Printf("[api] bad request: invalid movie tmdb_id '%s'", rawID)
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "invalid_parameters",
			"message": "tmdb_id must be a positive integer",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 35*time.Second)
	defer cancel()

	result, err := h.resolver.ResolveMovie(ctx, tmdbID)
	if err != nil || result == nil || !result.Success {
		log.Printf("[api] movie %d NOT FOUND across all tiers (err: %v)", tmdbID, err)
		w.WriteHeader(http.StatusNotFound)
		if result == nil {
			result = &models.ResolutionResult{
				Success: false,
				TmdbID:  tmdbID,
				Error:   "source_not_found",
			}
		}
		_ = json.NewEncoder(w).Encode(result)
		return
	}

	log.Printf("[api] movie %d successfully resolved via Tier %d (%s): url=%s", tmdbID, result.Tier, result.Source, result.URL)
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(result)
}

// HandleTV handles GET /api/tv/{tmdb_id}/{season_number}/{episode_number}.
func (h *APIHandler) HandleTV(w http.ResponseWriter, r *http.Request) {
	AddCORSHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	path := strings.TrimPrefix(r.URL.Path, "/api/tv/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	log.Printf("[api] incoming TV request: path='%s' from %s", path, r.RemoteAddr)

	if len(parts) < 3 {
		log.Printf("[api] bad request: insufficient TV route parts '%s'", path)
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "invalid_parameters",
			"message": "usage: /api/tv/{tmdb_id}/{season}/{episode}",
		})
		return
	}

	tmdbID, errID := strconv.Atoi(parts[0])
	season, errS := strconv.Atoi(parts[1])
	episode, errE := strconv.Atoi(parts[2])

	if errID != nil || errS != nil || errE != nil || tmdbID <= 0 || season <= 0 || episode <= 0 {
		log.Printf("[api] bad request: invalid TV parameters in path '%s'", path)
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "invalid_parameters",
			"message": "tmdb_id, season, and episode must be positive integers",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 35*time.Second)
	defer cancel()

	result, err := h.resolver.ResolveTV(ctx, tmdbID, season, episode)
	if err != nil || result == nil || !result.Success {
		log.Printf("[api] TV %d S%dE%d NOT FOUND across all tiers (err: %v)", tmdbID, season, episode, err)
		w.WriteHeader(http.StatusNotFound)
		if result == nil {
			result = &models.ResolutionResult{
				Success: false,
				TmdbID:  tmdbID,
				Season:  &season,
				Episode: &episode,
				Error:   "source_not_found",
			}
		}
		_ = json.NewEncoder(w).Encode(result)
		return
	}

	log.Printf("[api] TV %d S%dE%d successfully resolved via Tier %d (%s): url=%s", tmdbID, season, episode, result.Tier, result.Source, result.URL)
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(result)
}

// HandleResolve handles GET / POST /api/resolve (preserving app.py functionality).
func (h *APIHandler) HandleResolve(w http.ResponseWriter, r *http.Request) {
	AddCORSHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	stParsed, vaParsed := tiers.ParseIDsFromString(r.URL.RequestURI())
	stInput := stParsed
	vaInput := vaParsed

	if r.Method == http.MethodPost {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if st, ok := body["st"].(string); ok && st != "" {
			stInput = st
		} else if play, ok := body["play"].(string); ok && play != "" {
			stInput = play
		}
		if va, ok := body["va"].(string); ok && va != "" {
			vaInput = va
		} else if playd, ok := body["playd"].(string); ok && playd != "" {
			vaInput = playd
		}
	} else {
		q := r.URL.Query()
		if st := q.Get("st"); st != "" {
			stInput = st
		}
		if va := q.Get("va"); va != "" {
			vaInput = va
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	log.Printf("[api] resolving dual stream: st=%s, va=%s from %s", stInput, vaInput, r.RemoteAddr)
	resp := h.vidaraTier.ResolveDual(ctx, stInput, vaInput)
	log.Printf("[api] dual resolution finished: status=%s, mode=%s (st=%s, va=%s)", resp.Status, resp.Mode, resp.STID, resp.VAID)
	_ = json.NewEncoder(w).Encode(resp)
}

// HandleExtract handles GET / POST /api/extract (preserving app.py functionality).
func (h *APIHandler) HandleExtract(w http.ResponseWriter, r *http.Request) {
	AddCORSHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	targetURL := r.URL.Query().Get("url")
	formatType := strings.ToLower(r.URL.Query().Get("format"))

	if r.Method == http.MethodPost && targetURL == "" {
		_ = r.ParseForm()
		targetURL = r.FormValue("url")
		if targetURL == "" {
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if u, ok := body["url"].(string); ok {
				targetURL = u
			}
		}
	}

	if targetURL == "" {
		log.Printf("[api] /api/extract bad request: missing url parameter from %s", r.RemoteAddr)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": "Missing 'url' parameter",
		})
		return
	}

	log.Printf("[api] /api/extract extracting url: %s from %s", targetURL, r.RemoteAddr)
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	result, err := h.vidaraTier.ExtractAny(ctx, targetURL)
	if err != nil {
		log.Printf("[api] /api/extract error for %s: %v", targetURL, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": err.Error(),
			"url":     targetURL,
		})
		return
	}

	log.Printf("[api] /api/extract success for %s: title='%s', url=%s", targetURL, result.Title, result.URL)

	accept := r.Header.Get("Accept")
	if formatType == "text" || strings.Contains(accept, "text/plain") {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "Text: %s\nURL: %s\n", result.Text, result.URL)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(result)
}

// HandleProxyStream handles GET / HEAD /api/proxy/stream with byte range support.
func (h *APIHandler) HandleProxyStream(w http.ResponseWriter, r *http.Request) {
	AddCORSHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	targetURL := r.URL.Query().Get("url")
	if targetURL == "" {
		log.Printf("[api] proxy stream bad request: missing url parameter from %s", r.RemoteAddr)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": "Missing 'url' parameter",
		})
		return
	}

	log.Printf("[api] proxying stream: method=%s, url=%s, range='%s' from %s", r.Method, targetURL, r.Header.Get("Range"), r.RemoteAddr)
	req, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, nil)
	if err != nil {
		log.Printf("[api] proxy stream request creation error: %v", err)
		http.Error(w, "Proxy error: "+err.Error(), http.StatusBadGateway)
		return
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "*/*")
	if rg := r.Header.Get("Range"); rg != "" {
		req.Header.Set("Range", rg)
	}

	upstream, err := h.httpClient.Do(req)
	if err != nil {
		log.Printf("[api] proxy upstream connection error for %s: %v", targetURL, err)
		http.Error(w, "Proxy error: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer upstream.Body.Close()

	log.Printf("[api] proxy upstream responded: status=%d, content-type='%s', content-length='%s'", upstream.StatusCode, upstream.Header.Get("Content-Type"), upstream.Header.Get("Content-Length"))

	for _, k := range []string{"Content-Range", "Content-Length", "Content-Type", "Accept-Ranges"} {
		if v := upstream.Header.Get(k); v != "" {
			w.Header().Set(k, v)
		}
	}
	w.Header().Set("Accept-Ranges", "bytes")

	w.WriteHeader(upstream.StatusCode)
	if r.Method == http.MethodHead {
		return
	}

	buf := make([]byte, 128*1024)
	copied, copyErr := io.CopyBuffer(w, upstream.Body, buf)
	if copyErr != nil {
		log.Printf("[api] proxy stream transfer error: %v (copied %d bytes)", copyErr, copied)
	}
}

// HandleStreamTapeDefault handles GET /api/streamtape.
func (h *APIHandler) HandleStreamTapeDefault(w http.ResponseWriter, r *http.Request) {
	AddCORSHeaders(w)
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	log.Printf("[api] resolving default streamtape stream from %s", r.RemoteAddr)
	result, err := h.vidaraTier.ExtractStreamTape(ctx, StreamtapeDefaultURL)
	if err != nil {
		log.Printf("[api] default streamtape error: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": err.Error()})
		return
	}

	if r.URL.Query().Get("format") == "text" || strings.Contains(r.Header.Get("Accept"), "text/plain") {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "Text: %s\nURL: %s\n", result.Text, result.URL)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

// HandleVidaraDefault handles GET /api/vidara.
func (h *APIHandler) HandleVidaraDefault(w http.ResponseWriter, r *http.Request) {
	AddCORSHeaders(w)
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	log.Printf("[api] resolving default vidara stream from %s", r.RemoteAddr)
	result, err := h.vidaraTier.ExtractVidara(ctx, VidaraDefaultURL)
	if err != nil {
		log.Printf("[api] default vidara error: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": err.Error()})
		return
	}

	if r.URL.Query().Get("format") == "text" || strings.Contains(r.Header.Get("Accept"), "text/plain") {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "Text: %s\nURL: %s\n", result.Text, result.URL)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

// HandleTextStreamTape handles GET /text/streamtape.
func (h *APIHandler) HandleTextStreamTape(w http.ResponseWriter, r *http.Request) {
	AddCORSHeaders(w)
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	result, err := h.vidaraTier.ExtractStreamTape(ctx, StreamtapeDefaultURL)
	if err != nil {
		log.Printf("[api] text streamtape error: %v", err)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Error: %v\n", err)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "Text: %s\nURL: %s\n", result.Text, result.URL)
}

// HandleTextVidara handles GET /text/vidara.
func (h *APIHandler) HandleTextVidara(w http.ResponseWriter, r *http.Request) {
	AddCORSHeaders(w)
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	result, err := h.vidaraTier.ExtractVidara(ctx, VidaraDefaultURL)
	if err != nil {
		log.Printf("[api] text vidara error: %v", err)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Error: %v\n", err)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "Text: %s\nURL: %s\n", result.Text, result.URL)
}

// HandleMasterPlaylist handles GET /playlist.m3u8 and /api/playlist.m3u8.
func (h *APIHandler) HandleMasterPlaylist(w http.ResponseWriter, r *http.Request) {
	AddCORSHeaders(w)
	vaID := r.URL.Query().Get("va")
	if vaID == "" {
		vaID = "d932127894f1"
	}

	log.Printf("[api] master playlist requested for va=%s from %s", vaID, r.RemoteAddr)
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	vaRes, err := h.vidaraTier.ExtractVidara(ctx, vaID)
	if err == nil && vaRes != nil && strings.HasPrefix(vaRes.URL, "http") {
		req, err := http.NewRequestWithContext(ctx, "GET", vaRes.URL, nil)
		if err == nil {
			req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64)")
			resp, err := h.httpClient.Do(req)
			if err == nil && resp.StatusCode == http.StatusOK {
				defer resp.Body.Close()
				log.Printf("[api] successfully proxied master playlist for va=%s", vaID)
				_, _ = io.Copy(w, resp.Body)
				return
			}
		}
	}

	log.Printf("[api] fallback master playlist served for va=%s", vaID)
	w.Write([]byte("#EXTM3U\n#EXT-X-VERSION:6\n"))
}

// HandlePlayer serves player.html if available or simple HTML fallback.
func (h *APIHandler) HandlePlayer(w http.ResponseWriter, r *http.Request) {
	AddCORSHeaders(w)
	playerPath := tiers.FindFirstExistingFile("player.html", "cinetv/player.html")
	if playerPath != "" {
		log.Printf("[api] serving player file: %s for %s", playerPath, r.URL.Path)
		http.ServeFile(w, r, playerPath)
		return
	}

	log.Printf("[api] serving default HTML player page for %s", r.URL.Path)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	html := `<!DOCTYPE html>
<html>
<head><title>Vidara Player</title></head>
<body style="font-family:sans-serif;padding:20px;">
  <h1>Vidara 3-Tier Multi-Source Streaming API</h1>
  <p>Status: Running</p>
  <ul>
    <li><a href="/health">/health</a></li>
    <li>Movie API: <code>/api/movie/{tmdb_id}</code></li>
    <li>TV API: <code>/api/tv/{tmdb_id}/{season}/{episode}</code></li>
  </ul>
</body>
</html>`
	w.Write([]byte(html))
}

// HandleHlsJs serves hls.min.js if found.
func (h *APIHandler) HandleHlsJs(w http.ResponseWriter, r *http.Request) {
	AddCORSHeaders(w)
	jsPath := tiers.FindFirstExistingFile("hls.min.js")
	if jsPath != "" {
		w.Header().Set("Content-Type", "application/javascript")
		http.ServeFile(w, r, jsPath)
		return
	}
	w.Header().Set("Content-Type", "application/javascript")
	w.Write([]byte("// HLS not cached locally\n"))
}
