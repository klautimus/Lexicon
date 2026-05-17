package main

import (
	"context"
	"embed"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/joho/godotenv"

	"github.com/kevin/lexicon/internal/analytics"
	"github.com/kevin/lexicon/internal/auth"
	"github.com/kevin/lexicon/internal/config"
	"github.com/kevin/lexicon/internal/db"
	"github.com/kevin/lexicon/internal/downloader"
	"github.com/kevin/lexicon/internal/history"
	"github.com/kevin/lexicon/internal/library"
	"github.com/kevin/lexicon/internal/playlists"
	"github.com/kevin/lexicon/internal/recommender"
	"github.com/kevin/lexicon/internal/scanner"
	"github.com/kevin/lexicon/internal/spotify"
	"github.com/kevin/lexicon/internal/streamer"
	"github.com/kevin/lexicon/internal/websearch"
)

//go:embed all:dist
var distFS embed.FS

func main() {
	_ = godotenv.Load()

	cfg := config.Load()
	if cfg.SpotifyFrontendURL == "" {
		cfg.SpotifyFrontendURL = "http://127.0.0.1:" + cfg.Port
	}

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("db open: %v", err)
	}
	defer database.Close()

	if err := db.Migrate(database); err != nil {
		log.Fatalf("db migrate: %v", err)
	}

	scn := scanner.New(database)
	libAPI := library.New(database)
	playlistAPI := playlists.New(database)
	strm := streamer.New(database)
	hist := history.New(database)
	analyt := analytics.New(database, cfg.Timezone)
	ws := websearch.New(cfg.WebSearchEnabled)
	rec := recommender.New(database, recommender.DeepSeekConfig{
		APIKey:   cfg.DeepSeekAPIKey,
		Model:    cfg.DeepSeekModel,
		Thinking: cfg.DeepSeekThinking,
		BaseURL:  cfg.DeepSeekBaseURL,
	}, ws)
	spotifyAPI := spotify.New(database, spotify.Config{
		ClientID:    cfg.SpotifyClientID,
		RedirectURI: cfg.SpotifyRedirectURI,
		FrontendURL: cfg.SpotifyFrontendURL,
	})

	// Helper closure used by both the rescan endpoint and the downloader
	doRescan := func() {
		roots := strings.Split(cfg.MediaRoots, ";")
		for _, root := range roots {
			root = strings.TrimSpace(root)
			if root == "" {
				continue
			}
			if err := scn.ScanRoot(context.Background(), root); err != nil {
				log.Printf("[scanner] %s: %v", root, err)
			}
		}
	}

	// SpotiFLAC downloader. Output dir falls back to first MEDIA_ROOTS entry.
	dlOutput := cfg.SpotiflacOutput
	if dlOutput == "" {
		for _, r := range strings.Split(cfg.MediaRoots, ";") {
			if r = strings.TrimSpace(r); r != "" {
				dlOutput = r
				break
			}
		}
	}
	dlAPI := downloader.New(downloader.Config{
		Bin:                 cfg.SpotiflacBin,
		Output:              dlOutput,
		FolderFormat:        cfg.SpotiflacFolderFmt,
		SpotdlBin:           cfg.SpotdlBin,
		SpotdlFormat:        cfg.SpotdlFormat,
		SpotdlAudio:         cfg.SpotdlAudio,
		SpotifyClientID:     cfg.SpotifyClientID,
		SpotifyClientSecret: cfg.SpotifyClientSecret,
		YtdlpBin:            cfg.YtdlpBin,
		YtdlpFormat:         cfg.YtdlpFormat,
		FfmpegBin:           cfg.FfmpegBin,
		DeepSeekAPIKey:      cfg.DeepSeekAPIKey,
		DeepSeekModel:       cfg.DeepSeekModel,
		DeepSeekThinking:    cfg.DeepSeekThinking,
		DeepSeekBaseURL:     "",
		DownloadConcurrency: cfg.DownloadConcurrency,
	}, database, doRescan)

	// Initial scan in background
	go func() {
		roots := strings.Split(cfg.MediaRoots, ";")
		for _, r := range roots {
			r = strings.TrimSpace(r)
			if r == "" {
				continue
			}
			log.Printf("[scanner] scanning %s", r)
			if err := scn.ScanRoot(context.Background(), r); err != nil {
				log.Printf("[scanner] %s: %v", r, err)
			}
		}
		log.Printf("[scanner] initial scan complete")
	}()

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// CORS: always enabled
	corsOrigins := []string{"http://localhost:5173", "http://127.0.0.1:5173"}
	if origins := os.Getenv("CORS_ORIGINS"); origins != "" {
		corsOrigins = strings.Split(origins, ",")
		for i := range corsOrigins {
			corsOrigins[i] = strings.TrimSpace(corsOrigins[i])
		}
	}
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   corsOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
	}))

	// API key auth for write operations (POST/PUT/DELETE)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "POST" || r.Method == "PUT" || r.Method == "DELETE" {
				auth.RequireAPIKey(next).ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	})

	r.Get("/api/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"ok":true}`))
	})

	libAPI.Mount(r)
	playlistAPI.Mount(r)
	strm.Mount(r)
	hist.Mount(r)
	analyt.Mount(r)
	rec.Mount(r)
	spotifyAPI.Mount(r)
	dlAPI.Mount(r)

	// Start Spotify background syncer (no-op if SPOTIFY_CLIENT_ID empty or not connected)
	spotifyAPI.Syncer().Start(context.Background())

	// Trigger rescan endpoint
	r.Post("/api/scan", func(w http.ResponseWriter, _ *http.Request) {
		go doRescan()
		w.Write([]byte(`{"started":true}`))
	})

	// Serve embedded frontend static files (SPA catch-all)
	fileServer := http.FileServer(http.FS(distFS))
	staticHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		path := "/dist" + req.URL.Path
		_, err := distFS.Open(strings.TrimPrefix(path, "/"))
		if err != nil {
			path = "/dist/"
		}
		req.URL.Path = path
		fileServer.ServeHTTP(w, req)
	})

	// Parent handler: API routes → chi router, everything else → static files
	parentHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasPrefix(req.URL.Path, "/api/") {
			r.ServeHTTP(w, req)
			return
		}
		staticHandler.ServeHTTP(w, req)
	})

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           parentHandler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	if cfg.SpotifyClientID != "" {
		log.Printf("[spotify] redirect_uri=%s", cfg.SpotifyRedirectURI)
	}

	go func() {
		log.Printf("[lexicon] listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Printf("[lexicon] shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
}
