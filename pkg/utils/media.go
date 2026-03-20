package utils

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/media"
)

// IsAudioFile checks if a file is an audio file based on its filename extension and content type.
func IsAudioFile(filename, contentType string) bool {
	audioExtensions := []string{".mp3", ".wav", ".ogg", ".m4a", ".flac", ".aac", ".wma"}
	audioTypes := []string{"audio/", "application/ogg", "application/x-ogg"}

	for _, ext := range audioExtensions {
		if strings.HasSuffix(strings.ToLower(filename), ext) {
			return true
		}
	}

	for _, audioType := range audioTypes {
		if strings.HasPrefix(strings.ToLower(contentType), audioType) {
			return true
		}
	}

	return false
}

// SanitizeFilename removes potentially dangerous characters from a filename
// and returns a safe version for local filesystem storage.
func SanitizeFilename(filename string) string {
	// Get the base filename without path
	base := filepath.Base(filename)

	// Remove any directory traversal attempts
	base = strings.ReplaceAll(base, "..", "")
	base = strings.ReplaceAll(base, "/", "_")
	base = strings.ReplaceAll(base, "\\", "_")

	return base
}

// DownloadOptions holds optional parameters for downloading files
type DownloadOptions struct {
	Timeout      time.Duration
	IdleTimeout  time.Duration // max time to wait for a single chunk read
	ExtraHeaders map[string]string
	LoggerPrefix string
	ProxyURL     string
}

// DownloadFile downloads a file from URL to a local temp directory.
// Returns the local file path or empty string on error.
func DownloadFile(urlStr, filename string, opts DownloadOptions) string {
	// Set defaults
	if opts.Timeout == 0 {
		opts.Timeout = 60 * time.Second
	}
	if opts.IdleTimeout == 0 {
		opts.IdleTimeout = 30 * time.Second // Default 30s idle timeout
	}
	if opts.LoggerPrefix == "" {
		opts.LoggerPrefix = "utils"
	}

	mediaDir := media.TempDir()
	if err := os.MkdirAll(mediaDir, 0o700); err != nil {
		logger.ErrorCF(opts.LoggerPrefix, "Failed to create media directory", map[string]any{
			"error": err.Error(),
		})
		return ""
	}

	// Generate unique filename with UUID prefix to prevent conflicts
	safeName := SanitizeFilename(filename)
	localPath := filepath.Join(mediaDir, uuid.New().String()[:8]+"_"+safeName)

	// Create HTTP request
	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		logger.ErrorCF(opts.LoggerPrefix, "Failed to create download request", map[string]any{
			"error": err.Error(),
		})
		return ""
	}

	// Add extra headers (e.g., Authorization for Slack)
	for key, value := range opts.ExtraHeaders {
		req.Header.Set(key, value)
	}

	// Wrap in an idle-timeout reader to detect stalls mid-stream.
	// We use the request context as parent.
	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()

	req = req.WithContext(ctx)

	client := &http.Client{Timeout: opts.Timeout}
	if opts.ProxyURL != "" {
		proxyURL, parseErr := url.Parse(opts.ProxyURL)
		if parseErr != nil {
			logger.ErrorCF(opts.LoggerPrefix, "Invalid proxy URL for download", map[string]any{
				"error": parseErr.Error(),
				"proxy": opts.ProxyURL,
			})
			return ""
		}
		client.Transport = &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		logger.ErrorCF(opts.LoggerPrefix, "Failed to download file", map[string]any{
			"error": err.Error(),
			"url":   urlStr,
		})
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.ErrorCF(opts.LoggerPrefix, "File download returned non-200 status", map[string]any{
			"status": resp.StatusCode,
			"url":    urlStr,
		})
		return ""
	}

	out, err := os.Create(localPath)
	if err != nil {
		logger.ErrorCF(opts.LoggerPrefix, "Failed to create local file", map[string]any{
			"error": err.Error(),
		})
		return ""
	}
	defer out.Close()

	idleReader := &idleTimeoutReader{
		r:       resp.Body,
		timeout: opts.IdleTimeout,
		cancel:  cancel,
	}
	idleReader.start()
	defer idleReader.stop()

	if _, err := io.Copy(out, idleReader); err != nil {
		out.Close()
		os.Remove(localPath)
		logger.ErrorCF(opts.LoggerPrefix, "Failed to write file (possible stall or timeout)", map[string]any{
			"error": err.Error(),
		})
		return ""
	}

	logger.DebugCF(opts.LoggerPrefix, "File downloaded successfully", map[string]any{
		"path": localPath,
	})

	return localPath
}

// DownloadFileSimple is a simplified version of DownloadFile without options
func DownloadFileSimple(url, filename string) string {
	return DownloadFile(url, filename, DownloadOptions{
		LoggerPrefix: "media",
	})
}

// idleTimeoutReader cancels a context if no data is read for a specified duration.
type idleTimeoutReader struct {
	r       io.ReadCloser
	timeout time.Duration
	cancel  context.CancelFunc
	timer   *time.Timer
}

func (ir *idleTimeoutReader) start() {
	if ir.timeout > 0 {
		ir.timer = time.AfterFunc(ir.timeout, func() {
			ir.cancel()
		})
	}
}

func (ir *idleTimeoutReader) stop() {
	if ir.timer != nil {
		ir.timer.Stop()
	}
}

func (ir *idleTimeoutReader) Read(p []byte) (n int, err error) {
	if ir.timer != nil {
		ir.timer.Reset(ir.timeout)
	}
	return ir.r.Read(p)
}
