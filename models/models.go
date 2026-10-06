package models

// Playback holds playback identifiers for Vidara and Streamtape.
type Playback struct {
	VA string `json:"va"`
	ST string `json:"st"`
}

// EpisodeItem represents an episode in a TV season.
type EpisodeItem struct {
	Episode int      `json:"episode"`
	Play    Playback `json:"play"`
}

// MediaItem represents an item in the local Vidara database.
type MediaItem struct {
	ID       int           `json:"id"`
	Title    string        `json:"title"`
	Type     string        `json:"type"` // "movie" or "tv"
	Quality  string        `json:"quality,omitempty"`
	Season   int           `json:"season,omitempty"`
	Episodes []EpisodeItem `json:"episodes,omitempty"`
	Play     *Playback     `json:"play,omitempty"`
	VA       string        `json:"va,omitempty"`
	ST       string        `json:"st,omitempty"`
}

// ResolutionResult represents the final response from the 3-Tier resolver.
type ResolutionResult struct {
	Success bool      `json:"success"`
	TmdbID  int       `json:"tmdb_id"`
	Type    string    `json:"type,omitempty"`
	Tier    int       `json:"tier,omitempty"`
	Source  string    `json:"source,omitempty"`
	Title   string    `json:"title,omitempty"`
	Name    string    `json:"name,omitempty"`
	Season  *int              `json:"season,omitempty"`
	Episode *int              `json:"episode,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Play    *Playback         `json:"play,omitempty"`
	Error   string            `json:"error,omitempty"`
}

// AudioTrack represents dynamic audio track extracted from HLS / M3U.
type AudioTrack struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Language string `json:"language"`
	Default  bool   `json:"default"`
	URL      string `json:"url"`
}

// ExtractorResponse represents extraction responses from Vidara, Streamtape, or VidFast.
type ExtractorResponse struct {
	Status         string       `json:"status,omitempty"`
	Success        bool         `json:"success,omitempty"`
	Source         string       `json:"source,omitempty"`
	OriginalURL    string       `json:"original_url,omitempty"`
	URL            string       `json:"url,omitempty"`
	ProxyURL       string       `json:"proxy_url,omitempty"`
	DirectURL      string       `json:"direct_url,omitempty"`
	TapecontentURL string       `json:"tapecontent_url,omitempty"`
	StreamURL      string       `json:"stream_url,omitempty"`
	Text           string       `json:"text,omitempty"`
	Title          string       `json:"title,omitempty"`
	Thumbnail      *string      `json:"thumbnail,omitempty"`
	Subtitles      interface{}  `json:"subtitles,omitempty"`
	AudioTracks    []AudioTrack      `json:"audio_tracks,omitempty"`
	StreamType     string            `json:"stream_type,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
	Error          string            `json:"error,omitempty"`
	Message        string            `json:"message,omitempty"`
	ID             string            `json:"id,omitempty"`
}

// ResolveResponse represents dual resolver endpoint response (preserving app.py /api/resolve).
type ResolveResponse struct {
	Status      string             `json:"status"`
	Mode        string             `json:"mode"`
	STID        string             `json:"st_id"`
	VAID        string             `json:"va_id"`
	Primary     *ExtractorResponse `json:"primary,omitempty"`
	Secondary   *ExtractorResponse `json:"secondary,omitempty"`
	ST          *ExtractorResponse `json:"st,omitempty"`
	VA          *ExtractorResponse `json:"va,omitempty"`
	Thumbnail   *string            `json:"thumbnail,omitempty"`
	Subtitles   interface{}        `json:"subtitles,omitempty"`
	AudioTracks []AudioTrack       `json:"audio_tracks,omitempty"`
}
