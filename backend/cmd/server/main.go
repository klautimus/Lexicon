package main

import (
	"context"
	"embed"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/joho/godotenv"
	"github.com/skip2/go-qrcode"

	"github.com/kevin/lexicon/internal/analytics"
	"github.com/kevin/lexicon/internal/apple"
	"github.com/kevin/lexicon/internal/auth"
	"github.com/kevin/lexicon/internal/config"
	"github.com/kevin/lexicon/internal/db"
	"github.com/kevin/lexicon/internal/downloader"
	"github.com/kevin/lexicon/internal/history"
	"github.com/kevin/lexicon/internal/library"
	"github.com/kevin/lexicon/internal/playerws"
	"github.com/kevin/lexicon/internal/playlists"
	"github.com/kevin/lexicon/internal/podcaster"
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

	// Require LEXICON_API_KEY to be set before starting the server.
	// Without it, all write endpoints are completely unauthenticated.
	if !auth.KeyIsSet() {
		log.Fatalf("[lexicon] FATAL: LEXICON_API_KEY environment variable is not set. Set it before starting the server.")
	}
	if auth.KeyLen() < 16 {
		log.Printf("[lexicon] WARNING: LEXICON_API_KEY is only %d characters — use at least 16 characters for security", auth.KeyLen())
	}

	cfg := config.Load()
	if cfg.SpotifyFrontendURL == "" {
		cfg.SpotifyFrontendURL = "http://127.0.0.1:" + cfg.Port
	}

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("db open: %v", err)
	}
	// database.Close() is called explicitly in the shutdown handler
	// after all goroutines have finished, to avoid races.

	if err := db.Migrate(database); err != nil {
		log.Fatalf("db migrate: %v", err)
	}

	scn := scanner.New(database)

	// Compute podcast output directory early so it can be included in
	// the media roots for the streamer and cover handler's path checks.
	podcastOutput := cfg.PodcastDir
	if podcastOutput == "" {
		for _, r := range strings.Split(cfg.MediaRoots, ";") {
			if r = strings.TrimSpace(r); r != "" {
				podcastOutput = r
				break
			}
		}
	}

	// Build media roots: MEDIA_ROOTS + PODCAST_DIR (if not already included).
	mediaRoots := parseMediaRoots(cfg.MediaRoots)
	if podcastOutput != "" {
		found := false
		for _, r := range mediaRoots {
			if r == podcastOutput {
				found = true
				break
			}
		}
		if !found {
			mediaRoots = append(mediaRoots, filepath.Clean(podcastOutput))
		}
	}

	libAPI := library.New(database, mediaRoots)
	playlistAPI := playlists.New(database)

	// Streamer takes the same combined roots as the library so podcast files
	// (which may live outside MEDIA_ROOTS, e.g. under PODCAST_DIR in
	// Program Files) pass the path-traversal guard. DO NOT rebuild this
	// list independently — see plans/lexicon-fix-podcast-403-streamer-roots-regression.md.
	strm := streamer.New(database, strings.Join(mediaRoots, ";"))
	hist := history.New(database)
	analyt := analytics.New(database, cfg.Timezone)
	ws := websearch.New(cfg.WebSearchEnabled)

	// Create Spotify API first so we can pass it to the recommender
	spotifyAPI := spotify.New(database, spotify.Config{
		ClientID:     cfg.SpotifyClientID,
		ClientSecret: cfg.SpotifyClientSecret,
		RedirectURI:  cfg.SpotifyRedirectURI,
		FrontendURL:  cfg.SpotifyFrontendURL,
	})

	// Apple Music API. Credentials live in the DB (entered via Settings),
	// not in env. This is intentional — the user requested GUI-only setup.
	appleAPI := apple.New(database, apple.Config{AppName: "Lexicon"})

	rec := recommender.New(database, recommender.DeepSeekConfig{
		APIKey:   cfg.DeepSeekAPIKey,
		Model:    cfg.DeepSeekModel,
		Thinking: cfg.DeepSeekThinking,
		BaseURL:  cfg.DeepSeekBaseURL,
	}, ws, spotifyAPI, appleAPI)

	// rescan state: mutex + cancellable context so a new rescan cancels any
	// in-flight one. This prevents goroutine accumulation when /api/scan is
	// called repeatedly while a previous scan is still running.
	var (
		rescanMu       sync.Mutex
		rescanCancel   context.CancelFunc
		rescanGen      atomic.Int64 // generation counter; incremented on each new rescan
	)

	// shutdown coordination: WaitGroup tracks all long-lived background
	// goroutines so we can wait for them before closing the database.
	var (
		wg          sync.WaitGroup
		shutdownCtx context.Context
		shutdown    context.CancelFunc
	)
	shutdownCtx, shutdown = context.WithCancel(context.Background())

	// Cancellable context for the initial scan goroutine.
	initScanCtx, initScanCancel := context.WithCancel(context.Background())

	// Helper closure used by both the rescan endpoint and the downloader.
	// Only one rescan runs at a time; a new call cancels the previous one.
	doRescan := func() {
		rescanMu.Lock()
		// Cancel any in-flight rescan
		if rescanCancel != nil {
			rescanCancel()
		}
		ctx, cancel := context.WithCancel(context.Background())
		rescanCancel = cancel
		myGen := rescanGen.Add(1) - 1 // capture generation before increment
		rescanMu.Unlock()

		wg.Add(1)
		defer wg.Done()

		roots := strings.Split(cfg.MediaRoots, ";")
		// Always include the podcast output directory so downloaded episodes
		// get indexed into the tracks table and become searchable/playable.
		if podcastOutput != "" {
			found := false
			for _, r := range roots {
				if strings.TrimSpace(r) == podcastOutput {
					found = true
					break
				}
			}
			if !found {
				roots = append(roots, podcastOutput)
			}
		}
		for _, root := range roots {
			root = strings.TrimSpace(root)
			if root == "" {
				continue
			}
			if err := scn.ScanRoot(ctx, root); err != nil {
				log.Printf("[scanner] %s: %v", root, err)
			}
		}

		rescanMu.Lock()
		// Only clear if no new rescan has started since we began.
		if rescanGen.Load() == myGen+1 {
			rescanCancel = nil
		}
		rescanMu.Unlock()
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
	// Auto-detect ffprobe from ffmpeg path if not explicitly set
	ffprobeBin := cfg.FfprobeBin
	if ffprobeBin == "" && cfg.FfmpegBin != "" {
		ffprobeBin = strings.Replace(cfg.FfmpegBin, "ffmpeg.exe", "ffprobe.exe", 1)
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
		FfprobeBin:          ffprobeBin,
		DeepSeekAPIKey:      cfg.DeepSeekAPIKey,
		DeepSeekModel:       cfg.DeepSeekModel,
		DeepSeekThinking:    cfg.DeepSeekThinking,
		DeepSeekBaseURL:     "",
		DownloadConcurrency: cfg.DownloadConcurrency,
	}, database, doRescan)

	// Podcast manager
	podcastAPI := podcaster.New(database, podcaster.Config{
		PoddlBin:     cfg.PoddlBin,
		OutputDir:    podcastOutput,
		AutoDownload: true,
	}, doRescan, dlAPI)

	// WebSocket hub for multi-device playback control
	wsHub := playerws.New()
	wg.Add(1)
	go func() {
		defer wg.Done()
		wsHub.Run()
	}()

	// Initial scan in background — uses a cancellable context so shutdown
	// can abort it, and registers on the WaitGroup so db.Close() waits.
	wg.Add(1)
	go func() {
		defer wg.Done()
		roots := strings.Split(cfg.MediaRoots, ";")
		// Include podcast output directory in initial scan
		if podcastOutput != "" {
			found := false
			for _, r := range roots {
				if strings.TrimSpace(r) == podcastOutput {
					found = true
					break
				}
			}
			if !found {
				roots = append(roots, podcastOutput)
			}
		}
		for _, r := range roots {
			r = strings.TrimSpace(r)
			if r == "" {
				continue
			}
			log.Printf("[scanner] scanning %s", r)
			if err := scn.ScanRoot(initScanCtx, r); err != nil {
				log.Printf("[scanner] %s: %v", r, err)
			}
		}
		log.Printf("[scanner] initial scan complete")
	}()

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// CORS: dynamic private-network origin checker
	// Allows localhost, loopback, and all private/LAN IP ranges (192.168.x.x, 10.x.x.x, 172.16-31.x.x)
	isAllowedOrigin := func(r *http.Request, origin string) bool {
		if origin == "" {
			return true // non-browser clients (curl, etc.)
		}
		// Strip scheme for parsing
		host := origin
		if i := strings.Index(host, "://"); i >= 0 {
			host = host[i+3:]
		}
		// Strip port
		if i := strings.LastIndex(host, ":"); i >= 0 {
			host = host[:i]
		}
		// Always allow localhost/loopback
		if host == "localhost" || host == "127.0.0.1" || host == "::1" {
			return true
		}
		// Check private IP ranges
		ip := net.ParseIP(host)
		if ip == nil {
			return false
		}
		// 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, 127.0.0.0/8
		return ip.IsLoopback() || ip.IsPrivate()
	}

	r.Use(cors.Handler(cors.Options{
		AllowOriginFunc:  isAllowedOrigin,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
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

	// Network info endpoint — returns all detected IPs for LAN debugging
	r.Get("/api/network", func(w http.ResponseWriter, _ *http.Request) {
		type netInfo struct {
			LocalIP   string   `json:"local_ip"`
			AllIPs    []string `json:"all_ips"`
			PrivateIPs []string `json:"private_ips"`
		}
		info := netInfo{LocalIP: getLocalIP()}
		addrs, _ := net.InterfaceAddrs()
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
				if ipNet.IP.To4() != nil {
					ip := ipNet.IP.String()
					info.AllIPs = append(info.AllIPs, ip)
					if net.ParseIP(ip).IsPrivate() && !net.ParseIP(ip).IsLinkLocalUnicast() {
						info.PrivateIPs = append(info.PrivateIPs, ip)
					}
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(info); err != nil {
			log.Printf("[server] netinfo encode: %v", err)
		}
	})

	libAPI.Mount(r)
	playlistAPI.Mount(r)
	strm.Mount(r)
	hist.Mount(r)
	analyt.Mount(r)
	rec.Mount(r)
	spotifyAPI.Mount(r)
	dlAPI.Mount(r)
	podcastAPI.Mount(r)
	appleAPI.Mount(r)

	// Start Spotify background syncer (no-op if SPOTIFY_CLIENT_ID empty or not connected)
	spotifyAPI.StartSyncer()
	// Start Apple Music background syncer (no-op until credentials saved + user connects)
	appleAPI.StartSyncer()

	// Trigger rescan endpoint
	r.Post("/api/scan", func(w http.ResponseWriter, _ *http.Request) {
		go doRescan()
		w.Write([]byte(`{"started":true}`))
	})

	// WebSocket endpoint for multi-device playback control
	r.Get("/api/ws/player", wsHub.ServeHTTP)

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

	// QR code endpoint for mobile LAN connection
	r.Get("/api/qr", func(w http.ResponseWriter, r *http.Request) {
		localIP := getLocalIP()
		url := "http://" + localIP + ":" + cfg.Port
		png, err := qrcode.Encode(url, qrcode.Medium, 256)
		if err != nil {
			http.Error(w, "QR generation failed", 500)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.Write(png)
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

	wg.Add(1)
	go func() {
		defer wg.Done()
		localIP := getLocalIP()
		lanURL := "http://" + localIP + ":" + cfg.Port
		log.Printf("[lexicon] listening on :%s", cfg.Port)
		log.Printf("[lexicon] Local:   http://localhost:%s", cfg.Port)
		log.Printf("[lexicon] Network: %s", lanURL)
		log.Printf("[lexicon] QR code: %s/api/qr", lanURL)

		// Print ASCII QR code to terminal
		if qr, err := qrcode.New(lanURL, qrcode.Medium); err == nil {
			log.Printf("[lexicon] Scan to connect your phone:")
			bitmap := qr.Bitmap()
			for _, row := range bitmap {
				var line strings.Builder
				for _, cell := range row {
					if cell {
						line.WriteString("██")
					} else {
						line.WriteString("  ")
					}
				}
				log.Print(line.String())
			}
		}

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Printf("[lexicon] shutting down...")

	// Shut down HTTP server first to stop accepting new requests.
	srv.Shutdown(shutdownCtx)
	shutdown()

	// Shut down subsystems that have their own goroutines.
	dlAPI.Shutdown()
	podcastAPI.Shutdown()
	spotifyAPI.Shutdown()
	appleAPI.Shutdown()
	wsHub.Shutdown()

	// Cancel all background contexts to abort in-flight work.
	rescanMu.Lock()
	if rescanCancel != nil {
		rescanCancel()
	}
	rescanMu.Unlock()
	initScanCancel()

	// Wait for all background goroutines to finish, with a timeout.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		log.Printf("[lexicon] all goroutines stopped")
	case <-time.After(10 * time.Second):
		log.Printf("[lexicon] WARNING: shutdown timeout — some goroutines still running")
	}

	// Now safe to close the database — no in-flight queries.
	database.Close()
	log.Printf("[lexicon] shutdown complete")
}

// parseMediaRoots splits the semicolon-separated MEDIA_ROOTS env var into a slice.
func parseMediaRoots(raw string) []string {
	var roots []string
	for _, r := range strings.Split(raw, ";") {
		r = strings.TrimSpace(r)
		if r != "" {
			roots = append(roots, r)
		}
	}
	return roots
}

// getLocalIP returns the preferred outbound IP address of this machine.
// Prefers private LAN addresses (192.168.x.x, 10.x.x.x, 172.16-31.x.x).
// Explicitly excludes link-local (169.254.x.x) and other non-routable ranges.
// Falls back to 127.0.0.1 if no suitable address is found.
func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	var fallback string
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				ip := ipNet.IP
				// Skip link-local (169.254.x.x) — not routable on LAN
				if ip.IsLinkLocalUnicast() {
					continue
				}
				// Prefer private LAN addresses (192.168.x.x, 10.x.x.x, 172.16-31.x.x)
				if ip.IsPrivate() {
					return ip.String()
				}
				// Remember first non-loopback as fallback
				if fallback == "" {
					fallback = ip.String()
				}
			}
		}
	}
	if fallback != "" {
		return fallback
	}
	// Fallback: try to get the IP by connecting to a public address
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err == nil {
		defer conn.Close()
		if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
			return addr.IP.String()
		}
	}
	return "127.0.0.1"
}
