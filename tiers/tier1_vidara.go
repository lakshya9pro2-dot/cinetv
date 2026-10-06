package tiers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"vidara-api/models"
)

var (
	streamtapeDomains = map[string]bool{
		"streamtape.com":     true,
		"watchadsontape.com": true,
		"streamtape.net":     true,
		"streamtape.xyz":     true,
		"shavetape.cash":     true,
	}

	vidaraDomains = map[string]bool{
		"vidara.to":           true,
		"vidavaca.net":        true,
		"vidaarax.net":        true,
		"vidaarax.com":        true,
		"vidaratem.com":       true,
		"vidaraw.com":         true,
		"vidarax.cc":          true,
		"vidaraa.cc":          true,
		"vidara.so":           true,
		"odysseusa.cc":        true,
		"handfacesnap.cc":     true,
		"namefacesnap.cc":     true,
		"thebesthosterv.com":  true,
		"thebesthostertv.com": true,
		"vidmatrixa.com":      true,
		"vidchampions.com":    true,
		"antarcticadocs.com":  true,
		"nameitweb.com":       true,
	}

	defaultHeaders = map[string]string{
		"User-Agent":      "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
		"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		"Accept-Language": "en-US,en;q=0.5",
	}

	reCleanID       = regexp.MustCompile(`[^a-zA-Z0-9_\-]`)
	reSTParam       = regexp.MustCompile(`(?i)(?:st\?play=|\bst/|/st/|[?&]st=|[?&]play=)([^/&?#\s]+)`)
	reSTFull        = regexp.MustCompile(`(?i)streamtape\.com/v/([a-zA-Z0-9_\-]+)`)
	reVAParam       = regexp.MustCompile(`(?i)(?:va\?playd=|\bva/|/va/|[?&]va=|[?&]playd=)([^/&?#\s]+)`)
	reVAFull        = regexp.MustCompile(`(?i)vidara\.to/v/([a-zA-Z0-9_\-]+)`)
	reTitle         = regexp.MustCompile(`(?i)<title>(.*?)</title>`)
	reBotlink       = regexp.MustCompile(`(?i)botlink['"]\)\.innerHTML\s*=\s*([^;]+);`)
	reRobotlink     = regexp.MustCompile(`(?i)getElementById\(['"](?:no)?robotlink['"]\)\.innerHTML\s*=\s*([^;]+);`)
	reSubToken      = regexp.MustCompile(`['"]([^'"]*)['"]((?:\.substring\(\s*\d+\s*(?:,\s*\d+\s*)?\))*)`)
	reSubCall       = regexp.MustCompile(`\.substring\((\d+)(?:,\s*(\d+))?\)`)
	reExtMedia      = regexp.MustCompile(`(?i)#EXT-X-MEDIA:.*TYPE=AUDIO`)
	reNameAttr      = regexp.MustCompile(`(?i)NAME="([^"]+)"`)
	reLangAttr      = regexp.MustCompile(`(?i)LANGUAGE="([^"]+)"`)
	reURIAttr       = regexp.MustCompile(`(?i)URI="([^"]+)"`)
	reDefAttr       = regexp.MustCompile(`(?i)DEFAULT=(YES|NO)`)
	reAlphaNumScore = regexp.MustCompile(`[^a-zA-Z0-9_\-]`)
)

// MovieRecord represents an indexed movie in memory.
type MovieRecord struct {
	ID      int
	Title   string
	Quality string
	Play    models.Playback
}

// TVEpisodeRecord represents an indexed TV episode in memory.
type TVEpisodeRecord struct {
	ID      int
	Title   string
	Quality string
	Season  int
	Episode int
	Play    models.Playback
}

// VidaraTier represents Tier 1 implementation with in-memory indexes and extractors.
type VidaraTier struct {
	mu           sync.RWMutex
	movieIndex   map[int]MovieRecord
	tvIndex      map[int]map[int]map[int]TVEpisodeRecord // tmdbID -> season -> episode -> record
	vidaraBase   string
	streamBase   string
	client       *http.Client
	noRedirClient *http.Client
}

// NewVidaraTier creates a new Tier 1 resolver.
func NewVidaraTier(vidaraBase, streamtapeBase string, timeout time.Duration) *VidaraTier {
	if vidaraBase == "" {
		vidaraBase = "https://vidara.to"
	}
	if streamtapeBase == "" {
		streamtapeBase = "https://streamtape.com"
	}

	return &VidaraTier{
		movieIndex:   make(map[int]MovieRecord),
		tvIndex:      make(map[int]map[int]map[int]TVEpisodeRecord),
		vidaraBase:   strings.TrimRight(vidaraBase, "/"),
		streamBase:   strings.TrimRight(streamtapeBase, "/"),
		client:       &http.Client{Timeout: timeout},
		noRedirClient: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// CleanID cleans an ID from any URL or decoration.
func CleanID(val string) string {
	if val == "" {
		return ""
	}
	val = strings.TrimSpace(val)
	val = strings.ReplaceAll(val, "(id)", "")
	val = strings.ReplaceAll(val, "%28id%29", "")
	if strings.Contains(val, "://") {
		u, err := url.Parse(val)
		if err == nil {
			val = strings.TrimRight(u.Path, "/")
			parts := strings.Split(val, "/")
			val = parts[len(parts)-1]
		}
	}
	return reCleanID.ReplaceAllString(val, "")
}

// ParseIDsFromString extracts st_id and va_id from any string format.
func ParseIDsFromString(s string) (stID, vaID string) {
	decoded, err := url.QueryUnescape(s)
	if err == nil {
		s = decoded
	}

	if m := reSTParam.FindStringSubmatch(s); len(m) > 1 {
		stID = CleanID(m[1])
	} else if m := reSTFull.FindStringSubmatch(s); len(m) > 1 {
		stID = CleanID(m[1])
	}

	if m := reVAParam.FindStringSubmatch(s); len(m) > 1 {
		vaID = CleanID(m[1])
	} else if m := reVAFull.FindStringSubmatch(s); len(m) > 1 {
		vaID = CleanID(m[1])
	}

	return stID, vaID
}

// LoadData loads and indexes the Vidara dataset from disk or remote URL.
func (v *VidaraTier) LoadData(sources ...string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	var dataBytes []byte
	var err error

	for _, s := range sources {
		if s == "" {
			continue
		}

		if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
			req, reqErr := http.NewRequest("GET", s, nil)
			if reqErr == nil {
				req.Header.Set("User-Agent", defaultHeaders["User-Agent"])
				req.Header.Set("Accept", "application/json")
				resp, doErr := v.client.Do(req)
				if doErr == nil {
					if resp.StatusCode == http.StatusOK {
						dataBytes, err = io.ReadAll(resp.Body)
						resp.Body.Close()
						if err == nil && len(dataBytes) > 0 {
							log.Printf("[tier1] successfully fetched dataset from URL %s (%d bytes)", s, len(dataBytes))
							break
						}
					} else {
						resp.Body.Close()
						log.Printf("[tier1] URL %s returned HTTP status %d", s, resp.StatusCode)
					}
				} else {
					log.Printf("[tier1] network error fetching dataset from %s: %v", s, doErr)
				}
			}
			continue
		}

		dataBytes, err = os.ReadFile(s)
		if err == nil && len(dataBytes) > 0 {
			log.Printf("[tier1] successfully read dataset from %s (%d bytes)", s, len(dataBytes))
			break
		}
	}

	if len(dataBytes) == 0 {
		return fmt.Errorf("no dataset file or URL found in sources: %v", sources)
	}

	items, parseErr := v.parseDataset(dataBytes)
	if parseErr != nil {
		return fmt.Errorf("failed to parse dataset: %w", parseErr)
	}

	countMovies := 0
	countTVEpisodes := 0

	for _, item := range items {
		if strings.EqualFold(item.Type, "movie") {
			play := models.Playback{}
			if item.Play != nil {
				play = *item.Play
			}
			if play.VA == "" && item.VA != "" {
				play.VA = item.VA
			}
			if play.ST == "" && item.ST != "" {
				play.ST = item.ST
			}

			v.movieIndex[item.ID] = MovieRecord{
				ID:      item.ID,
				Title:   item.Title,
				Quality: item.Quality,
				Play:    play,
			}
			countMovies++
		} else if strings.EqualFold(item.Type, "tv") {
			if _, ok := v.tvIndex[item.ID]; !ok {
				v.tvIndex[item.ID] = make(map[int]map[int]TVEpisodeRecord)
			}
			if _, ok := v.tvIndex[item.ID][item.Season]; !ok {
				v.tvIndex[item.ID][item.Season] = make(map[int]TVEpisodeRecord)
			}

			for _, ep := range item.Episodes {
				v.tvIndex[item.ID][item.Season][ep.Episode] = TVEpisodeRecord{
					ID:      item.ID,
					Title:   item.Title,
					Quality: item.Quality,
					Season:  item.Season,
					Episode: ep.Episode,
					Play:    ep.Play,
				}
				countTVEpisodes++
			}
		}
	}

	log.Printf("[tier1] indexed %d movies and %d TV episodes", countMovies, countTVEpisodes)
	return nil
}

// LoadFromURL fetches and indexes the Vidara dataset directly from a remote URL.
func (v *VidaraTier) LoadFromURL(dataURL string) error {
	return v.LoadData(dataURL)
}

// parseDataset parses either standard JSON or repairs malformed structures internally.
func (v *VidaraTier) parseDataset(raw []byte) ([]models.MediaItem, error) {
	// First attempt direct standard unmarshal
	var items []models.MediaItem
	if err := json.Unmarshal(raw, &items); err == nil && len(items) > 0 {
		return items, nil
	}

	// Internal normalizer for malformed JSON
	text := string(raw)

	// Fix `"play": "va": "...", "st": "..."` -> `"play": {"va": "...", "st": "..."}`
	rePlayMalformed := regexp.MustCompile(`"play"\s*:\s*"va"\s*:\s*"([^"]+)"\s*,\s*"st"\s*:\s*"([^"]+)"(?:"")?`)
	text = rePlayMalformed.ReplaceAllString(text, `"play": {"va": "$1", "st": "$2"}`)

	// Fix trailing double quotes on strings like "6a44824dcd175""
	text = regexp.MustCompile(`"([a-zA-Z0-9_\-]+)""`).ReplaceAllString(text, `"$1"`)

	// Handle malformed TV objects where "season": 2 is chained inside the same object
	// e.g. "episodes": [ ... ], \n "season": 2, \n "episodes": [ ... ]
	if strings.Contains(text, `"season": 2`) && strings.Contains(text, `"India's Got Latent"`) {
		// Clean trailing commas before brackets
		text = regexp.MustCompile(`,\s*([\]}])`).ReplaceAllString(text, "$1")
		// Split multiple seasons if wrapped in one object
		reMultiSeason := regexp.MustCompile(`(?s)\{\s*"id":\s*(\d+),\s*"title":\s*"([^"]+)",\s*"type":\s*"tv",\s*"quality":\s*"([^"]+)",\s*"season":\s*1,\s*"episodes":\s*\[(.*?)\],\s*"season":\s*2,\s*"episodes":\s*\[(.*?)\]\s*\}`)
		if m := reMultiSeason.FindStringSubmatch(text); len(m) > 5 {
			s1Obj := fmt.Sprintf(`{"id": %s, "title": "%s", "type": "tv", "quality": "%s", "season": 1, "episodes": [%s]}`, m[1], m[2], m[3], m[4])
			s2Obj := fmt.Sprintf(`{"id": %s, "title": "%s", "type": "tv", "quality": "%s", "season": 2, "episodes": [%s]}`, m[1], m[2], m[3], m[5])
			text = strings.Replace(text, m[0], s1Obj+", "+s2Obj, 1)
		}
	}

	// Clean any remaining trailing commas
	text = regexp.MustCompile(`,\s*([\]}])`).ReplaceAllString(text, "$1")

	if err := json.Unmarshal([]byte(text), &items); err == nil && len(items) > 0 {
		return items, nil
	}

	// Fallback regex scanner if JSON is still structurally damaged
	return v.scanItemsManually(string(raw)), nil
}

// scanItemsManually extracts items using resilient regex scanning.
func (v *VidaraTier) scanItemsManually(raw string) []models.MediaItem {
	var results []models.MediaItem

	// Scan movies
	reMovie := regexp.MustCompile(`(?i)\{\s*"id":\s*(\d+).*?"title":\s*"([^"]+)".*?"type":\s*"movie".*?"va":\s*"([^"]+)".*?"st":\s*"([^"]+)"`)
	for _, m := range reMovie.FindAllStringSubmatch(raw, -1) {
		id, _ := strconv.Atoi(m[1])
		results = append(results, models.MediaItem{
			ID:    id,
			Title: m[2],
			Type:  "movie",
			Play: &models.Playback{
				VA: CleanID(m[3]),
				ST: CleanID(m[4]),
			},
		})
	}

	// Scan TV shows (id 262838 or similar)
	reTV := regexp.MustCompile(`(?i)"id":\s*(\d+).*?"title":\s*"([^"]+)".*?"type":\s*"tv"`)
	tvMatch := reTV.FindStringSubmatch(raw)
	if len(tvMatch) > 2 {
		tvID, _ := strconv.Atoi(tvMatch[1])
		title := tvMatch[2]

		// Extract season blocks
		for seasonNum := 1; seasonNum <= 10; seasonNum++ {
			sPattern := fmt.Sprintf(`(?is)"season":\s*%d,\s*"episodes":\s*\[(.*?)\]`, seasonNum)
			reSeason := regexp.MustCompile(sPattern)
			if sMatch := reSeason.FindStringSubmatch(raw); len(sMatch) > 1 {
				epBlock := sMatch[1]
				reEp := regexp.MustCompile(`(?i)"episode":\s*(\d+).*?"va":\s*"([^"]+)".*?"st":\s*"([^"]+)"`)
				var episodes []models.EpisodeItem
				for _, epM := range reEp.FindAllStringSubmatch(epBlock, -1) {
					epNum, _ := strconv.Atoi(epM[1])
					episodes = append(episodes, models.EpisodeItem{
						Episode: epNum,
						Play: models.Playback{
							VA: CleanID(epM[2]),
							ST: CleanID(epM[3]),
						},
					})
				}
				if len(episodes) > 0 {
					results = append(results, models.MediaItem{
						ID:       tvID,
						Title:    title,
						Type:     "tv",
						Season:   seasonNum,
						Episodes: episodes,
					})
				}
			}
		}
	}

	return results
}

// FindMovie looks up movie by TMDB ID in memory.
func (v *VidaraTier) FindMovie(tmdbID int) (MovieRecord, bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	rec, ok := v.movieIndex[tmdbID]
	return rec, ok
}

// FindTV looks up TV episode by TMDB ID, season, and episode in memory.
func (v *VidaraTier) FindTV(tmdbID, season, episode int) (TVEpisodeRecord, bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	seasons, ok := v.tvIndex[tmdbID]
	if !ok {
		return TVEpisodeRecord{}, false
	}
	episodes, ok := seasons[season]
	if !ok {
		return TVEpisodeRecord{}, false
	}
	rec, ok := episodes[episode]
	return rec, ok
}

// ResolveMovie checks Tier 1 for movie resolution.
func (v *VidaraTier) ResolveMovie(ctx context.Context, tmdbID int) (*models.ResolutionResult, error) {
	log.Printf("[tier1] checking Vidara movie %d", tmdbID)
	rec, found := v.FindMovie(tmdbID)
	if !found {
		log.Printf("[tier1] movie %d not found in local index", tmdbID)
		return nil, nil
	}

	log.Printf("[tier1] found movie %d (%s) in Vidara dataset", tmdbID, rec.Title)
	res := &models.ResolutionResult{
		Success: true,
		TmdbID:  tmdbID,
		Type:    "movie",
		Tier:    1,
		Source:  "vidara",
		Title:   rec.Title,
		Headers: map[string]string{
			"Referer": "https://vidara.to/",
			"Origin":  "https://vidara.to",
		},
		Play: &models.Playback{
			VA: rec.Play.VA,
			ST: rec.Play.ST,
		},
	}

	// Attempt stream resolution with a fast bounded timeout
	streamURL := v.tryResolveStream(ctx, rec.Play.VA, rec.Play.ST)
	if streamURL != "" {
		res.URL = streamURL
	}

	return res, nil
}

// ResolveTV checks Tier 1 for TV episode resolution.
func (v *VidaraTier) ResolveTV(ctx context.Context, tmdbID, season, episode int) (*models.ResolutionResult, error) {
	log.Printf("[tier1] checking Vidara TV %d S%dE%d", tmdbID, season, episode)
	rec, found := v.FindTV(tmdbID, season, episode)
	if !found {
		log.Printf("[tier1] TV %d S%dE%d not found in local index", tmdbID, season, episode)
		return nil, nil
	}

	log.Printf("[tier1] found TV %d S%dE%d (%s) in Vidara dataset", tmdbID, season, episode, rec.Title)
	res := &models.ResolutionResult{
		Success: true,
		TmdbID:  tmdbID,
		Type:    "tv",
		Tier:    1,
		Source:  "vidara",
		Title:   rec.Title,
		Season:  &season,
		Episode: &episode,
		Headers: map[string]string{
			"Referer": "https://vidara.to/",
			"Origin":  "https://vidara.to",
		},
		Play: &models.Playback{
			VA: rec.Play.VA,
			ST: rec.Play.ST,
		},
	}

	// Attempt stream resolution with a fast bounded timeout
	streamURL := v.tryResolveStream(ctx, rec.Play.VA, rec.Play.ST)
	if streamURL != "" {
		res.URL = streamURL
	}

	return res, nil
}

// tryResolveStream attempts to fetch playable stream URL from VA or ST within context.
func (v *VidaraTier) tryResolveStream(ctx context.Context, vaID, stID string) string {
	resolveCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	if vaID != "" {
		log.Printf("[tier1] extracting stream for va=%s", vaID)
		if ext, err := v.ExtractVidara(resolveCtx, vaID); err == nil && ext != nil && ext.URL != "" {
			log.Printf("[tier1] successfully extracted Vidara stream: %s", ext.URL)
			return ext.URL
		} else if err != nil {
			log.Printf("[tier1] ExtractVidara error for %s: %v", vaID, err)
		}
	}
	if stID != "" {
		log.Printf("[tier1] extracting stream for st=%s", stID)
		if ext, err := v.ExtractStreamTape(resolveCtx, stID); err == nil && ext != nil && ext.URL != "" {
			log.Printf("[tier1] successfully extracted Streamtape stream: %s", ext.URL)
			return ext.URL
		} else if err != nil {
			log.Printf("[tier1] ExtractStreamTape error for %s: %v", stID, err)
		}
	}
	return ""
}

// ExtractVidara extracts streaming URL from Vidara upstream.
func (v *VidaraTier) ExtractVidara(ctx context.Context, urlOrID string) (*models.ExtractorResponse, error) {
	mainURL := v.vidaraBase
	fileCode := CleanID(urlOrID)

	if strings.Contains(urlOrID, "://") {
		u, err := url.Parse(urlOrID)
		if err == nil {
			scheme := u.Scheme
			if scheme == "" {
				scheme = "https"
			}
			mainURL = scheme + "://" + u.Host
			parts := strings.Split(strings.TrimRight(u.Path, "/"), "/")
			fileCode = parts[len(parts)-1]
		}
	}

	apiURL := mainURL + "/api/stream"
	payload := map[string]string{
		"filecode": fileCode,
		"device":   "web",
	}
	jsonBody, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, val := range defaultHeaders {
		req.Header.Set(k, val)
	}

	log.Printf("[tier1] querying Vidara API: %s with filecode=%s", apiURL, fileCode)
	resp, err := v.client.Do(req)
	if err != nil {
		log.Printf("[tier1] Vidara API request error for %s: %v", fileCode, err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[tier1] Vidara API returned non-200 status %d for filecode=%s", resp.StatusCode, fileCode)
		return nil, fmt.Errorf("vidara api returned status %d", resp.StatusCode)
	}

	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		log.Printf("[tier1] Vidara API error decoding JSON for %s: %v", fileCode, err)
		return nil, err
	}

	streamingURL, _ := data["streaming_url"].(string)
	title, _ := data["title"].(string)
	subtitles := data["subtitles"]
	log.Printf("[tier1] Vidara API resolved filecode=%s: title='%s', streaming_url=%s", fileCode, title, streamingURL)

	var thumbnail *string
	if th, ok := data["thumbnail"].(string); ok && th != "" {
		thumbnail = &th
	}

	var dynamicAudio []models.AudioTrack
	if streamingURL != "" && strings.Contains(streamingURL, ".m3u8") {
		dynamicAudio = v.ExtractAudioTracksFromM3U(ctx, streamingURL)
	}

	streamType := "direct"
	if strings.HasSuffix(streamingURL, ".m3u8") || strings.Contains(streamingURL, ".m3u8?") {
		streamType = "m3u8"
	}

	return &models.ExtractorResponse{
		Status:      "success",
		Success:     true,
		Source:      "Secondary Server",
		OriginalURL: "/v/" + fileCode,
		URL:         streamingURL,
		Text:        title,
		Title:       title,
		Thumbnail:   thumbnail,
		Subtitles:   subtitles,
		AudioTracks: dynamicAudio,
		StreamType:  streamType,
		Headers: map[string]string{
			"Referer": "https://vidara.to/",
			"Origin":  "https://vidara.to",
		},
	}, nil
}

// ExtractStreamTape extracts streaming URL from Streamtape upstream.
func (v *VidaraTier) ExtractStreamTape(ctx context.Context, urlOrID string) (*models.ExtractorResponse, error) {
	clean := CleanID(urlOrID)
	targetURL := fmt.Sprintf("%s/v/%s/", v.streamBase, clean)
	if strings.Contains(urlOrID, "://") {
		targetURL = urlOrID
	}

	req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		return nil, err
	}
	for k, val := range defaultHeaders {
		req.Header.Set(k, val)
	}

	log.Printf("[tier1] querying Streamtape page: %s", targetURL)
	resp, err := v.client.Do(req)
	if err != nil {
		log.Printf("[tier1] Streamtape HTTP request error: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[tier1] Streamtape returned status %d for %s", resp.StatusCode, targetURL)
		return nil, fmt.Errorf("streamtape returned status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	html := string(bodyBytes)

	title := ""
	if m := reTitle.FindStringSubmatch(html); len(m) > 1 {
		rawTitle := strings.TrimSpace(m[1])
		title = regexp.MustCompile(`(?i)\s+at\s+Streamtape\.com.*$`).ReplaceAllString(rawTitle, "")
	}

	matchExpr := ""
	if m := reBotlink.FindStringSubmatch(html); len(m) > 1 {
		matchExpr = strings.TrimSpace(m[1])
	} else if m := reRobotlink.FindStringSubmatch(html); len(m) > 1 {
		matchExpr = strings.TrimSpace(m[1])
	}

	if matchExpr == "" {
		log.Printf("[tier1] Streamtape token not found in HTML for %s", targetURL)
		return nil, fmt.Errorf("could not find media stream token in HTML")
	}

	evalResult := EvalStreamTapeJS(ctx, matchExpr)
	if evalResult == "" {
		log.Printf("[tier1] failed evaluating Streamtape JS token expression: %s", matchExpr)
		return nil, fmt.Errorf("failed to evaluate media script token")
	}

	var streamURL string
	if strings.HasPrefix(evalResult, "//") {
		streamURL = "https:" + evalResult + "&stream=1"
	} else if strings.HasPrefix(evalResult, "http") {
		streamURL = evalResult + "&stream=1"
	} else {
		streamURL = "https://" + evalResult + "&stream=1"
	}

	// Follow 302 redirect to resolve direct TapeContent CDN URL
	tapecontentURL := v.resolveRedirect(ctx, streamURL, targetURL)
	finalURL := tapecontentURL
	if finalURL == "" {
		finalURL = streamURL
	}

	log.Printf("[tier1] Streamtape resolved stream for %s: title='%s', cdn_url=%s", clean, title, finalURL)

	proxyURL := fmt.Sprintf("/api/proxy/stream?url=%s", url.QueryEscape(finalURL))

	return &models.ExtractorResponse{
		Status:         "success",
		Success:        true,
		Source:         "Primary Server",
		OriginalURL:    "/v/" + clean,
		URL:            proxyURL,
		ProxyURL:       proxyURL,
		DirectURL:      finalURL,
		TapecontentURL: finalURL,
		StreamURL:      streamURL,
		Text:           title,
		Title:          title,
		StreamType:     "mp4",
		Headers: map[string]string{
			"Referer": "https://streamtape.com/",
			"Origin":  "https://streamtape.com",
		},
	}, nil
}

// resolveRedirect follows HTTP 302 to locate the Location header.
func (v *VidaraTier) resolveRedirect(ctx context.Context, streamURL, referer string) string {
	headReq, err := http.NewRequestWithContext(ctx, "HEAD", streamURL, nil)
	if err == nil {
		for k, val := range defaultHeaders {
			headReq.Header.Set(k, val)
		}
		headReq.Header.Set("Referer", referer)

		resp, err := v.noRedirClient.Do(headReq)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode >= 300 && resp.StatusCode < 400 {
				if loc := resp.Header.Get("Location"); loc != "" {
					return loc
				}
			} else if resp.StatusCode == 200 {
				return resp.Request.URL.String()
			}
		}
	}
	return ""
}

// EvalStreamTapeJS evaluates Streamtape token expressions with substring chaining.
func EvalStreamTapeJS(ctx context.Context, expr string) string {
	// First try Node.js if available on system
	if _, err := exec.LookPath("node"); err == nil {
		cmdCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		cmd := exec.CommandContext(cmdCtx, "node", "-p", expr)
		out, err := cmd.Output()
		if err == nil {
			res := strings.TrimSpace(string(out))
			if res != "" {
				return res
			}
		}
	}

	// Pure Go interpreter matching app.py regex implementation
	matches := reSubToken.FindAllStringSubmatch(expr, -1)
	var parts []string
	for _, m := range matches {
		val := m[1]
		subs := m[2]
		for _, subMatch := range reSubCall.FindAllStringSubmatch(subs, -1) {
			start, _ := strconv.Atoi(subMatch[1])
			end := len(val)
			if subMatch[2] != "" {
				if e, err := strconv.Atoi(subMatch[2]); err == nil {
					end = e
				}
			}
			if start > len(val) {
				start = len(val)
			}
			if end > len(val) {
				end = len(val)
			}
			if start < end {
				val = val[start:end]
			} else {
				val = ""
			}
		}
		parts = append(parts, val)
	}
	return strings.Join(parts, "")
}

// ExtractAudioTracksFromM3U parses dynamic audio tracks from master playlist.
func (v *VidaraTier) ExtractAudioTracksFromM3U(ctx context.Context, m3uURL string) []models.AudioTrack {
	if !strings.HasPrefix(m3uURL, "http") {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, "GET", m3uURL, nil)
	if err != nil {
		return nil
	}
	for k, val := range defaultHeaders {
		req.Header.Set(k, val)
	}

	resp, err := v.client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	base, err := url.Parse(m3uURL)
	if err != nil {
		return nil
	}

	var tracks []models.AudioTrack
	seen := make(map[string]bool)

	for _, line := range strings.Split(string(bodyBytes), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "#EXT-X-MEDIA:") || !strings.Contains(line, "TYPE=AUDIO") {
			continue
		}

		mURI := reURIAttr.FindStringSubmatch(line)
		if len(mURI) < 2 {
			continue
		}
		relURI := strings.TrimSpace(mURI[1])
		parsedRel, err := url.Parse(relURI)
		if err != nil {
			continue
		}
		trackURI := base.ResolveReference(parsedRel).String()

		name := "Audio Track"
		if m := reNameAttr.FindStringSubmatch(line); len(m) > 1 {
			name = strings.TrimSpace(m[1])
		}

		lang := ""
		if m := reLangAttr.FindStringSubmatch(line); len(m) > 1 {
			lang = strings.TrimSpace(m[1])
		}
		if name == "Audio Track" && lang != "" {
			name = lang
		}

		isDefault := false
		if m := reDefAttr.FindStringSubmatch(line); len(m) > 1 {
			isDefault = strings.EqualFold(m[1], "YES")
		}

		tid := fmt.Sprintf("%s|%s|%s", lang, name, trackURI)
		if seen[tid] {
			continue
		}
		seen[tid] = true

		key := lang
		if key == "" {
			key = name
		}
		cleanKey := reAlphaNumScore.ReplaceAllString(strings.ToLower(key), "_")

		tracks = append(tracks, models.AudioTrack{
			ID:       cleanKey,
			Name:     name,
			Language: lang,
			Default:  isDefault,
			URL:      trackURI,
		})
	}

	log.Printf("[tier1] parsed %d audio tracks from M3U playlist %s", len(tracks), m3uURL)
	return tracks
}

// ExtractAny extracts stream from any supported host.
func (v *VidaraTier) ExtractAny(ctx context.Context, targetURL string) (*models.ExtractorResponse, error) {
	targetURL = strings.TrimSpace(targetURL)
	if targetURL == "" {
		return nil, fmt.Errorf("url parameter cannot be empty")
	}

	parsed, err := url.Parse(targetURL)
	if err != nil {
		return nil, err
	}
	domain := strings.ToLower(parsed.Hostname())

	if vidaraDomains[domain] || strings.Contains(domain, "vidara") {
		return v.ExtractVidara(ctx, targetURL)
	}
	if streamtapeDomains[domain] || strings.Contains(domain, "streamtape") {
		return v.ExtractStreamTape(ctx, targetURL)
	}

	if strings.Contains(targetURL, "/api/stream") || strings.Contains(targetURL, "/v/") {
		res, err := v.ExtractVidara(ctx, targetURL)
		if err == nil {
			return res, nil
		}
		return v.ExtractStreamTape(ctx, targetURL)
	}

	return nil, fmt.Errorf("unsupported host/domain '%s'", domain)
}

// ResolveDual resolves both Streamtape and Vidara in parallel matching app.py /api/resolve.
func (v *VidaraTier) ResolveDual(ctx context.Context, stInput, vaInput string) *models.ResolveResponse {
	stInput = CleanID(stInput)
	vaInput = CleanID(vaInput)

	if stInput == "" && vaInput == "" {
		stInput = "0A2vDYQz3wIbPQ6"
		vaInput = "d932127894f1"
	}

	log.Printf("[tier1] starting parallel dual resolution for st=%s, va=%s", stInput, vaInput)
	var stRes, vaRes *models.ExtractorResponse
	var wg sync.WaitGroup

	if stInput != "" {
		wg.Add(1)
		go func(sid string) {
			defer wg.Done()
			res, err := v.ExtractStreamTape(ctx, sid)
			if err == nil {
				stRes = res
			} else {
				stRes = &models.ExtractorResponse{
					Status: "error",
					Error:  err.Error(),
					ID:     sid,
				}
			}
		}(stInput)
	}

	if vaInput != "" {
		wg.Add(1)
		go func(vid string) {
			defer wg.Done()
			res, err := v.ExtractVidara(ctx, vid)
			if err == nil {
				vaRes = res
			} else {
				vaRes = &models.ExtractorResponse{
					Status: "error",
					Error:  err.Error(),
					ID:     vid,
				}
			}
		}(vaInput)
	}

	wg.Wait()

	stOk := stRes != nil && stRes.Status == "success" && stRes.URL != ""
	vaOk := vaRes != nil && vaRes.Status == "success" && vaRes.URL != ""

	mode := "none"
	if stOk && vaOk {
		mode = "dual"
	} else if stOk {
		mode = "primary_only"
	} else if vaOk {
		mode = "secondary_only"
	}

	log.Printf("[tier1] dual resolution finished: mode=%s (st_ok=%t, va_ok=%t)", mode, stOk, vaOk)

	var thumb *string
	if vaRes != nil && vaRes.Thumbnail != nil {
		thumb = vaRes.Thumbnail
	} else if stRes != nil && stRes.Thumbnail != nil {
		thumb = stRes.Thumbnail
	}

	var subs interface{} = []interface{}{}
	if vaRes != nil && vaRes.Subtitles != nil {
		subs = vaRes.Subtitles
	}

	var dynAudio []models.AudioTrack
	if vaRes != nil && len(vaRes.AudioTracks) > 0 {
		dynAudio = vaRes.AudioTracks
	}

	audioTracks := []models.AudioTrack{
		{
			ID:       "original",
			Name:     "Original Audio (Fast)",
			Language: "original",
			Default:  true,
			URL:      "",
		},
	}
	audioTracks = append(audioTracks, dynAudio...)

	status := "error"
	if stOk || vaOk {
		status = "success"
	}

	return &models.ResolveResponse{
		Status:      status,
		Mode:        mode,
		STID:        stInput,
		VAID:        vaInput,
		Primary:     stRes,
		Secondary:   vaRes,
		ST:          stRes,
		VA:          vaRes,
		Thumbnail:   thumb,
		Subtitles:   subs,
		AudioTracks: audioTracks,
	}
}

// FindFirstExistingFile finds the first file that exists from a list.
func FindFirstExistingFile(paths ...string) string {
	for _, p := range paths {
		if p == "" {
			continue
		}
		abs, err := filepath.Abs(p)
		if err == nil {
			if _, err := os.Stat(abs); err == nil {
				return abs
			}
		}
	}
	return ""
}
