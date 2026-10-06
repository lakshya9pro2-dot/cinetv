package tiers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestExtractorTier_ResolveMovie_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/extract" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		target := r.URL.Query().Get("url")
		if target != "https://vidfast.vc/movie/126560" {
			t.Errorf("expected url query param 'https://vidfast.vc/movie/126560', got '%s'", target)
		}
		timeoutParam := r.URL.Query().Get("timeout")
		if timeoutParam != "20" {
			t.Errorf("expected timeout query param '20', got '%s'", timeoutParam)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success": true, "url": "https://stream.vidfast.vc/126560.mp4"}`))
	}))
	defer ts.Close()

	tier := NewExtractorTier(ts.URL, "https://vidfast.vc", 2*time.Second)
	res, err := tier.ResolveMovie(context.Background(), 126560)
	if err != nil || res == nil {
		t.Fatalf("expected resolution, got err: %v, res: %+v", err, res)
	}

	if !res.Success || res.Tier != 2 || res.Source != "vidfast-extractor" || res.URL != "https://stream.vidfast.vc/126560.mp4" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestExtractorTier_ResolveTV_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target := r.URL.Query().Get("url")
		if target != "https://vidfast.vc/tv/444/3/2" {
			t.Errorf("expected url query param 'https://vidfast.vc/tv/444/3/2', got '%s'", target)
		}
		timeoutParam := r.URL.Query().Get("timeout")
		if timeoutParam != "20" {
			t.Errorf("expected timeout query param '20', got '%s'", timeoutParam)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success": true, "url": "https://stream.vidfast.vc/tv444s3e2.mp4"}`))
	}))
	defer ts.Close()

	tier := NewExtractorTier(ts.URL, "https://vidfast.vc", 2*time.Second)
	res, err := tier.ResolveTV(context.Background(), 444, 3, 2)
	if err != nil || res == nil {
		t.Fatalf("expected TV resolution, got err: %v, res: %+v", err, res)
	}

	if !res.Success || res.Tier != 2 || *res.Season != 3 || *res.Episode != 2 {
		t.Fatalf("unexpected TV result: %+v", res)
	}
}

func TestExtractorTier_FailureHandling(t *testing.T) {
	// Case 1: success: false
	tsFalse := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"success": false, "url": null}`))
	}))
	defer tsFalse.Close()

	tier1 := NewExtractorTier(tsFalse.URL, "https://vidfast.vc", 2*time.Second)
	res1, err1 := tier1.ResolveMovie(context.Background(), 999)
	if err1 != nil {
		t.Errorf("error should be nil so it cascades to Tier 3, got: %v", err1)
	}
	if res1 != nil {
		t.Errorf("expected nil result on extractor failure, got %+v", res1)
	}

	// Case 2: 500 error
	ts500 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer ts500.Close()

	tier2 := NewExtractorTier(ts500.URL, "https://vidfast.vc", 2*time.Second)
	res2, err2 := tier2.ResolveMovie(context.Background(), 999)
	if err2 != nil {
		t.Errorf("error should be nil so it cascades to Tier 3, got: %v", err2)
	}
	if res2 != nil {
		t.Errorf("expected nil result on 500 status, got %+v", res2)
	}
}
