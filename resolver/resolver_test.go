package resolver

import (
	"context"
	"fmt"
	"testing"

	"vidara-api/models"
)

type mockTier struct {
	name            string
	movieHandler    func(ctx context.Context, tmdbID int) (*models.ResolutionResult, error)
	tvHandler       func(ctx context.Context, tmdbID, season, episode int) (*models.ResolutionResult, error)
	movieCallCount  int
	tvCallCount     int
}

func (m *mockTier) ResolveMovie(ctx context.Context, tmdbID int) (*models.ResolutionResult, error) {
	m.movieCallCount++
	if m.movieHandler != nil {
		return m.movieHandler(ctx, tmdbID)
	}
	return nil, nil
}

func (m *mockTier) ResolveTV(ctx context.Context, tmdbID, season, episode int) (*models.ResolutionResult, error) {
	m.tvCallCount++
	if m.tvHandler != nil {
		return m.tvHandler(ctx, tmdbID, season, episode)
	}
	return nil, nil
}

// Test Fallback Order for Movie:
// 1. Tier 1 succeeds -> Tier 2 is NOT called -> Tier 3 is NOT called
func TestMovieFallback_Tier1Succeeds(t *testing.T) {
	t1 := &mockTier{
		movieHandler: func(ctx context.Context, tmdbID int) (*models.ResolutionResult, error) {
			return &models.ResolutionResult{Success: true, TmdbID: tmdbID, Tier: 1, Source: "vidara"}, nil
		},
	}
	t2 := &mockTier{}
	t3 := &mockTier{}

	r := NewCentralResolver(t1, t2, t3)
	res, err := r.ResolveMovie(context.Background(), 1167307)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !res.Success || res.Tier != 1 || res.Source != "vidara" {
		t.Fatalf("expected tier 1 success, got %+v", res)
	}
	if t1.movieCallCount != 1 {
		t.Errorf("expected t1 call count 1, got %d", t1.movieCallCount)
	}
	if t2.movieCallCount != 0 {
		t.Errorf("Tier 2 MUST NOT be called when Tier 1 succeeds, got %d calls", t2.movieCallCount)
	}
	if t3.movieCallCount != 0 {
		t.Errorf("Tier 3 MUST NOT be called when Tier 1 succeeds, got %d calls", t3.movieCallCount)
	}
}

// 2. Tier 1 fails -> Tier 2 succeeds -> Tier 3 is NOT called
func TestMovieFallback_Tier1Fails_Tier2Succeeds(t *testing.T) {
	t1 := &mockTier{
		movieHandler: func(ctx context.Context, tmdbID int) (*models.ResolutionResult, error) {
			return nil, nil // not found
		},
	}
	t2 := &mockTier{
		movieHandler: func(ctx context.Context, tmdbID int) (*models.ResolutionResult, error) {
			return &models.ResolutionResult{Success: true, TmdbID: tmdbID, Tier: 2, Source: "vidfast-extractor", URL: "http://stream.mp4"}, nil
		},
	}
	t3 := &mockTier{}

	r := NewCentralResolver(t1, t2, t3)
	res, err := r.ResolveMovie(context.Background(), 126560)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !res.Success || res.Tier != 2 || res.Source != "vidfast-extractor" {
		t.Fatalf("expected tier 2 success, got %+v", res)
	}
	if t1.movieCallCount != 1 {
		t.Errorf("expected t1 call count 1, got %d", t1.movieCallCount)
	}
	if t2.movieCallCount != 1 {
		t.Errorf("expected t2 call count 1, got %d", t2.movieCallCount)
	}
	if t3.movieCallCount != 0 {
		t.Errorf("Tier 3 MUST NOT be called when Tier 2 succeeds, got %d calls", t3.movieCallCount)
	}
}

// 3. Tier 1 fails -> Tier 2 fails -> Tier 3 succeeds
func TestMovieFallback_Tier1Fails_Tier2Fails_Tier3Succeeds(t *testing.T) {
	t1 := &mockTier{
		movieHandler: func(ctx context.Context, tmdbID int) (*models.ResolutionResult, error) {
			return nil, nil
		},
	}
	t2 := &mockTier{
		movieHandler: func(ctx context.Context, tmdbID int) (*models.ResolutionResult, error) {
			return nil, fmt.Errorf("extractor offline")
		},
	}
	t3 := &mockTier{
		movieHandler: func(ctx context.Context, tmdbID int) (*models.ResolutionResult, error) {
			return &models.ResolutionResult{Success: true, TmdbID: tmdbID, Tier: 3, Source: "cinetv", URL: "http://cinetv.mp4"}, nil
		},
	}

	r := NewCentralResolver(t1, t2, t3)
	res, err := r.ResolveMovie(context.Background(), 550)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !res.Success || res.Tier != 3 || res.Source != "cinetv" {
		t.Fatalf("expected tier 3 success, got %+v", res)
	}
	if t1.movieCallCount != 1 {
		t.Errorf("expected t1 call count 1, got %d", t1.movieCallCount)
	}
	if t2.movieCallCount != 1 {
		t.Errorf("expected t2 call count 1, got %d", t2.movieCallCount)
	}
	if t3.movieCallCount != 1 {
		t.Errorf("expected t3 call count 1, got %d", t3.movieCallCount)
	}
}

// 4. Tier 1 fails -> Tier 2 fails -> Tier 3 fails -> returns 404 structure
func TestMovieFallback_AllFail(t *testing.T) {
	t1 := &mockTier{movieHandler: func(ctx context.Context, tmdbID int) (*models.ResolutionResult, error) { return nil, nil }}
	t2 := &mockTier{movieHandler: func(ctx context.Context, tmdbID int) (*models.ResolutionResult, error) { return nil, nil }}
	t3 := &mockTier{movieHandler: func(ctx context.Context, tmdbID int) (*models.ResolutionResult, error) { return nil, nil }}

	r := NewCentralResolver(t1, t2, t3)
	res, err := r.ResolveMovie(context.Background(), 9999999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Success || res.Error != "source_not_found" || res.TmdbID != 9999999 {
		t.Fatalf("expected source_not_found failure result, got %+v", res)
	}
	if t1.movieCallCount != 1 || t2.movieCallCount != 1 || t3.movieCallCount != 1 {
		t.Errorf("all tiers should have been attempted exactly once")
	}
}

// TV Fallback Order Tests:
func TestTVFallback_Order(t *testing.T) {
	// Case 1: Tier 1 succeeds
	t1 := &mockTier{
		tvHandler: func(ctx context.Context, tmdbID, season, episode int) (*models.ResolutionResult, error) {
			return &models.ResolutionResult{Success: true, TmdbID: tmdbID, Season: &season, Episode: &episode, Tier: 1, Source: "vidara"}, nil
		},
	}
	t2 := &mockTier{}
	t3 := &mockTier{}

	r := NewCentralResolver(t1, t2, t3)
	res, _ := r.ResolveTV(context.Background(), 262838, 1, 1)
	if !res.Success || res.Tier != 1 {
		t.Fatalf("expected Tier 1 TV resolution")
	}
	if t2.tvCallCount != 0 || t3.tvCallCount != 0 {
		t.Errorf("Tiers 2 and 3 should not be called when Tier 1 succeeds")
	}

	// Case 2: Tier 1 fails, Tier 2 succeeds
	t1Fail := &mockTier{}
	t2Success := &mockTier{
		tvHandler: func(ctx context.Context, tmdbID, season, episode int) (*models.ResolutionResult, error) {
			return &models.ResolutionResult{Success: true, TmdbID: tmdbID, Season: &season, Episode: &episode, Tier: 2, Source: "vidfast-extractor"}, nil
		},
	}
	t3Uncalled := &mockTier{}

	r2 := NewCentralResolver(t1Fail, t2Success, t3Uncalled)
	res2, _ := r2.ResolveTV(context.Background(), 444, 3, 2)
	if !res2.Success || res2.Tier != 2 {
		t.Fatalf("expected Tier 2 TV resolution")
	}
	if t3Uncalled.tvCallCount != 0 {
		t.Errorf("Tier 3 should not be called when Tier 2 succeeds")
	}

	// Case 3: Tier 1 fails, Tier 2 fails, Tier 3 succeeds
	t3Success := &mockTier{
		tvHandler: func(ctx context.Context, tmdbID, season, episode int) (*models.ResolutionResult, error) {
			return &models.ResolutionResult{Success: true, TmdbID: tmdbID, Season: &season, Episode: &episode, Tier: 3, Source: "cinetv"}, nil
		},
	}
	r3 := NewCentralResolver(t1Fail, t1Fail, t3Success)
	res3, _ := r3.ResolveTV(context.Background(), 1399, 1, 6)
	if !res3.Success || res3.Tier != 3 {
		t.Fatalf("expected Tier 3 TV resolution")
	}

	// Case 4: All fail
	r4 := NewCentralResolver(t1Fail, t1Fail, t1Fail)
	res4, _ := r4.ResolveTV(context.Background(), 999, 1, 1)
	if res4.Success || res4.Error != "source_not_found" {
		t.Fatalf("expected failure for nonexistent TV")
	}
}
