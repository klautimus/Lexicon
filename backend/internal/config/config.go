package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Port               string
	DBPath             string
	MediaRoots         string
	Timezone           string
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
	SpotiflacService   string
	SpotdlBin          string
	SpotdlFormat       string
	SpotdlAudio        string
	YtdlpBin           string
	YtdlpFormat        string
	FfmpegBin          string
	FfprobeBin         string
	DownloadConcurrency int
	WebSearchEnabled   bool
	PoddlBin           string
	PodcastDir         string
}

func Load() (Config, error) {
	cfg := Config{
		Port:               env("PORT", "8787"),
		DBPath:             env("DB_PATH", "./data/lexicon.db"),
		MediaRoots:         env("MEDIA_ROOTS", ""),
		Timezone:           env("TIMEZONE", "local"),
		DeepSeekAPIKey:     env("DEEPSEEK_API_KEY", ""),
		DeepSeekModel:      env("DEEPSEEK_MODEL", "deepseek-v4-flash"),
		DeepSeekThinking:   env("DEEPSEEK_THINKING", "medium"),
		DeepSeekBaseURL:    env("DEEPSEEK_BASE_URL", "https://api.deepseek.com"),
		SpotifyClientID:     env("SPOTIFY_CLIENT_ID", ""),
		SpotifyClientSecret: env("SPOTIFY_CLIENT_SECRET", ""),
		SpotifyRedirectURI:  env("SPOTIFY_REDIRECT_URI", "http://127.0.0.1:8787/api/spotify/callback"),
		SpotifyFrontendURL:  env("SPOTIFY_FRONTEND_URL", ""),
		SpotiflacBin:       env("SPOTIFLAC_BIN", ""),
		SpotdlBin:          env("SPOTDL_BIN", ""),
		SpotdlFormat:       env("SPOTDL_FORMAT", "mp3"),
		SpotiflacOutput:    env("SPOTIFLAC_OUTPUT", ""),
		SpotiflacFolderFmt: env("SPOTIFLAC_FOLDER_FORMAT", ""),
		SpotiflacService:   env("SPOTIFLAC_SERVICE", "qobuz"),
		SpotdlAudio:        env("SPOTDL_AUDIO_PROVIDERS", "piped,youtube,soundcloud,bandcamp"),
		YtdlpBin:           env("YTDLP_BIN", ""),
		YtdlpFormat:        env("YTDLP_FORMAT", "opus"),
		FfmpegBin:          env("FFMPEG_BIN", ""),
		FfprobeBin:         env("FFPROBE_BIN", ""),
		PoddlBin:           env("PODDL_BIN", ""),
		PodcastDir:         env("PODCAST_DIR", ""),
		WebSearchEnabled:   env("WEBSEARCH_ENABLED", "true") == "true",
	}

	if v := os.Getenv("DOWNLOAD_CONCURRENCY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.DownloadConcurrency = n
		}
	}
	if cfg.DownloadConcurrency <= 0 {
		cfg.DownloadConcurrency = 2
	}

	// Validate critical config values
	if cfg.Port == "" {
		return Config{}, fmt.Errorf("PORT cannot be empty")
	}
	if p, err := strconv.Atoi(cfg.Port); err != nil || p <= 0 || p > 65535 {
		return Config{}, fmt.Errorf("PORT must be a valid port number (1-65535), got: %s", cfg.Port)
	}
	if cfg.DBPath == "" {
		return Config{}, fmt.Errorf("DB_PATH cannot be empty")
	}
	if cfg.DeepSeekBaseURL == "" {
		return Config{}, fmt.Errorf("DEEPSEEK_BASE_URL cannot be empty")
	}

	return cfg, nil
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
