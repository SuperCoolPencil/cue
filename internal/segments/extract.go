package segments

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"strconv"
	"time"
)

type limitedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.Len() {
		return 0, fmt.Errorf("fingerprint exceeds limit")
	}
	return b.Buffer.Write(p)
}

// Extract passes only a private loopback URL to FFmpeg, keeping authenticated
// server URLs out of subprocess arguments and diagnostic output. Range requests
// are forwarded so remote media remains seekable. The caller owns cancellation.
func Extract(ctx context.Context, source string, offsetMs int64, seconds, audioTrack int) ([]uint32, error) {
	if seconds < 30 || seconds > 900 || audioTrack < 0 || offsetMs < 0 {
		return nil, fmt.Errorf("invalid analysis window or audio track")
	}
	parsed, err := url.Parse(source)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("analysis requires an HTTP media source")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("cannot start analysis proxy")
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		_ = listener.Close()
		return nil, err
	}
	proxyPath := "/" + hex.EncodeToString(nonce)
	client := &http.Client{Timeout: 5 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != proxyPath {
			http.NotFound(w, r)
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		req, err := http.NewRequestWithContext(ctx, r.Method, source, nil)
		if err != nil {
			http.Error(w, "source unavailable", http.StatusBadGateway)
			return
		}
		for _, key := range []string{"Range", "If-Range"} {
			if v := r.Header.Get(key); v != "" {
				req.Header.Set(key, v)
			}
		}
		response, err := client.Do(req)
		if err != nil {
			http.Error(w, "source unavailable", http.StatusBadGateway)
			return
		}
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != 200 && response.StatusCode != 206 {
			http.Error(w, "source unavailable", http.StatusBadGateway)
			return
		}
		for _, key := range []string{"Content-Length", "Content-Range", "Accept-Ranges", "Content-Type"} {
			if v := response.Header.Get(key); v != "" {
				w.Header().Set(key, v)
			}
		}
		w.WriteHeader(response.StatusCode)
		if r.Method != "HEAD" {
			_, _ = io.Copy(w, response.Body)
		}
	})}
	go func() { _ = server.Serve(listener) }()
	defer func() { _ = server.Close() }()
	cmd := exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-v", "error", "-threads", "1", "-ss", strconv.FormatFloat(float64(offsetMs)/1000, 'f', 3, 64), "-i", "http://"+listener.Addr().String()+proxyPath, "-t", strconv.Itoa(seconds), "-map", fmt.Sprintf("0:a:%d", audioTrack), "-ac", "1", "-ar", "11025", "-f", "chromaprint", "-algorithm", "1", "-fp_format", "raw", "pipe:1")
	output := &limitedBuffer{limit: 128 * 1024}
	cmd.Stdout = output
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("FFmpeg fingerprint extraction failed (check Chromaprint support and audio track)")
	}
	b := output.Bytes()
	if len(b) == 0 || len(b)%4 != 0 {
		return nil, fmt.Errorf("invalid raw fingerprint")
	}
	fp := make([]uint32, len(b)/4)
	for i := range fp {
		fp[i] = binary.NativeEndian.Uint32(b[i*4:])
	}
	return fp, nil
}
