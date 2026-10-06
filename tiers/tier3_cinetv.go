package tiers

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/des"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	mathrand "math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"vidara-api/models"
)

const (
	cinetvUserAgent          = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	cinetvSecretKeyEncrypted = "MxASAkl/yHTGg+/Tw1R7u96nGqkWsOZ2"
	cinetvDesKey             = "dsawdf634eebGFHITR5UT9kS0"
	cinetvDesIV              = "32456738"
	cinetvAesKeyStr          = "0123456789123456"
	cinetvAesIVStr           = "2015030120123456"
	cinetvWsSecret           = "00b5f05c40b4f1d91dbc9b3fd8a059ef"
	cinetvAppID              = "filmin"
	cinetvChannelCode        = "filmin_sh_1000"
	cinetvPackageName        = "com.dramarush.shortin"
	cinetvP2PSalt            = "Zox882LYjEn4Rqpa"
)

var (
	cinetvBrandModels = map[string][]string{
		"Samsung": {"SM-S918B", "SM-A528B", "SM-M336B"},
		"Xiaomi":  {"2201117TI", "M2012K11AI", "Redmi Note 11"},
		"OnePlus": {"LE2111", "CPH2449", "IN2023"},
		"Google":  {"Pixel 6", "Pixel 7", "Pixel 8"},
		"Realme":  {"RMX3085", "RMX3360", "RMX3551"},
	}
)

// CineTVTier represents Tier 3 CineTV resolver.
type CineTVTier struct {
	mainURL     string
	hostHeader  string
	tmdbKey     string
	deviceID    string
	mobMfr      string
	mobModel    string
	cachedToken string
	tokenMu     sync.Mutex
	client      *http.Client
}

// NewCineTVTier initializes a new CineTV Tier.
func NewCineTVTier(mainURL, tmdbKey string, timeout time.Duration) *CineTVTier {
	if mainURL == "" {
		mainURL = "https://filmin.ajfysu.com"
	}
	if tmdbKey == "" {
		tmdbKey = "e6333b32409e02a4a6eba6fb7ff866bb"
	}

	b := make([]byte, 16)
	_, _ = rand.Read(b)
	devID := hex.EncodeToString(b)

	var mfrs []string
	for k := range cinetvBrandModels {
		mfrs = append(mfrs, k)
	}
	chosenMfr := mfrs[mathrand.Intn(len(mfrs))]
	modelsList := cinetvBrandModels[chosenMfr]
	chosenModel := modelsList[mathrand.Intn(len(modelsList))]

	parsed, _ := url.Parse(mainURL)
	host := "filmin.ajfysu.com"
	if parsed != nil && parsed.Host != "" {
		host = parsed.Host
	}

	return &CineTVTier{
		mainURL:    strings.TrimRight(mainURL, "/"),
		hostHeader: host,
		tmdbKey:    tmdbKey,
		deviceID:   devID,
		mobMfr:     chosenMfr,
		mobModel:   chosenModel,
		client:     &http.Client{Timeout: timeout},
	}
}

// TmdbDetails holds extracted metadata from TMDB.
type TmdbDetails struct {
	ID    int
	Title string
	Year  string
	Type  string
}

func (c *CineTVTier) md5Hex(text string) string {
	hash := md5.Sum([]byte(text))
	return hex.EncodeToString(hash[:])
}

func (c *CineTVTier) des3Decrypt(encryptedBase64 string) string {
	data, err := base64.StdEncoding.DecodeString(encryptedBase64)
	if err != nil {
		return ""
	}
	keyBytes := make([]byte, 24)
	desKeyBytes := []byte(cinetvDesKey)
	copy(keyBytes, desKeyBytes)

	block, err := des.NewTripleDESCipher(keyBytes)
	if err != nil {
		return ""
	}

	mode := cipher.NewCBCDecrypter(block, []byte(cinetvDesIV))
	decrypted := make([]byte, len(data))
	mode.CryptBlocks(decrypted, data)

	padding := int(decrypted[len(decrypted)-1])
	if padding > 0 && padding <= 8 {
		decrypted = decrypted[:len(decrypted)-padding]
	}

	return string(decrypted)
}

func (c *CineTVTier) generateSign(curTime string) string {
	secret := c.des3Decrypt(cinetvSecretKeyEncrypted)
	return strings.ToUpper(c.md5Hex(secret + c.deviceID + curTime))
}

func (c *CineTVTier) generateP2pToken(vodID, timestamp string) string {
	return strings.ToUpper(c.md5Hex(cinetvP2PSalt + c.deviceID + vodID + timestamp))
}

func (c *CineTVTier) signVideoURL(videoURL string) string {
	if videoURL == "" {
		return ""
	}
	u, err := url.Parse(videoURL)
	if err != nil {
		return videoURL
	}
	nowSec := time.Now().Unix()
	wsTime := strconv.FormatInt(nowSec+60, 16)
	wsSecret := c.md5Hex(cinetvWsSecret + u.Path + wsTime)
	sep := "?"
	if strings.Contains(videoURL, "?") {
		sep = "&"
	}
	signed := fmt.Sprintf("%s%swsSecret=%s&wsTime=%s", videoURL, sep, wsSecret, wsTime)
	log.Printf("[tier3] generated signed stream URL: %s", signed)
	return signed
}

func (c *CineTVTier) aesDecrypt(encryptedBase64 string) string {
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encryptedBase64))
	if err != nil {
		return ""
	}

	block, err := aes.NewCipher([]byte(cinetvAesKeyStr))
	if err != nil {
		return ""
	}

	if len(data)%aes.BlockSize != 0 {
		return ""
	}

	mode := cipher.NewCBCDecrypter(block, []byte(cinetvAesIVStr))
	unpadded := make([]byte, len(data))
	mode.CryptBlocks(unpadded, data)

	padding := int(unpadded[len(unpadded)-1])
	if padding > 0 && padding <= 16 {
		unpadded = unpadded[:len(unpadded)-padding]
	}

	if len(unpadded) >= 2 && unpadded[0] == 0x1f && unpadded[1] == 0x8b {
		gzipReader, err := gzip.NewReader(bytes.NewReader(unpadded))
		if err == nil {
			defer gzipReader.Close()
			decompressed, err := io.ReadAll(gzipReader)
			if err == nil {
				return string(decompressed)
			}
		}
	}

	return string(unpadded)
}

func (c *CineTVTier) buildHeaders(curTime, token string) map[string]string {
	return map[string]string{
		"Accept-Encoding": "identity",
		"androidid":       c.deviceID,
		"app_id":          cinetvAppID,
		"app_language":    "en",
		"channel_code":    cinetvChannelCode,
		"Connection":      "Keep-Alive",
		"Content-Type":    "application/x-www-form-urlencoded",
		"cur_time":        curTime,
		"device_id":       c.deviceID,
		"en_al":           "0",
		"gaid":            "",
		"Host":            c.hostHeader,
		"is_display":      "GMT+05:30",
		"is_language":     "en",
		"is_vvv":          "0",
		"log-header":      "I am the log request header.",
		"mob_mfr":         c.mobMfr,
		"mobmodel":        c.mobModel,
		"package_name":    cinetvPackageName,
		"sign":            c.generateSign(curTime),
		"sys_platform":    "2",
		"sysrelease":      "13",
		"token":           token,
		"User-Agent":      "okhttp/4.11.0",
		"version":         "30000",
	}
}

func (c *CineTVTier) httpsPost(ctx context.Context, endpoint string, formData, headers map[string]string) []byte {
	u := c.mainURL + endpoint
	data := url.Values{}
	for k, v := range formData {
		data.Set(k, v)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", u, strings.NewReader(data.Encode()))
	if err != nil {
		return nil
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	return body
}

func (c *CineTVTier) fetchToken(ctx context.Context) string {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()

	if c.cachedToken != "" {
		return c.cachedToken
	}

	log.Printf("[tier3] token cache empty, requesting session token from /api/public/init")
	curTime := strconv.FormatInt(time.Now().UnixMilli(), 10)
	headers := c.buildHeaders(curTime, "")
	buf := c.httpsPost(ctx, "/api/public/init", map[string]string{"invited_by": "", "is_install": "1"}, headers)
	if len(buf) == 0 {
		log.Printf("[tier3] /api/public/init returned empty body")
		return ""
	}

	text := strings.TrimSpace(string(buf))
	jsonStr := text
	if !strings.HasPrefix(text, "{") {
		jsonStr = c.aesDecrypt(text)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err == nil {
		if result, ok := parsed["result"].(map[string]interface{}); ok {
			if userInfo, ok := result["user_info"].(map[string]interface{}); ok {
				if t, ok := userInfo["token"].(string); ok && t != "" {
					c.cachedToken = t
					log.Printf("[tier3] acquired new session token: %s...", t[:min(8, len(t))])
					return c.cachedToken
				}
			}
		}
	}
	log.Printf("[tier3] failed parsing session token from /api/public/init")
	return ""
}

func (c *CineTVTier) apiPost(ctx context.Context, endpoint string, formData map[string]string) map[string]interface{} {
	token := c.fetchToken(ctx)
	curTime := strconv.FormatInt(time.Now().UnixMilli(), 10)
	log.Printf("[tier3] POST %s with formData=%v", endpoint, formData)
	buf := c.httpsPost(ctx, endpoint, formData, c.buildHeaders(curTime, token))
	if len(buf) == 0 {
		log.Printf("[tier3] POST %s returned empty response", endpoint)
		return nil
	}

	dec := c.aesDecrypt(strings.TrimSpace(string(buf)))
	var result map[string]interface{}
	_ = json.Unmarshal([]byte(dec), &result)
	return result
}

func (c *CineTVTier) getVodInfo(ctx context.Context, vodID string, audioType int) map[string]interface{} {
	log.Printf("[tier3] fetching VOD info for vod_id=%s, audio_type=%d", vodID, audioType)
	token := c.fetchToken(ctx)
	curTime := strconv.FormatInt(time.Now().UnixMilli(), 10)
	buf := c.httpsPost(
		ctx,
		"/api/vod/info_new",
		map[string]string{
			"sign":       c.generateP2pToken(vodID, curTime),
			"vod_id":     vodID,
			"cur_time":   curTime,
			"audio_type": strconv.Itoa(audioType),
		},
		c.buildHeaders(curTime, token),
	)
	if len(buf) == 0 {
		log.Printf("[tier3] /api/vod/info_new returned empty body for vod_id=%s", vodID)
		return nil
	}

	dec := c.aesDecrypt(strings.TrimSpace(string(buf)))
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(dec), &result); err != nil {
		log.Printf("[tier3] failed parsing decrypted VOD info for vod_id=%s: %v", vodID, err)
		return nil
	}

	if resObj, ok := result["result"].(map[string]interface{}); ok {
		if collections, ok := resObj["vod_collection"].([]interface{}); ok {
			for _, epIntf := range collections {
				if ep, ok := epIntf.(map[string]interface{}); ok {
					rawURL, _ := ep["vod_url"].(string)
					if rawURL == "" {
						rawURL, _ = ep["down_url"].(string)
					}
					ep["raw_url"] = rawURL
					ep["signed_url"] = c.signVideoURL(rawURL)
				}
			}
		}
	}
	return result
}

func (c *CineTVTier) fetchTmdbDetails(ctx context.Context, id, mediaType string) *TmdbDetails {
	reqURL := fmt.Sprintf("https://api.themoviedb.org/3/%s/%s?api_key=%s", mediaType, id, c.tmdbKey)
	log.Printf("[tier3] fetching TMDB %s metadata for ID %s", mediaType, id)
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		log.Printf("[tier3] error creating TMDB request: %v", err)
		return nil
	}
	req.Header.Set("User-Agent", cinetvUserAgent)

	resp, err := c.client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		log.Printf("[tier3] TMDB request failed for ID %s: err=%v", id, err)
		return nil
	}
	defer resp.Body.Close()

	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		log.Printf("[tier3] TMDB response decode error for ID %s: %v", id, err)
		return nil
	}

	idFloat, _ := data["id"].(float64)
	titleVal, _ := data["title"].(string)
	if titleVal == "" {
		titleVal, _ = data["name"].(string)
	}
	if titleVal == "" {
		titleVal, _ = data["original_title"].(string)
	}
	if titleVal == "" {
		titleVal, _ = data["original_name"].(string)
	}

	dateVal, _ := data["release_date"].(string)
	if dateVal == "" {
		dateVal, _ = data["first_air_date"].(string)
	}
	yearVal := dateVal
	if strings.Contains(dateVal, "-") {
		yearVal = strings.Split(dateVal, "-")[0]
	}

	log.Printf("[tier3] TMDB %s details resolved for ID %s: title='%s', year='%s'", mediaType, id, titleVal, yearVal)
	return &TmdbDetails{
		ID:    int(idFloat),
		Title: titleVal,
		Year:  yearVal,
		Type:  mediaType,
	}
}

func (c *CineTVTier) extractVodID(response map[string]interface{}) string {
	if response == nil {
		return ""
	}

	if items, ok := response["result"].([]interface{}); ok && len(items) > 0 {
		if item, ok := items[0].(map[string]interface{}); ok {
			if idFloat, ok := item["id"].(float64); ok {
				return strconv.FormatFloat(idFloat, 'f', -1, 64)
			} else if idStr, ok := item["id"].(string); ok {
				return idStr
			}
		}
	} else if resultMap, ok := response["result"].(map[string]interface{}); ok {
		if items, ok := resultMap["items"].([]interface{}); ok && len(items) > 0 {
			if item, ok := items[0].(map[string]interface{}); ok {
				if idFloat, ok := item["id"].(float64); ok {
					return strconv.FormatFloat(idFloat, 'f', -1, 64)
				} else if idStr, ok := item["id"].(string); ok {
					return idStr
				}
			}
		}
	}
	return ""
}

// ResolveMovie resolves a movie via Tier 3 CineTV.
func (c *CineTVTier) ResolveMovie(ctx context.Context, tmdbID int) (*models.ResolutionResult, error) {
	log.Printf("[tier3] checking CineTV for movie %d", tmdbID)

	tmdb := c.fetchTmdbDetails(ctx, strconv.Itoa(tmdbID), "movie")
	if tmdb == nil || tmdb.Title == "" {
		log.Printf("[tier3] failed: TMDB movie info not found for %d", tmdbID)
		return nil, nil
	}

	log.Printf("[tier3] searching CineTV for '%s'", tmdb.Title)
	res := c.apiPost(ctx, "/api/search/result", map[string]string{"kw": tmdb.Title, "pn": "1"})
	vodID := c.extractVodID(res)

	if vodID == "" && tmdb.Year != "" {
		res = c.apiPost(ctx, "/api/search/result", map[string]string{"kw": fmt.Sprintf("%s %s", tmdb.Title, tmdb.Year), "pn": "1"})
		vodID = c.extractVodID(res)
	}

	if vodID == "" {
		log.Printf("[tier3] failed: video not found on server for '%s'", tmdb.Title)
		return nil, nil
	}

	vodDetails := c.getVodInfo(ctx, vodID, 0)
	var signedURL string
	if vodDetails != nil {
		if result, ok := vodDetails["result"].(map[string]interface{}); ok {
			if collections, ok := result["vod_collection"].([]interface{}); ok && len(collections) > 0 {
				if ep, ok := collections[0].(map[string]interface{}); ok {
					if u, ok := ep["signed_url"].(string); ok && u != "" {
						signedURL = u
					}
				}
			}
		}
	}

	if signedURL == "" {
		log.Printf("[tier3] failed: stream URL could not be resolved for vod %s", vodID)
		return nil, nil
	}

	log.Printf("[tier3] success resolving movie %d ('%s')", tmdbID, tmdb.Title)
	return &models.ResolutionResult{
		Success: true,
		TmdbID:  tmdbID,
		Name:    tmdb.Title,
		Title:   tmdb.Title,
		Type:    "movie",
		Tier:    3,
		Source:  "cinetv",
		URL:     signedURL,
		Headers: map[string]string{
			"Referer": c.mainURL + "/",
			"Origin":  c.mainURL,
		},
	}, nil
}

// ResolveTV resolves a TV episode via Tier 3 CineTV.
func (c *CineTVTier) ResolveTV(ctx context.Context, tmdbID, season, episode int) (*models.ResolutionResult, error) {
	log.Printf("[tier3] checking CineTV for TV %d S%dE%d", tmdbID, season, episode)

	tmdb := c.fetchTmdbDetails(ctx, strconv.Itoa(tmdbID), "tv")
	if tmdb == nil || tmdb.Title == "" {
		log.Printf("[tier3] failed: TMDB TV info not found for %d", tmdbID)
		return nil, nil
	}

	query := fmt.Sprintf("%s Season %d", tmdb.Title, season)
	log.Printf("[tier3] searching CineTV for '%s'", query)
	res := c.apiPost(ctx, "/api/search/result", map[string]string{"kw": query, "pn": "1"})
	vodID := c.extractVodID(res)

	if vodID == "" {
		res = c.apiPost(ctx, "/api/search/result", map[string]string{"kw": tmdb.Title, "pn": "1"})
		vodID = c.extractVodID(res)
	}

	if vodID == "" {
		log.Printf("[tier3] failed: TV series '%s' not found on CineTV", tmdb.Title)
		return nil, nil
	}

	vodDetails := c.getVodInfo(ctx, vodID, 0)
	var signedURL string
	if vodDetails != nil {
		if result, ok := vodDetails["result"].(map[string]interface{}); ok {
			if collections, ok := result["vod_collection"].([]interface{}); ok {
				for _, epIntf := range collections {
					if ep, ok := epIntf.(map[string]interface{}); ok {
						colNum := 0
						if colFloat, ok := ep["collection"].(float64); ok {
							colNum = int(colFloat)
						}
						if colNum == episode {
							if u, ok := ep["signed_url"].(string); ok && u != "" {
								signedURL = u
								break
							}
						}
					}
				}
			}
		}
	}

	if signedURL == "" {
		log.Printf("[tier3] failed: episode %d not found in CineTV vod %s", episode, vodID)
		return nil, nil
	}

	log.Printf("[tier3] success resolving TV %d S%dE%d ('%s')", tmdbID, season, episode, tmdb.Title)
	return &models.ResolutionResult{
		Success: true,
		TmdbID:  tmdbID,
		Name:    tmdb.Title,
		Title:   tmdb.Title,
		Type:    "tv",
		Tier:    3,
		Source:  "cinetv",
		Season:  &season,
		Episode: &episode,
		URL:     signedURL,
		Headers: map[string]string{
			"Referer": c.mainURL + "/",
			"Origin":  c.mainURL,
		},
	}, nil
}
