package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port               string
	DBPath             string
	MediaRoots         string
	DeepSeekAPIKey     string
	DeepSeekModel      string
	DeepSeekThinking   string
	DeepSeekBaseURL    string
	SpotifyClientID     string
	SpotifyClientSecret string
	SpotifyRedirectURI  string
	SpotifyFrontendURL  string
	SpotiflacBin       string
	SpotiflacOutput    string
	SpotiflacFolderFmt string
	SpotdlBin          string
	SpotdlFormat       string
	SpotdlAudio        string
	YtdlpBin           string
	YtdlpFormat        string
	FfmpegBin          string
	DownloadConcurrency int
}

func Load() Config {
	cfg := Config{
		Port:               env("PORT", "8787"),
		DBPath:             env("DB_PATH", "./data/lexicon.db"),
		MediaRoots:         env("MEDIA_ROOTS", ""),
		DeepSeekAPIKey:     env("DEEPSEEK_API_KEY", ""),
		DeepSeekModel:      env("DEEPSEEK_MODEL", "deepseek-v4-flash"),
		DeepSeekThinking:   env("DEEPSEEK_THINKING", "medium"),
		DeepSeekBaseURL:    env("DEEPSEEK_BASE_URL", "https://api.deepseek.com"),
		SpotifyClientID:     env("SPOTIFY_CLIENT_ID", ""),
		SpotifyClientSecret: env("SPOTIFY_CLIENT_SECRET", ""),
		SpotifyRedirectURI:  env("SPOTIFY_REDIRECT_URI", "http://localhost:8787/api/spotify/callback"),
		SpotifyFrontendURL:  env("SPOTIFY_FRONTEND_URL", ""),
		SpotiflacBin:       env("SPOTIFLAC_BIN", ""),
		SpotdlBin:          env("SPOTDL_BIN", ""),
		SpotdlFormat:       env("SPOTDL_FORMAT", "mp3"),
		SpotiflacOutput:    env("SPOTIFLAC_OUTPUT", ""),
		SpotiflacFolderFmt: env("SPOTIFLAC_FOLDER_FORMAT", ""),
		SpotdlAudio:        env("SPOTDL_AUDIO_PROVIDERS", "piped,youtube,soundcloud,bandcamp"),
		YtdlpBin:           env("YTDLP_BIN", ""),
		YtdlpFormat:        env("YTDLP_FORMAT", "mp3"),
		FfmpegBin:          env("FFMPEG_BIN", ""),
	}

	if v := os.Getenv("DOWNLOAD_CONCURRENCY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.DownloadConcurrency = n
		}
	}
	if cfg.DownloadConcurrency <= 0 {
		cfg.DownloadConcurrency = 2
	}

	return cfg
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
