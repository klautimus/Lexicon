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
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/joho/godotenv"
	"github.com/skip2/go-qrcode"

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

	// Create Spotify API first so we can pass it to the recommender
	spotifyAPI := spotify.New(database, spotify.Config{
		ClientID:    cfg.SpotifyClientID,
		RedirectURI: cfg.SpotifyRedirectURI,
		FrontendURL: cfg.SpotifyFrontendURL,
	})

	rec := recommender.New(database, recommender.DeepSeekConfig{
		APIKey:   cfg.DeepSeekAPIKey,
		Model:    cfg.DeepSeekModel,
		Thinking: cfg.DeepSeekThinking,
		BaseURL:  cfg.DeepSeekBaseURL,
	}, ws, spotifyAPI)

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
		json.NewEncoder(w).Encode(info)
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

	go func() {
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
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
