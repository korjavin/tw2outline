package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/korjavin/tw2outline/internal/config"
	"github.com/korjavin/tw2outline/internal/logger"
	"github.com/korjavin/tw2outline/internal/ntfy"
	"github.com/korjavin/tw2outline/internal/outline"
	"github.com/korjavin/tw2outline/internal/scheduler"
	"github.com/korjavin/tw2outline/internal/storage"
	"github.com/korjavin/tw2outline/internal/twitter"
)

// App holds the application's dependencies.
type App struct {
	Config    *config.Config
	Logger    *logger.Logger
	Storage   storage.Storage
	Outline   *outline.Client
	Twitter   twitter.Client
	Scheduler scheduler.Scheduler
	Metrics   *Metrics
	Mux       *http.ServeMux
	Ntfy      ntfy.Client
	server    *http.Server
}

// New creates a new App.
func New() (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %v", err)
	}

	log := logger.New(cfg.LogLevel)
	log.Info("Log level set to: %s", cfg.LogLevel)

	store, err := storage.NewFileStorage(cfg.CacheFilePath, log)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %v", err)
	}

	outlineClient := outline.NewClient(cfg.OutlineURL, cfg.OutlineToken)
	mux := http.NewServeMux()
	metrics := NewMetrics(cfg.CheckInterval)

	// Register dashboard routes before starting the server.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(getDashboardHTML(metrics.GetSafeCopy())))
	})
	mux.HandleFunc("/api/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		jsonData, err := json.MarshalIndent(metrics.GetSafeCopy(), "", "  ")
		if err != nil {
			http.Error(w, "Failed to encode metrics", http.StatusInternalServerError)
			return
		}
		w.Write(jsonData)
	})

	// Start the HTTP server BEFORE initialising the Twitter client.
	// twitter.NewClient() blocks waiting for the OAuth /callback redirect when
	// no token file exists, so the server must already be listening when the
	// user visits the Twitter auth URL.
	port := cfg.CallbackPort
	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}
	go func() {
		log.Info("Starting web server on http://localhost:%s", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("Web server error: %v", err)
		}
	}()

	twitterClient, err := twitter.NewClient(cfg, log, mux)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Twitter client: %v", err)
	}

	ntfyClient := ntfy.NewClient(cfg.NtfyServer, cfg.NtfyTopic, cfg.NtfyUsername, cfg.NtfyPassword, log)

	app := &App{
		Config:  cfg,
		Logger:  log,
		Storage: store,
		Outline: outlineClient,
		Twitter: twitterClient,
		Metrics: metrics,
		Mux:     mux,
		Ntfy:    ntfyClient,
		server:  server,
	}

	app.Scheduler = scheduler.NewSimpleScheduler(cfg.CheckInterval, app.processBookmarks, log)

	return app, nil
}

// Run starts the application.
func (a *App) Run() {
	a.Logger.Info("Starting application")

	// Cleanup processed bookmarks if requested
	if a.Config.CleanupProcessedBookmarks {
		a.Logger.Info("Cleanup mode enabled - removing already processed bookmarks")
		if err := a.Twitter.CleanupProcessedBookmarks(a.Storage); err != nil {
			a.Logger.Error("Cleanup failed: %v", err)
		} else {
			a.Logger.Info("Cleanup completed successfully")
		}
		if err := a.Storage.Save(); err != nil {
			a.Logger.Error("Error saving cache after cleanup: %v", err)
		}
	}

	go a.Scheduler.Start()

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	a.Logger.Info("Shutting down...")
	a.Scheduler.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.server.Shutdown(ctx); err != nil {
		a.Logger.Error("Web server shutdown error: %v", err)
	}

	a.Logger.Info("Application stopped")
}

// generateTitle picks the first non-empty line of the tweet text, trimmed to 80 chars.
func generateTitle(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return "Twitter bookmark"
	}
	line := strings.TrimSpace(strings.SplitN(text, "\n", 2)[0])
	if line == "" {
		return "Twitter bookmark"
	}
	if len(line) > 80 {
		line = line[:80] + "..."
	}
	return line
}

func (a *App) processBookmarks() {
	a.Logger.Info("Starting to process bookmarks")
	a.Metrics.UpdateStatus("Processing")

	tweets, err := a.Twitter.GetBookmarks()
	if err != nil {
		a.Logger.Error("failed to get bookmarks: %v", err)
		a.Metrics.RecordError(err.Error())
		a.Metrics.UpdateStatus("Error")
		return
	}

	a.Logger.Info("Found %d bookmarked tweets", len(tweets))

	var processed, skipped, failed int
	for _, tweet := range tweets {
		if a.Storage.IsProcessed(tweet.ID) {
			skipped++
			continue
		}

		title := generateTitle(tweet.Text)
		body := fmt.Sprintf("%s\n\n[Source](%s)", tweet.Text, tweet.URL)

		if _, err := a.Outline.CreateDocument(a.Config.OutlineCollectionID, title, body); err != nil {
			a.Logger.Error("Error adding tweet %s to Outline: %v", tweet.ID, err)
			failed++
			continue
		}

		a.Storage.MarkProcessed(tweet.ID)
		processed++

		if err := a.Ntfy.Send(tweet.Text, "New Bookmark Saved to Outline"); err != nil {
			a.Logger.Warn("Failed to send ntfy notification for tweet %s: %v", tweet.ID, err)
		}

		if a.Config.RemoveBookmarks {
			if err := a.Twitter.RemoveBookmark(tweet.ID); err != nil {
				a.Logger.Warn("Failed to remove bookmark for tweet %s: %v", tweet.ID, err)
			}
		}
		time.Sleep(200 * time.Millisecond)
	}

	if err := a.Storage.Save(); err != nil {
		a.Logger.Error("Error saving cache: %v", err)
	}

	a.Metrics.RecordCheck(processed, processed, time.Now().Add(a.Config.CheckInterval))
	a.Metrics.UpdateStatus("Running")
	a.Logger.Info("Bookmark processing complete. Processed: %d, Skipped: %d, Failed: %d", processed, skipped, failed)
}

// Metrics holds application status and metrics.
type Metrics struct {
	mu                      sync.Mutex
	StartTime               time.Time
	LastCheckTime           *time.Time
	NextCheckTime           *time.Time
	Status                  string
	TotalBookmarksProcessed int
	TotalOutlineSaves       int
	LastError               string
	LastErrorTime           *time.Time
	CheckInterval           time.Duration
}

// NewMetrics creates a new Metrics struct.
func NewMetrics(checkInterval time.Duration) *Metrics {
	return &Metrics{
		StartTime:     time.Now(),
		Status:        "Starting",
		CheckInterval: checkInterval,
	}
}

func (m *Metrics) UpdateStatus(status string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Status = status
}

func (m *Metrics) RecordCheck(bookmarksProcessed, outlineSaves int, nextCheck time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	m.LastCheckTime = &now
	m.NextCheckTime = &nextCheck
	m.TotalBookmarksProcessed += bookmarksProcessed
	m.TotalOutlineSaves += outlineSaves
}

func (m *Metrics) RecordError(err string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	m.LastError = err
	m.LastErrorTime = &now
}

func (m *Metrics) GetSafeCopy() Metrics {
	m.mu.Lock()
	defer m.mu.Unlock()
	return *m
}



func getDashboardHTML(metrics Metrics) string {
	statusColor := "#22c55e"
	switch metrics.Status {
	case "Error":
		statusColor = "#ef4444"
	case "Processing":
		statusColor = "#f59e0b"
	case "Starting":
		statusColor = "#6366f1"
	}

	lastErr := metrics.LastError
	if lastErr == "" {
		lastErr = "None"
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <meta http-equiv="refresh" content="30">
    <title>tw2outline — Status</title>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: #0f172a;
            color: #e2e8f0;
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            padding: 2rem;
        }
        .card {
            background: #1e293b;
            border: 1px solid #334155;
            border-radius: 1rem;
            padding: 2rem;
            width: 100%%;
            max-width: 560px;
            box-shadow: 0 25px 50px -12px rgba(0,0,0,0.5);
        }
        h1 {
            font-size: 1.5rem;
            font-weight: 700;
            margin-bottom: 0.25rem;
            background: linear-gradient(135deg, #60a5fa, #a78bfa);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
        }
        .subtitle { color: #64748b; font-size: 0.85rem; margin-bottom: 2rem; }
        .badge {
            display: inline-flex;
            align-items: center;
            gap: 0.4rem;
            padding: 0.25rem 0.75rem;
            border-radius: 9999px;
            font-size: 0.8rem;
            font-weight: 600;
            background: rgba(255,255,255,0.05);
            border: 1px solid %s;
            color: %s;
        }
        .dot {
            width: 8px; height: 8px;
            border-radius: 50%%;
            background: %s;
            animation: pulse 2s infinite;
        }
        @keyframes pulse {
            0%%, 100%% { opacity: 1; }
            50%% { opacity: 0.4; }
        }
        .grid { display: grid; grid-template-columns: 1fr 1fr; gap: 1rem; margin-top: 1.5rem; }
        .stat {
            background: #0f172a;
            border: 1px solid #1e293b;
            border-radius: 0.75rem;
            padding: 1rem;
        }
        .stat-label { font-size: 0.75rem; color: #64748b; text-transform: uppercase; letter-spacing: 0.05em; margin-bottom: 0.4rem; }
        .stat-value { font-size: 1.25rem; font-weight: 700; color: #e2e8f0; }
        .error-box {
            margin-top: 1.5rem;
            padding: 0.75rem 1rem;
            background: rgba(239,68,68,0.1);
            border: 1px solid rgba(239,68,68,0.3);
            border-radius: 0.5rem;
            font-size: 0.8rem;
            color: #fca5a5;
            word-break: break-all;
        }
        .footer { margin-top: 1.5rem; font-size: 0.75rem; color: #475569; text-align: center; }
    </style>
</head>
<body>
<div class="card">
    <h1>tw2outline</h1>
    <p class="subtitle">Twitter Bookmarks → Outline documents</p>
    <div class="badge">
        <span class="dot"></span>
        %s
    </div>
    <div class="grid">
        <div class="stat">
            <div class="stat-label">Uptime</div>
            <div class="stat-value">%s</div>
        </div>
        <div class="stat">
            <div class="stat-label">Bookmarks saved</div>
            <div class="stat-value">%d</div>
        </div>
        <div class="stat">
            <div class="stat-label">Last check</div>
            <div class="stat-value" style="font-size:0.85rem">%s</div>
        </div>
        <div class="stat">
            <div class="stat-label">Next check</div>
            <div class="stat-value" style="font-size:0.85rem">%s</div>
        </div>
    </div>
    %s
    <p class="footer">Auto-refreshes every 30 s &nbsp;·&nbsp; <a href="/api/metrics" style="color:#60a5fa">JSON metrics</a></p>
</div>
</body>
</html>`,
		statusColor, statusColor, statusColor,
		metrics.Status,
		time.Since(metrics.StartTime).Round(time.Second).String(),
		metrics.TotalBookmarksProcessed,
		formatOptionalTime(metrics.LastCheckTime, "Never"),
		formatOptionalTime(metrics.NextCheckTime, "Not scheduled"),
		func() string {
			if metrics.LastError == "" || metrics.LastError == "None" {
				return ""
			}
			return fmt.Sprintf(`<div class="error-box">Last error: %s</div>`, lastErr)
		}(),
	)
}

func formatOptionalTime(t *time.Time, defaultStr string) string {
	if t == nil {
		return defaultStr
	}
	return t.Format("15:04:05")
}
