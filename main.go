package main

import (
	"fmt"
	"log"
	"net/http"

	"vidara-api/handlers"
	"vidara-api/resolver"
	"vidara-api/tiers"
)

func main() {
	cfg := LoadConfig()

	log.Printf("[*] Initializing 3-Tier Multi-Source Streaming API...")

	// 1. Tier 1 - Vidara
	tier1 := tiers.NewVidaraTier(cfg.VidaraURL, cfg.StreamTapeURL, cfg.RequestTimeout)
	if err := tier1.LoadData(cfg.DataURL, cfg.DataFilePath, "data/vidara.json", "data.json"); err != nil {
		log.Printf("[!] Warning loading Vidara data: %v", err)
	}

	// 2. Tier 2 - Extractor API
	tier2 := tiers.NewExtractorTier(cfg.ExtractorURL, cfg.VidFastBaseURL, cfg.RequestTimeout)

	// 3. Tier 3 - CineTV
	tier3 := tiers.NewCineTVTier(cfg.FilminURL, cfg.TMDBKey, cfg.RequestTimeout)

	// Central Fallback Resolver
	centralResolver := resolver.NewCentralResolver(tier1, tier2, tier3)

	// API Handlers
	apiHandler := handlers.NewAPIHandler(centralResolver, tier1, cfg.RequestTimeout)

	mux := http.NewServeMux()

	// Core 3-Tier endpoints
	mux.HandleFunc("/health", apiHandler.HandleHealth)
	mux.HandleFunc("/api/movie/", apiHandler.HandleMovie)
	mux.HandleFunc("/api/tv/", apiHandler.HandleTV)

	// Preserved Vidara app.py endpoints
	mux.HandleFunc("/api/resolve", apiHandler.HandleResolve)
	mux.HandleFunc("/api/resolve/", apiHandler.HandleResolve)
	mux.HandleFunc("/api/extract", apiHandler.HandleExtract)
	mux.HandleFunc("/api/proxy/stream", apiHandler.HandleProxyStream)
	mux.HandleFunc("/api/vidara", apiHandler.HandleVidaraDefault)
	mux.HandleFunc("/api/streamtape", apiHandler.HandleStreamTapeDefault)
	mux.HandleFunc("/text/vidara", apiHandler.HandleTextVidara)
	mux.HandleFunc("/text/streamtape", apiHandler.HandleTextStreamTape)
	mux.HandleFunc("/playlist.m3u8", apiHandler.HandleMasterPlaylist)
	mux.HandleFunc("/api/playlist.m3u8", apiHandler.HandleMasterPlaylist)
	mux.HandleFunc("/player.html", apiHandler.HandlePlayer)
	mux.HandleFunc("/player.html/", apiHandler.HandlePlayer)
	mux.HandleFunc("/player", apiHandler.HandlePlayer)
	mux.HandleFunc("/player/", apiHandler.HandlePlayer)
	mux.HandleFunc("/hls.min.js", apiHandler.HandleHlsJs)
	mux.HandleFunc("/", apiHandler.HandlePlayer)

	addr := fmt.Sprintf("0.0.0.0:%s", cfg.Port)
	log.Printf("[*] Server listening on http://%s", addr)
	if err := http.ListenAndServe(":"+cfg.Port, mux); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
