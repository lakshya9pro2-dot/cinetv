package tiers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestVidaraTier_LoadDataAndLookup(t *testing.T) {
	tier := NewVidaraTier("https://vidara.to", "https://streamtape.com", 5*time.Second)

	// Test loading data from data.json or data/vidara.json
	err := tier.LoadData("data/vidara.json", "data.json", "../data/vidara.json", "../data.json")
	if err != nil {
		t.Fatalf("failed to load dataset: %v", err)
	}

	// 1. Movie lookup: TMDB ID 1167307
	movie, found := tier.FindMovie(1167307)
	if !found {
		t.Fatalf("movie 1167307 should be found in local dataset")
	}
	if movie.Play.VA != "mzfdFOguU87x" || movie.Play.ST != "6a44824dcd175" {
		t.Errorf("unexpected movie playback data: %+v", movie.Play)
	}

	// 2. Movie not in dataset: 550
	_, found550 := tier.FindMovie(550)
	if found550 {
		t.Errorf("movie 550 should NOT be found in Vidara Tier 1 dataset")
	}

	// 3. TV show lookup: TMDB ID 262838 Season 1 Episode 1
	tvEp1, foundEp1 := tier.FindTV(262838, 1, 1)
	if !foundEp1 {
		t.Fatalf("TV 262838 S1E1 should be found")
	}
	if tvEp1.Season != 1 || tvEp1.Episode != 1 {
		t.Errorf("expected S1E1, got S%dE%d", tvEp1.Season, tvEp1.Episode)
	}

	// 4. TV show lookup: Season 2 Episode 2
	tvEp2, foundEp2 := tier.FindTV(262838, 2, 2)
	if !foundEp2 {
		t.Fatalf("TV 262838 S2E2 should be found")
	}
	if tvEp2.Season != 2 || tvEp2.Episode != 2 {
		t.Errorf("expected S2E2, got S%dE%d", tvEp2.Season, tvEp2.Episode)
	}

	// 5. Nonexistent episode: Season 1 Episode 99
	_, foundNonexistent := tier.FindTV(262838, 1, 99)
	if foundNonexistent {
		t.Errorf("TV 262838 S1E99 should NOT be found")
	}

	// 6. Nonexistent season: Season 9 Episode 1
	_, foundNonexistentS := tier.FindTV(262838, 9, 1)
	if foundNonexistentS {
		t.Errorf("TV 262838 S9E1 should NOT be found")
	}
}

func TestVidaraTier_ResolveMovieAndTV(t *testing.T) {
	tier := NewVidaraTier("https://vidara.to", "https://streamtape.com", 2*time.Second)
	_ = tier.LoadData("data/vidara.json", "data.json", "../data/vidara.json", "../data.json")

	// Movie resolution
	resM, err := tier.ResolveMovie(context.Background(), 1167307)
	if err != nil || resM == nil || !resM.Success {
		t.Fatalf("expected successful movie resolution, got %+v (err: %v)", resM, err)
	}
	if resM.Tier != 1 || resM.Source != "vidara" || resM.Play == nil || resM.Play.VA == "" {
		t.Errorf("invalid Tier 1 movie resolution response: %+v", resM)
	}

	// TV resolution
	resTV, err := tier.ResolveTV(context.Background(), 262838, 1, 2)
	if err != nil || resTV == nil || !resTV.Success {
		t.Fatalf("expected successful TV resolution, got %+v (err: %v)", resTV, err)
	}
	if resTV.Tier != 1 || resTV.Source != "vidara" || *resTV.Season != 1 || *resTV.Episode != 2 {
		t.Errorf("invalid Tier 1 TV resolution response: %+v", resTV)
	}
}

func TestVidaraTier_ParseMalformedDataset(t *testing.T) {
	// Directly test parsing the raw malformed data.json
	tier := NewVidaraTier("https://vidara.to", "https://streamtape.com", 2*time.Second)
	data, err := os.ReadFile("data.json")
	if err != nil {
		data, err = os.ReadFile("../data.json")
	}
	if err != nil {
		t.Skip("data.json not found in test dir")
	}

	items, err := tier.parseDataset(data)
	if err != nil {
		t.Fatalf("parseDataset failed on raw data.json: %v", err)
	}
	if len(items) == 0 {
		t.Fatalf("expected parsed items from data.json, got 0")
	}
}

func TestCleanIDAndParsing(t *testing.T) {
	if CleanID("https://streamtape.com/v/0A2vDYQz3wIbPQ6/") != "0A2vDYQz3wIbPQ6" {
		t.Errorf("CleanID failed for full url")
	}
	if CleanID("(id)d932127894f1%28id%29") != "d932127894f1" {
		t.Errorf("CleanID failed for id with decorators")
	}

	st, va := ParseIDsFromString("https://example.com/play?st=0A2vDYQz3wIbPQ6&va=d932127894f1")
	if st != "0A2vDYQz3wIbPQ6" || va != "d932127894f1" {
		t.Errorf("ParseIDsFromString query param failed, got st=%s, va=%s", st, va)
	}

	st2, va2 := ParseIDsFromString("st/0A2vDYQz3wIbPQ6/va/d932127894f1")
	if st2 != "0A2vDYQz3wIbPQ6" || va2 != "d932127894f1" {
		t.Errorf("ParseIDsFromString path failed, got st=%s, va=%s", st2, va2)
	}
}

func TestEvalStreamTapeJS(t *testing.T) {
	expr := `'hello'.substring(0, 4) + 'world'.substring(1)`
	res := EvalStreamTapeJS(context.Background(), expr)
	if res != "hellorld" {
		t.Errorf("expected 'hellorld', got '%s'", res)
	}
}

func TestVidaraTier_LoadFromURL(t *testing.T) {
	sampleJSON := `[
		{"id": 12345, "title": "Test Movie", "type": "movie", "play": {"va": "testva", "st": "testst"}}
	]`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(sampleJSON))
	}))
	defer ts.Close()

	tier := NewVidaraTier("https://vidara.to", "https://streamtape.com", 2*time.Second)
	err := tier.LoadFromURL(ts.URL)
	if err != nil {
		t.Fatalf("LoadFromURL failed: %v", err)
	}

	movie, found := tier.FindMovie(12345)
	if !found || movie.Title != "Test Movie" || movie.Play.VA != "testva" {
		t.Fatalf("expected movie 12345 to be loaded from URL, got: %+v", movie)
	}
}
