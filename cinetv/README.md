# NanoServer Go Backend

A lightweight backend API written in Go that resolves TMDB IDs (for both Movies and TV Shows) into signed, playable MP4/M3U8 streaming URLs. The server automatically searches the upstream streaming provider, handles token generation, payload decryption (AES/DES), and extracts the correct stream for a requested episode or movie.

## Prerequisites

- **Go (Golang)**: Make sure you have Go installed on your system. (e.g., `sudo apt install golang-go` on Ubuntu, or download from [golang.org](https://golang.org/)).

## Running the Server

1. Open your terminal in the directory containing `main.go`.
2. Run the following command to start the server:

```bash
go run main.go
```

3. The server will start and listen on port **1937** by default.

---

## API Endpoints & Usage

### 1. Movie Stream API
Fetch the stream URL for a specific movie using its TMDB ID.

**Endpoint:**
`GET /api/movie/{tmdb_id}`

**Example Request:**
```bash
# Fetch details and stream for "Fight Club" (TMDB ID: 550)
curl -s http://localhost:1937/api/movie/550
```

**Example Response:**
```json
{
  "id": "550",
  "name": "Fight Club",
  "url": "http://758d5c9d4a019e.e6r4r1.com/vod/1/2025/04/03/604be407c1fe/1135529346.mp4?wsSecret=40d9c86fd741f4a1740a43d548c3b0da&wsTime=6ac0e711"
}
```

### 2. TV Show Stream API
Fetch the stream URL for a specific TV show episode using its TMDB ID, Season number, and Episode number.

**Endpoint:**
`GET /api/tv/{tmdb_id}/{season_number}/{episode_number}`

**Example Request:**
```bash
# Fetch Season 1, Episode 6 of "Game of Thrones" (TMDB ID: 1399)
curl -s http://localhost:1937/api/tv/1399/1/6
```

**Example Response:**
```json
{
  "episode": "6",
  "id": "1399",
  "name": "Game of Thrones",
  "season": "1",
  "url": "http://7590e5287be439.e6r4r1.com/vod/1/2026/06/18/60565ddb818f/534473569.mp4?wsSecret=ee555d1b33054efe46bdef97bf9cd8ce&wsTime=6ac0e7b3"
}
```

### Notes / Troubleshooting
- **Video not found on server**: If the upstream provider does not host the movie or TV show episode, the API will return `{"error": "Video not found on server"}`.
- **Signed URLs**: The `url` returned contains temporary authentication tokens (`wsSecret` and `wsTime`) required for streaming. Do not cache the URLs for long periods, as the tokens expire. Retrieve a fresh URL using the API immediately before playback.
