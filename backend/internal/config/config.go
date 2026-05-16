package config

import (
\t"os"
\t"strconv"
)

type Config struct {
\tPort               string
\tDBPath             string
\tMediaRoots         string
\tDeepSeekAPIKey     string
\tDeepSeekModel      string
\tDeepSeekThinking   string
\tDeepSeekBaseURL    string
\tSpotifyClientID     string
\tSpotifyClientSecret string
\tSpotifyRedirectURI  string
\tSpotifyFrontendURL  string
\tSpotiflacBin       string
\tSpotiflacOutput    string
\tSpotiflacFolderFmt string
\tSpotdlBin          string
\tSpotdlFormat       string
\tSpotdlAudio        string
\tYtdlpBin           string
\tYtdlpFormat        string
\tFfmpegBin          string
\tDownloadConcurrency int
}

func Load() Config {
\tcfg := Config{
\t\tPort:               env("PORT", "8787"),
\t\tDBPath:             env("DB_PATH", "./data/lexicon.db"),
\t\tMediaRoots:         env("MEDIA_ROOTS", ""),
\t\tDeepSeekAPIKey:     env("DEEPSEEK_API_KEY", ""),
\t\tDeepSeekModel:      env("DEEPSEEK_MODEL", "deepseek-v4-flash"),
\t\tDeepSeekThinking:   env("DEEPSEEK_THINKING", "medium"),
\t\tDeepSeekBaseURL:    env("DEEPSEEK_BASE_URL", "https://api.deepseek.com"),
\t\tSpotifyClientID:     env("SPOTIFY_CLIENT_ID", ""),
\t\tSpotifyClientSecret: env("SPOTIFY_CLIENT_SECRET", ""),
\t\tSpotifyRedirectURI:  env("SPOTIFY_REDIRECT_URI", "http://localhost:8787/api/spotify/callback"),
\t\tSpotifyFrontendURL:  env("SPOTIFY_FRONTEND_URL", ""),
\t\tSpotiflacBin:       env("SPOTIFLAC_BIN", ""),
\t\tSpotdlBin:          env("SPOTDL_BIN", ""),
\t\tSpotdlFormat:       env("SPOTDL_FORMAT", "mp3"),
\t\tSpotiflacOutput:    env("SPOTIFLAC_OUTPUT", ""),
\t\tSpotiflacFolderFmt: env("SPOTIFLAC_FOLDER_FORMAT", ""),
\t\tSpotdlAudio:        env("SPOTDL_AUDIO_PROVIDERS", "piped,youtube,soundcloud,bandcamp"),
\t\tYtdlpBin:           env("YTDLP_BIN", ""),
\t\tYtdlpFormat:        env("YTDLP_FORMAT", "mp3"),
\t\tFfmpegBin:          env("FFMPEG_BIN", ""),
\t}

\tif v := os.Getenv("DOWNLOAD_CONCURRENCY"); v != "" {
\t\tif n, err := strconv.Atoi(v); err == nil && n > 0 {
\t\t\tcfg.DownloadConcurrency = n
\t\t}
\t}
\tif cfg.DownloadConcurrency <= 0 {
\t\tcfg.DownloadConcurrency = 2
\t}

\treturn cfg
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
