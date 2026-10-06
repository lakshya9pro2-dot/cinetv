package main

import (
	"os"
	"strconv"
	"time"
)

// Config holds runtime configuration settings.
type Config struct {
	Port            string
	ExtractorURL    string
	VidFastBaseURL  string
	VidaraURL       string
	StreamTapeURL   string
	FilminURL       string
	TMDBKey         string
	DataURL         string
	DataFilePath    string
	RequestTimeout  time.Duration
}

// LoadConfig initializes Config from environment variables or sensible defaults.
func LoadConfig() *Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	extractorURL := os.Getenv("EXTRACTOR_URL")
	if extractorURL == "" {
		extractorURL = "https://video-getter.onrender.com"
	}

	vidfastBaseURL := os.Getenv("VIDFAST_BASE_URL")
	if vidfastBaseURL == "" {
		vidfastBaseURL = "https://vidfast.vc"
	}

	vidaraURL := os.Getenv("VIDARA_BASE_URL")
	if vidaraURL == "" {
		vidaraURL = "https://vidara.to"
	}

	streamTapeURL := os.Getenv("STREAMTAPE_BASE_URL")
	if streamTapeURL == "" {
		streamTapeURL = "https://streamtape.com"
	}

	filminURL := os.Getenv("FILMIN_BASE_URL")
	if filminURL == "" {
		filminURL = "https://filmin.ajfysu.com"
	}

	tmdbKey := os.Getenv("TMDB_KEY")
	if tmdbKey == "" {
		tmdbKey = "e6333b32409e02a4a6eba6fb7ff866bb"
	}

	dataURL := os.Getenv("DATA_URL")
	if dataURL == "" {
		dataURL = "https://www.jsonkeeper.com/b/FWSAK"
	}

	dataFilePath := os.Getenv("DATA_FILE")
	if dataFilePath == "" {
		dataFilePath = "data/vidara.json"
	}

	timeoutSec := 10
	if secStr := os.Getenv("REQUEST_TIMEOUT_SECONDS"); secStr != "" {
		if s, err := strconv.Atoi(secStr); err == nil && s > 0 {
			timeoutSec = s
		}
	}

	return &Config{
		Port:           port,
		ExtractorURL:   extractorURL,
		VidFastBaseURL: vidfastBaseURL,
		VidaraURL:      vidaraURL,
		StreamTapeURL:  streamTapeURL,
		FilminURL:      filminURL,
		TMDBKey:        tmdbKey,
		DataURL:        dataURL,
		DataFilePath:   dataFilePath,
		RequestTimeout: time.Duration(timeoutSec) * time.Second,
	}
}
