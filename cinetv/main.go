package main

import (
	"bytes"
	"compress/gzip"
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
)

const (
	TAG                  = "NanoServer"
	DefaultPort          = 1937
	UserAgent            = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	SECRET_KEY_ENCRYPTED = "MxASAkl/yHTGg+/Tw1R7u96nGqkWsOZ2"
	DES_KEY              = "dsawdf634eebGFHITR5UT9kS0"
	DES_IV               = "32456738"
	AES_KEY_STR          = "0123456789123456"
	AES_IV_STR           = "2015030120123456"
	WS_SECRET            = "00b5f05c40b4f1d91dbc9b3fd8a059ef"
	MAIN_URL             = "https://filmin.ajfysu.com"
	HOST_HEADER          = "filmin.ajfysu.com"
	APP_ID               = "filmin"
	CHANNEL_CODE         = "filmin_sh_1000"
	PACKAGE_NAME         = "com.dramarush.shortin"
	GAID                 = ""
	P2P_SALT             = "Zox882LYjEn4Rqpa"
	TMDB_KEY             = "e6333b32409e02a4a6eba6fb7ff866bb"
)

var (
	brandModels = map[string][]string{
		"Samsung": {"SM-S918B", "SM-A528B", "SM-M336B"},
		"Xiaomi":  {"2201117TI", "M2012K11AI", "Redmi Note 11"},
		"OnePlus": {"LE2111", "CPH2449", "IN2023"},
		"Google":  {"Pixel 6", "Pixel 7", "Pixel 8"},
		"Realme":  {"RMX3085", "RMX3360", "RMX3551"},
	}
	deviceId    string
	mobMfr      string
	mobModel    string
	cachedToken string
	tokenLock   sync.Mutex
)

func init() {
	b := make([]byte, 16)
	rand.Read(b)
	deviceId = hex.EncodeToString(b)

	var keys []string
	for k := range brandModels {
		keys = append(keys, k)
	}
	mobMfr = keys[mathrand.Intn(len(keys))]
	models := brandModels[mobMfr]
	mobModel = models[mathrand.Intn(len(models))]
}

type TmdbInfo struct {
	ID      int    `json:"id"`
	Title   string `json:"title"`
	Year    string `json:"year"`
	Type    string `json:"type"`
	Season  *int   `json:"season,omitempty"`
	Episode *int   `json:"episode,omitempty"`
}

type NewapiStreamResult struct {
	Title      string   `json:"title"`
	Backdrop   string   `json:"backdrop"`
	StreamURLs []string `json:"streamURLs"`
	Found      bool     `json:"found"`
}

type TierResult struct {
	Tier       int               `json:"tier"`
	SourceName string            `json:"sourceName"`
	Title      string            `json:"title"`
	StreamURLs []string          `json:"streamURLs"`
	Headers    map[string]string `json:"headers,omitempty"`
	Found      bool              `json:"found"`
}

func md5Hex(text string) string {
	hash := md5.Sum([]byte(text))
	return hex.EncodeToString(hash[:])
}

func des3Decrypt(encryptedBase64 string) string {
	data, err := base64.StdEncoding.DecodeString(encryptedBase64)
	if err != nil {
		return ""
	}
	keyBytes := make([]byte, 24)
	desKeyBytes := []byte(DES_KEY)
	copy(keyBytes, desKeyBytes)

	block, err := des.NewTripleDESCipher(keyBytes)
	if err != nil {
		return ""
	}

	mode := cipher.NewCBCDecrypter(block, []byte(DES_IV))
	decrypted := make([]byte, len(data))
	mode.CryptBlocks(decrypted, data)

	// Unpad PKCS5
	padding := int(decrypted[len(decrypted)-1])
	if padding > 0 && padding <= 8 {
		decrypted = decrypted[:len(decrypted)-padding]
	}

	return string(decrypted)
}

func generateSign(curTime string) string {
	secret := des3Decrypt(SECRET_KEY_ENCRYPTED)
	return strings.ToUpper(md5Hex(secret + deviceId + curTime))
}

func generateP2pToken(vodId, timestamp string) string {
	return strings.ToUpper(md5Hex(P2P_SALT + deviceId + vodId + timestamp))
}

func signVideoUrl(videoUrl string) string {
	if videoUrl == "" {
		return ""
	}
	u, err := url.Parse(videoUrl)
	if err != nil {
		return videoUrl
	}
	nowSec := time.Now().Unix()
	wsTime := strconv.FormatInt(nowSec+60, 16)
	wsSecret := md5Hex(WS_SECRET + u.Path + wsTime)
	sep := "?"
	if strings.Contains(videoUrl, "?") {
		sep = "&"
	}
	return fmt.Sprintf("%s%swsSecret=%s&wsTime=%s", videoUrl, sep, wsSecret, wsTime)
}

func aesDecrypt(encryptedBase64 string) string {
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encryptedBase64))
	if err != nil {
		return ""
	}

	block, err := aes.NewCipher([]byte(AES_KEY_STR))
	if err != nil {
		return ""
	}

	mode := cipher.NewCBCDecrypter(block, []byte(AES_IV_STR))
	unpadded := make([]byte, len(data))
	mode.CryptBlocks(unpadded, data)

	// Unpad PKCS5
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

func buildHeaders(curTime, token string) map[string]string {
	return map[string]string{
		"Accept-Encoding": "identity",
		"androidid":       deviceId,
		"app_id":          APP_ID,
		"app_language":    "en",
		"channel_code":    CHANNEL_CODE,
		"Connection":      "Keep-Alive",
		"Content-Type":    "application/x-www-form-urlencoded",
		"cur_time":        curTime,
		"device_id":       deviceId,
		"en_al":           "0",
		"gaid":            GAID,
		"Host":            HOST_HEADER,
		"is_display":      "GMT+05:30",
		"is_language":     "en",
		"is_vvv":          "0",
		"log-header":      "I am the log request header.",
		"mob_mfr":         mobMfr,
		"mobmodel":        mobModel,
		"package_name":    PACKAGE_NAME,
		"sign":            generateSign(curTime),
		"sys_platform":    "2",
		"sysrelease":      "13",
		"token":           token,
		"User-Agent":      "okhttp/4.11.0",
		"version":         "30000",
	}
}

func httpsPost(endpoint string, formData, headers map[string]string) []byte {
	u := MAIN_URL + endpoint
	data := url.Values{}
	for k, v := range formData {
		data.Set(k, v)
	}

	req, err := http.NewRequest("POST", u, strings.NewReader(data.Encode()))
	if err != nil {
		return nil
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	return body
}

func fetchToken() string {
	tokenLock.Lock()
	defer tokenLock.Unlock()

	if cachedToken != "" {
		return cachedToken
	}

	curTime := strconv.FormatInt(time.Now().UnixMilli(), 10)
	headers := buildHeaders(curTime, "")
	buf := httpsPost("/api/public/init", map[string]string{"invited_by": "", "is_install": "1"}, headers)
	if len(buf) == 0 {
		return ""
	}

	text := strings.TrimSpace(string(buf))
	jsonStr := text
	if !strings.HasPrefix(text, "{") {
		jsonStr = aesDecrypt(text)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err == nil {
		if result, ok := parsed["result"].(map[string]interface{}); ok {
			if userInfo, ok := result["user_info"].(map[string]interface{}); ok {
				if t, ok := userInfo["token"].(string); ok && t != "" {
					cachedToken = t
					return cachedToken
				}
			}
		}
	}
	return ""
}

func apiPost(endpoint string, formData map[string]string) map[string]interface{} {
	token := fetchToken()
	curTime := strconv.FormatInt(time.Now().UnixMilli(), 10)
	buf := httpsPost(endpoint, formData, buildHeaders(curTime, token))
	if len(buf) == 0 {
		return nil
	}

	dec := aesDecrypt(strings.TrimSpace(string(buf)))
	var result map[string]interface{}
	json.Unmarshal([]byte(dec), &result)
	return result
}

func getVodInfo(vodId string, audioType int) map[string]interface{} {
	token := fetchToken()
	curTime := strconv.FormatInt(time.Now().UnixMilli(), 10)
	buf := httpsPost(
		"/api/vod/info_new",
		map[string]string{
			"sign":       generateP2pToken(vodId, curTime),
			"vod_id":     vodId,
			"cur_time":   curTime,
			"audio_type": strconv.Itoa(audioType),
		},
		buildHeaders(curTime, token),
	)
	if len(buf) == 0 {
		return nil
	}

	dec := aesDecrypt(strings.TrimSpace(string(buf)))
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(dec), &result); err != nil {
		return nil
	}

	if resObj, ok := result["result"].(map[string]interface{}); ok {
		if collections, ok := resObj["vod_collection"].([]interface{}); ok {
			for _, epIntf := range collections {
				if ep, ok := epIntf.(map[string]interface{}); ok {
					rawUrl, _ := ep["vod_url"].(string)
					if rawUrl == "" {
						rawUrl, _ = ep["down_url"].(string)
					}
					ep["raw_url"] = rawUrl
					ep["signed_url"] = signVideoUrl(rawUrl)
					if vUrl, ok := ep["vod_url"].(string); ok && vUrl != "" {
						ep["vod_url"] = signVideoUrl(vUrl)
					}
					if dUrl, ok := ep["down_url"].(string); ok && dUrl != "" {
						ep["down_url"] = signVideoUrl(dUrl)
					}
				}
			}
		}
	}
	return result
}

func httpGet(urlStr string, headers map[string]string) string {
	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", UserAgent)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return string(body)
}

func parseTmdbMap(data map[string]interface{}, mediaType string) *TmdbInfo {
	idVal, _ := data["id"].(float64)
	
	titleVal, _ := data["title"].(string)
	if titleVal == "" { titleVal, _ = data["name"].(string) }
	if titleVal == "" { titleVal, _ = data["original_title"].(string) }
	if titleVal == "" { titleVal, _ = data["original_name"].(string) }

	dateVal, _ := data["release_date"].(string)
	if dateVal == "" { dateVal, _ = data["first_air_date"].(string) }
	
	yearVal := dateVal
	if strings.Contains(dateVal, "-") {
		yearVal = strings.Split(dateVal, "-")[0]
	}

	return &TmdbInfo{
		ID:    int(idVal),
		Title: titleVal,
		Year:  yearVal,
		Type:  mediaType,
	}
}

func fetchTmdbDetails(id string, mediaType string) *TmdbInfo {
	if strings.HasPrefix(id, "tt") {
		findURL := fmt.Sprintf("https://api.themoviedb.org/3/find/%s?api_key=%s&external_source=imdb_id", id, TMDB_KEY)
		jsonStr := httpGet(findURL, nil)
		if jsonStr != "" {
			var findRes map[string]interface{}
			json.Unmarshal([]byte(jsonStr), &findRes)
			
			tvResults, _ := findRes["tv_results"].([]interface{})
			movieResults, _ := findRes["movie_results"].([]interface{})
			
			if mediaType == "tv" && len(tvResults) > 0 {
				return parseTmdbMap(tvResults[0].(map[string]interface{}), "tv")
			} else if len(movieResults) > 0 {
				return parseTmdbMap(movieResults[0].(map[string]interface{}), "movie")
			} else if len(tvResults) > 0 {
				return parseTmdbMap(tvResults[0].(map[string]interface{}), "tv")
			}
		}
	}
	
	if mediaType == "movie" || mediaType == "tv" {
		reqURL := fmt.Sprintf("https://api.themoviedb.org/3/%s/%s?api_key=%s", mediaType, id, TMDB_KEY)
		jsonStr := httpGet(reqURL, nil)
		if jsonStr != "" {
			var data map[string]interface{}
			json.Unmarshal([]byte(jsonStr), &data)
			return parseTmdbMap(data, mediaType)
		}
	}
	
	return nil
}

func addCorsHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, HEAD")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	w.Header().Set("Access-Control-Expose-Headers", "*")
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		addCorsHeaders(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	addCorsHeaders(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><title>NanoHTTPD Go Backend</title></head>
<body style="font-family:sans-serif;padding:20px;">
    <h1>NanoHTTPD Go Backend Server</h1>
    <p>Status: <b>Running</b> on port %d</p>
</body>
</html>`, DefaultPort)
	w.Write([]byte(html))
}

func handleTmdb(w http.ResponseWriter, r *http.Request) {
	addCorsHeaders(w)
	w.Header().Set("Content-Type", "application/json")
	id := strings.TrimPrefix(r.URL.Path, "/api/tmdb/movie/")
	if id == "" {
		id = "550"
	}
	details := fetchTmdbDetails(id, "movie")
	json.NewEncoder(w).Encode(details)
}

func handleTest(w http.ResponseWriter, r *http.Request) {
    addCorsHeaders(w)
	w.Header().Set("Content-Type", "application/json")
    vodId := "1466695562"
    vodDetails := getVodInfo(vodId, 0)
    json.NewEncoder(w).Encode(vodDetails)
}

func handleMovie(w http.ResponseWriter, r *http.Request) {
    addCorsHeaders(w)
    w.Header().Set("Content-Type", "application/json")
    
    id := strings.TrimPrefix(r.URL.Path, "/api/movie/")
    if id == "" {
        id = "550"
    }

    // 1. Get TMDB Details
    tmdb := fetchTmdbDetails(id, "movie")
    if tmdb == nil {
        w.Write([]byte(`{"error": "TMDB info not found"}`))
        return
    }

    // 2. Search for the title
    res := apiPost("/api/search/result", map[string]string{"kw": tmdb.Title, "pn": "1"})
    
    var vodId string
    if res != nil {
        if items, ok := res["result"].([]interface{}); ok && len(items) > 0 {
            // Find best match (simplified to first item)
            if item, ok := items[0].(map[string]interface{}); ok {
                if idFloat, ok := item["id"].(float64); ok {
                    vodId = strconv.FormatFloat(idFloat, 'f', -1, 64)
                } else if idStr, ok := item["id"].(string); ok {
                    vodId = idStr
                }
            }
        } else if result, ok := res["result"].(map[string]interface{}); ok {
            if items, ok := result["items"].([]interface{}); ok && len(items) > 0 {
                if item, ok := items[0].(map[string]interface{}); ok {
                    if idFloat, ok := item["id"].(float64); ok {
                        vodId = strconv.FormatFloat(idFloat, 'f', -1, 64)
                    } else if idStr, ok := item["id"].(string); ok {
                        vodId = idStr
                    }
                }
            }
        }
    }

    if vodId == "" {
        w.Write([]byte(`{"error": "Video not found on server"}`))
        return
    }

    // 3. Get Vod Info and extract signed URL
    vodDetails := getVodInfo(vodId, 0)
    var signedUrl string
    if vodDetails != nil {
        if result, ok := vodDetails["result"].(map[string]interface{}); ok {
            if collections, ok := result["vod_collection"].([]interface{}); ok && len(collections) > 0 {
                if ep, ok := collections[0].(map[string]interface{}); ok {
                    if url, ok := ep["signed_url"].(string); ok {
                        signedUrl = url
                    }
                }
            }
        }
    }

    // 4. Return simplified JSON
    response := map[string]string{
        "id": id,
        "name": tmdb.Title,
        "url": signedUrl,
    }
    json.NewEncoder(w).Encode(response)
}

func handleTv(w http.ResponseWriter, r *http.Request) {
    addCorsHeaders(w)
    w.Header().Set("Content-Type", "application/json")
    
    // Path looks like /api/tv/6678/1/6
    parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/tv/"), "/")
    if len(parts) < 3 {
        w.Write([]byte(`{"error": "Invalid format, use /api/tv/{id}/{season}/{episode}"}`))
        return
    }
    
    id := parts[0]
    season := parts[1]
    episode, _ := strconv.Atoi(parts[2])

    // 1. Get TMDB Details
    tmdb := fetchTmdbDetails(id, "tv")
    if tmdb == nil {
        w.Write([]byte(`{"error": "TMDB info not found"}`))
        return
    }

    // 2. Search for the title + season
    fmt.Printf("TMDB Title: %s\n", tmdb.Title)
    searchQuery := fmt.Sprintf("%s Season %s", tmdb.Title, season)
    res := apiPost("/api/search/result", map[string]string{"kw": searchQuery, "pn": "1"})
    
    var vodId string
    extractVodId := func(response map[string]interface{}) string {
        if response != nil {
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
        }
        return ""
    }

    vodId = extractVodId(res)
    
    // Fallback search to just the title
    if vodId == "" {
        res = apiPost("/api/search/result", map[string]string{"kw": tmdb.Title, "pn": "1"})
        vodId = extractVodId(res)
    }

    if vodId == "" {
        w.Write([]byte(`{"error": "Video not found on server"}`))
        return
    }

    // 3. Get Vod Info and extract signed URL for the specific episode
    vodDetails := getVodInfo(vodId, 0)
    var signedUrl string
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
                            if url, ok := ep["signed_url"].(string); ok {
                                signedUrl = url
                                break
                            }
                        }
                    }
                }
            }
        }
    }

    if signedUrl == "" {
        w.Write([]byte(`{"error": "Episode not found"}`))
        return
    }

    // 4. Return simplified JSON
    response := map[string]string{
        "id": id,
        "name": tmdb.Title,
        "season": season,
        "episode": strconv.Itoa(episode),
        "url": signedUrl,
    }
    json.NewEncoder(w).Encode(response)
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleRoot)
	mux.HandleFunc("/api/tmdb/movie/", handleTmdb)
    mux.HandleFunc("/api/movie/", handleMovie)
    mux.HandleFunc("/api/tv/", handleTv)


	mux.HandleFunc("/api/test", handleTest)
	
	log.Printf("Server listening on port %d", DefaultPort)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", DefaultPort), mux); err != nil {
		log.Fatal(err)
	}
}
