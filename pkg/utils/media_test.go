package utils

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDownloadFile_IdleTimeout(t *testing.T) {
	// A server that stalls mid-stream
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("start"))
		flusher, ok := w.(http.Flusher)
		if ok {
			flusher.Flush()
		}
		// Stall indefinitely
		time.Sleep(2 * time.Second)
		w.Write([]byte("end"))
	}))
	defer ts.Close()

	opts := DownloadOptions{
		IdleTimeout: 500 * time.Millisecond,
		Timeout:     5 * time.Second,
	}

	path := DownloadFile(ts.URL, "test.txt", opts)
	if path != "" {
		t.Fatal("Expected download to fail due to idle timeout, but it succeeded")
	}
}

func TestDownloadFile_Normal(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello world"))
	}))
	defer ts.Close()

	opts := DownloadOptions{
		IdleTimeout: 1 * time.Second,
		Timeout:     5 * time.Second,
	}

	path := DownloadFile(ts.URL, "hello.txt", opts)
	if path == "" {
		t.Fatal("Expected download to succeed, but it failed")
	}
}
