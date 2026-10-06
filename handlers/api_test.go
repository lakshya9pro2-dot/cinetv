package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"vidara-api/models"
	"vidara-api/resolver"
	"vidara-api/tiers"
)

type mockTier struct {
	movieFn func(ctx context.Context, id int) (*models.ResolutionResult, error)
	tvFn    func(ctx context.Context, id, s, e int) (*models.ResolutionResult, error)
}

func (m *mockTier) ResolveMovie(ctx context.Context, id int) (*models.ResolutionResult, error) {
	if m.movieFn != nil {
		return m.movieFn(ctx, id)
	}
	return nil, nil
}

func (m *mockTier) ResolveTV(ctx context.Context, id, s, e int) (*models.ResolutionResult, error) {
	if m.tvFn != nil {
		return m.tvFn(ctx, id, s, e)
	}
	return nil, nil
}

func setupTestServer() (*httptest.Server, *tiers.VidaraTier) {
	t1 := tiers.NewVidaraTier("https://vidara.to", "https://streamtape.com", 2*time.Second)
	_ = t1.LoadData("data/vidara.json", "data.json", "../data/vidara.json", "../data.json")

	// Mock Tier 2 that resolves movie 126560 and tv 444/3/2
	t2 := &mockTier{
		movieFn: func(ctx context.Context, id int) (*models.ResolutionResult, error) {
			if id == 126560 {
				return &models.ResolutionResult{
					Success: true,
					TmdbID:  id,
					Type:    "movie",
					Tier:    2,
					Source:  "vidfast-extractor",
					URL:     "https://stream.vidfast.vc/126560.mp4",
				}, nil
			}
			return nil, nil
		},
		tvFn: func(ctx context.Context, id, s, e int) (*models.ResolutionResult, error) {
			if id == 444 && s == 3 && e == 2 {
				return &models.ResolutionResult{
					Success: true,
					TmdbID:  id,
					Type:    "tv",
					Tier:    2,
					Source:  "vidfast-extractor",
					Season:  &s,
					Episode: &e,
					URL:     "https://stream.vidfast.vc/tv444s3e2.mp4",
				}, nil
			}
			return nil, nil
		},
	}

	// Mock Tier 3 that resolves movie 550 and tv 1399/1/6
	t3 := &mockTier{
		movieFn: func(ctx context.Context, id int) (*models.ResolutionResult, error) {
			if id == 550 {
				return &models.ResolutionResult{
					Success: true,
					TmdbID:  id,
					Type:    "movie",
					Tier:    3,
					Source:  "cinetv",
					Title:   "Fight Club",
					URL:     "http://cinetv.filmin/550.mp4",
				}, nil
			}
			return nil, nil
		},
		tvFn: func(ctx context.Context, id, s, e int) (*models.ResolutionResult, error) {
			if id == 1399 && s == 1 && e == 6 {
				return &models.ResolutionResult{
					Success: true,
					TmdbID:  id,
					Type:    "tv",
					Tier:    3,
					Source:  "cinetv",
					Title:   "Game of Thrones",
					Season:  &s,
					Episode: &e,
					URL:     "http://cinetv.filmin/got_s1e6.mp4",
				}, nil
			}
			return nil, nil
		},
	}

	r := resolver.NewCentralResolver(t1, t2, t3)
	h := NewAPIHandler(r, t1, 2*time.Second)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", h.HandleHealth)
	mux.HandleFunc("/api/movie/", h.HandleMovie)
	mux.HandleFunc("/api/tv/", h.HandleTV)

	return httptest.NewServer(mux), t1
}

func TestHealthEndpoint(t *testing.T) {
	ts, _ := setupTestServer()
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	var data map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&data)
	if data["status"] != "ok" {
		t.Errorf("expected status 'ok', got '%v'", data["status"])
	}
}

func TestMovieEndpoints_AllTiers(t *testing.T) {
	ts, _ := setupTestServer()
	defer ts.Close()

	// 1. Movie Tier 1: 1167307
	resp1, err := http.Get(ts.URL + "/api/movie/1167307")
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	defer resp1.Body.Close()
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp1.StatusCode)
	}
	var res1 models.ResolutionResult
	_ = json.NewDecoder(resp1.Body).Decode(&res1)
	if !res1.Success || res1.Tier != 1 || res1.Source != "vidara" || res1.TmdbID != 1167307 {
		t.Errorf("expected Tier 1 Vidara for 1167307, got %+v", res1)
	}

	// 2. Movie Tier 2 fallback: 126560
	resp2, err := http.Get(ts.URL + "/api/movie/126560")
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp2.StatusCode)
	}
	var res2 models.ResolutionResult
	_ = json.NewDecoder(resp2.Body).Decode(&res2)
	if !res2.Success || res2.Tier != 2 || res2.Source != "vidfast-extractor" {
		t.Errorf("expected Tier 2 for 126560, got %+v", res2)
	}

	// 3. Movie Tier 3 fallback: 550
	resp3, err := http.Get(ts.URL + "/api/movie/550")
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp3.StatusCode)
	}
	var res3 models.ResolutionResult
	_ = json.NewDecoder(resp3.Body).Decode(&res3)
	if !res3.Success || res3.Tier != 3 || res3.Source != "cinetv" {
		t.Errorf("expected Tier 3 for 550, got %+v", res3)
	}

	// 4. Movie Complete Failure: 9999999
	resp4, err := http.Get(ts.URL + "/api/movie/9999999")
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	defer resp4.Body.Close()
	if resp4.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 NotFound, got %d", resp4.StatusCode)
	}
	var res4 models.ResolutionResult
	_ = json.NewDecoder(resp4.Body).Decode(&res4)
	if res4.Success || res4.Error != "source_not_found" {
		t.Errorf("expected clean 404 failure structure, got %+v", res4)
	}

	// 5. Validation failures: 0, negative, string
	for _, badID := range []string{"0", "-1", "abc"} {
		respBad, _ := http.Get(ts.URL + "/api/movie/" + badID)
		if respBad.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request for id %s, got %d", badID, respBad.StatusCode)
		}
	}
}

func TestTVEndpoints_AllTiers(t *testing.T) {
	ts, _ := setupTestServer()
	defer ts.Close()

	// 1. TV Tier 1: 262838 Season 1 Episode 2
	resp1, err := http.Get(ts.URL + "/api/tv/262838/1/2")
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	defer resp1.Body.Close()
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp1.StatusCode)
	}
	var res1 models.ResolutionResult
	_ = json.NewDecoder(resp1.Body).Decode(&res1)
	if !res1.Success || res1.Tier != 1 || *res1.Season != 1 || *res1.Episode != 2 {
		t.Errorf("expected Tier 1 TV S1E2, got %+v", res1)
	}

	// 2. TV Tier 2 fallback: 444 Season 3 Episode 2
	resp2, err := http.Get(ts.URL + "/api/tv/444/3/2")
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp2.StatusCode)
	}
	var res2 models.ResolutionResult
	_ = json.NewDecoder(resp2.Body).Decode(&res2)
	if !res2.Success || res2.Tier != 2 || *res2.Season != 3 || *res2.Episode != 2 {
		t.Errorf("expected Tier 2 TV S3E2, got %+v", res2)
	}

	// 3. TV Tier 3 fallback: 1399 Season 1 Episode 6
	resp3, err := http.Get(ts.URL + "/api/tv/1399/1/6")
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp3.StatusCode)
	}
	var res3 models.ResolutionResult
	_ = json.NewDecoder(resp3.Body).Decode(&res3)
	if !res3.Success || res3.Tier != 3 || *res3.Season != 1 || *res3.Episode != 6 {
		t.Errorf("expected Tier 3 TV S1E6, got %+v", res3)
	}

	// 4. TV Complete Failure: 999 Season 1 Episode 1
	resp4, err := http.Get(ts.URL + "/api/tv/999/1/1")
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	defer resp4.Body.Close()
	if resp4.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp4.StatusCode)
	}
	var res4 models.ResolutionResult
	_ = json.NewDecoder(resp4.Body).Decode(&res4)
	if res4.Success || res4.Error != "source_not_found" {
		t.Errorf("expected source_not_found, got %+v", res4)
	}

	// 5. TV Validation: invalid route or values
	badRoutes := []string{
		"/api/tv/444/0/2",
		"/api/tv/444/1/0",
		"/api/tv/0/1/1",
		"/api/tv/444/1",
		"/api/tv/abc/1/1",
	}
	for _, br := range badRoutes {
		respBad, _ := http.Get(ts.URL + br)
		if respBad.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 for bad route %s, got %d", br, respBad.StatusCode)
		}
	}
}
