package tiers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"vidara-api/models"
)

// ExtractorTier represents Tier 2 resolver connecting to VidFast Extractor API.
type ExtractorTier struct {
	extractorURL   string
	vidfastBaseURL string
	client         *http.Client
}

// NewExtractorTier creates a new Tier 2 extractor resolver.
func NewExtractorTier(extractorURL, vidfastBaseURL string, timeout time.Duration) *ExtractorTier {
	if extractorURL == "" {
		extractorURL = "https://video-getter.onrender.com"
	}
	if vidfastBaseURL == "" {
		vidfastBaseURL = "https://vidfast.vc"
	}

	clientTimeout := timeout
	if clientTimeout < 25*time.Second {
		clientTimeout = 25 * time.Second
	}

	return &ExtractorTier{
		extractorURL:   strings.TrimRight(extractorURL, "/"),
		vidfastBaseURL: strings.TrimRight(vidfastBaseURL, "/"),
		client:         &http.Client{Timeout: clientTimeout},
	}
}

// extractorAPIResponse models possible extractor response formats.
type extractorAPIResponse struct {
	Success *bool   `json:"success,omitempty"`
	Status  string  `json:"status,omitempty"`
	URL     *string `json:"url,omitempty"`
	Error   string  `json:"error,omitempty"`
	Message string  `json:"message,omitempty"`
}

// ResolveMovie resolves a movie via Tier 2 VidFast extractor.
func (t *ExtractorTier) ResolveMovie(ctx context.Context, tmdbID int) (*models.ResolutionResult, error) {
	targetURL := fmt.Sprintf("%s/movie/%d", t.vidfastBaseURL, tmdbID)
	log.Printf("[tier2] extracting VidFast URL for movie %d: %s", tmdbID, targetURL)

	streamURL, err := t.callExtractor(ctx, targetURL)
	if err != nil {
		log.Printf("[tier2] failed: %v", err)
		return nil, nil // Return nil to allow fallback to Tier 3
	}

	if streamURL == "" {
		log.Printf("[tier2] failed: extractor returned empty url")
		return nil, nil
	}

	log.Printf("[tier2] success resolving movie %d", tmdbID)
	return &models.ResolutionResult{
		Success: true,
		TmdbID:  tmdbID,
		Type:    "movie",
		Tier:    2,
		Source:  "vidfast-extractor",
		URL:     streamURL,
		Headers: map[string]string{
			"Referer": t.vidfastBaseURL + "/",
			"Origin":  t.vidfastBaseURL,
		},
	}, nil
}

// ResolveTV resolves a TV episode via Tier 2 VidFast extractor.
func (t *ExtractorTier) ResolveTV(ctx context.Context, tmdbID, season, episode int) (*models.ResolutionResult, error) {
	targetURL := fmt.Sprintf("%s/tv/%d/%d/%d", t.vidfastBaseURL, tmdbID, season, episode)
	log.Printf("[tier2] extracting VidFast URL for TV %d S%dE%d: %s", tmdbID, season, episode, targetURL)

	streamURL, err := t.callExtractor(ctx, targetURL)
	if err != nil {
		log.Printf("[tier2] failed: %v", err)
		return nil, nil // Return nil to allow fallback to Tier 3
	}

	if streamURL == "" {
		log.Printf("[tier2] failed: extractor returned empty url")
		return nil, nil
	}

	log.Printf("[tier2] success resolving TV %d S%dE%d", tmdbID, season, episode)
	return &models.ResolutionResult{
		Success: true,
		TmdbID:  tmdbID,
		Type:    "tv",
		Tier:    2,
		Source:  "vidfast-extractor",
		Season:  &season,
		Episode: &episode,
		URL:     streamURL,
		Headers: map[string]string{
			"Referer": t.vidfastBaseURL + "/",
			"Origin":  t.vidfastBaseURL,
		},
	}, nil
}

// callExtractor contacts the extractor service with proper URL encoding.
func (t *ExtractorTier) callExtractor(ctx context.Context, targetURL string) (string, error) {
	parsedExtractor, err := url.Parse(t.extractorURL + "/extract")
	if err != nil {
		return "", fmt.Errorf("invalid extractor base url: %w", err)
	}

	params := url.Values{}
	params.Set("url", targetURL)
	parsedExtractor.RawQuery = params.Encode()
	fullURL := parsedExtractor.String()

	start := time.Now()
	log.Printf("[tier2] querying extractor API: %s (target: %s)", fullURL, targetURL)

	req, err := http.NewRequestWithContext(ctx, "GET", fullURL, nil)
	if err != nil {
		log.Printf("[tier2] failed creating request: %v", err)
		return "", fmt.Errorf("failed to create extractor request: %w", err)
	}
	req.Header.Set("User-Agent", defaultHeaders["User-Agent"])
	req.Header.Set("Accept", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		log.Printf("[tier2] extractor network error after %s: %v", time.Since(start), err)
		return "", fmt.Errorf("network error calling extractor: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[tier2] extractor returned status %d after %s", resp.StatusCode, time.Since(start))
		return "", fmt.Errorf("extractor http status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[tier2] error reading extractor body: %v", err)
		return "", fmt.Errorf("failed reading extractor response: %w", err)
	}

	var apiResp extractorAPIResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		log.Printf("[tier2] error parsing extractor JSON response: %v", err)
		return "", fmt.Errorf("failed parsing extractor json response: %w", err)
	}

	isSuccess := false
	if apiResp.Success != nil && *apiResp.Success {
		isSuccess = true
	} else if strings.EqualFold(apiResp.Status, "success") {
		isSuccess = true
	}

	if !isSuccess {
		errMsg := apiResp.Error
		if errMsg == "" {
			errMsg = apiResp.Message
		}
		if errMsg == "" {
			errMsg = "extractor returned success=false"
		}
		log.Printf("[tier2] extractor returned failure: %s (in %s)", errMsg, time.Since(start))
		return "", fmt.Errorf("%s", errMsg)
	}

	if apiResp.URL == nil || strings.TrimSpace(*apiResp.URL) == "" {
		log.Printf("[tier2] extractor returned empty url (in %s)", time.Since(start))
		return "", fmt.Errorf("extractor returned null or empty url")
	}

	resolvedURL := strings.TrimSpace(*apiResp.URL)
	log.Printf("[tier2] extractor successfully extracted stream URL in %s: %s", time.Since(start), resolvedURL)
	return resolvedURL, nil
}
