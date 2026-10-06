package resolver

import (
	"context"
	"log"

	"vidara-api/models"
)

// TierResolver defines the methods required for a tier to resolve movie and TV media.
type TierResolver interface {
	ResolveMovie(ctx context.Context, tmdbID int) (*models.ResolutionResult, error)
	ResolveTV(ctx context.Context, tmdbID, season, episode int) (*models.ResolutionResult, error)
}

// CentralResolver coordinates the fallback logic across Tier 1, Tier 2, and Tier 3.
type CentralResolver struct {
	tier1 TierResolver
	tier2 TierResolver
	tier3 TierResolver
}

// NewCentralResolver creates a new resolver orchestrator.
func NewCentralResolver(tier1, tier2, tier3 TierResolver) *CentralResolver {
	return &CentralResolver{
		tier1: tier1,
		tier2: tier2,
		tier3: tier3,
	}
}

// ResolveMovie resolves a movie TMDB ID across Tier 1 -> Tier 2 -> Tier 3 in strict order.
func (r *CentralResolver) ResolveMovie(ctx context.Context, tmdbID int) (*models.ResolutionResult, error) {
	log.Printf("[resolver] movie %d", tmdbID)

	// Tier 1 - Vidara
	if r.tier1 != nil {
		res, err := r.tier1.ResolveMovie(ctx, tmdbID)
		if err != nil {
			log.Printf("[resolver] tier1 error for movie %d: %v", tmdbID, err)
		} else if res != nil && res.Success {
			log.Printf("[resolver] resolved movie %d via Tier 1", tmdbID)
			return res, nil
		}
	}

	// Tier 2 - Extractor
	if r.tier2 != nil {
		res, err := r.tier2.ResolveMovie(ctx, tmdbID)
		if err != nil {
			log.Printf("[resolver] tier2 error for movie %d: %v", tmdbID, err)
		} else if res != nil && res.Success {
			log.Printf("[resolver] resolved movie %d via Tier 2", tmdbID)
			return res, nil
		}
	}

	// Tier 3 - CineTV
	if r.tier3 != nil {
		res, err := r.tier3.ResolveMovie(ctx, tmdbID)
		if err != nil {
			log.Printf("[resolver] tier3 error for movie %d: %v", tmdbID, err)
		} else if res != nil && res.Success {
			log.Printf("[resolver] resolved movie %d via Tier 3", tmdbID)
			return res, nil
		}
	}

	log.Printf("[resolver] movie %d not found in any tier", tmdbID)
	return &models.ResolutionResult{
		Success: false,
		TmdbID:  tmdbID,
		Error:   "source_not_found",
	}, nil
}

// ResolveTV resolves a TV show TMDB ID, season, and episode across Tier 1 -> Tier 2 -> Tier 3 in strict order.
func (r *CentralResolver) ResolveTV(ctx context.Context, tmdbID, season, episode int) (*models.ResolutionResult, error) {
	log.Printf("[resolver] TV %d S%dE%d", tmdbID, season, episode)

	// Tier 1 - Vidara
	if r.tier1 != nil {
		res, err := r.tier1.ResolveTV(ctx, tmdbID, season, episode)
		if err != nil {
			log.Printf("[resolver] tier1 error for TV %d S%dE%d: %v", tmdbID, season, episode, err)
		} else if res != nil && res.Success {
			log.Printf("[resolver] resolved TV %d S%dE%d via Tier 1", tmdbID, season, episode)
			return res, nil
		}
	}

	// Tier 2 - Extractor
	if r.tier2 != nil {
		res, err := r.tier2.ResolveTV(ctx, tmdbID, season, episode)
		if err != nil {
			log.Printf("[resolver] tier2 error for TV %d S%dE%d: %v", tmdbID, season, episode, err)
		} else if res != nil && res.Success {
			log.Printf("[resolver] resolved TV %d S%dE%d via Tier 2", tmdbID, season, episode)
			return res, nil
		}
	}

	// Tier 3 - CineTV
	if r.tier3 != nil {
		res, err := r.tier3.ResolveTV(ctx, tmdbID, season, episode)
		if err != nil {
			log.Printf("[resolver] tier3 error for TV %d S%dE%d: %v", tmdbID, season, episode, err)
		} else if res != nil && res.Success {
			log.Printf("[resolver] resolved TV %d S%dE%d via Tier 3", tmdbID, season, episode)
			return res, nil
		}
	}

	log.Printf("[resolver] TV %d S%dE%d not found in any tier", tmdbID, season, episode)
	return &models.ResolutionResult{
		Success: false,
		TmdbID:  tmdbID,
		Season:  &season,
		Episode: &episode,
		Error:   "source_not_found",
	}, nil
}
