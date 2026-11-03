package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

type downloadSources struct {
	FilePath  string `json:"filePath"`
	FilePaths []struct {
		URL string `json:"url"`
	} `json:"filePaths"`
}

func parseDownloadURLs(body []byte) ([]string, error) {
	var sources downloadSources
	if err := json.Unmarshal(body, &sources); err != nil {
		return nil, fmt.Errorf("invalid download metadata")
	}
	urls := []string{}
	seen := map[string]bool{}
	add := func(candidate string) {
		parsed, err := url.Parse(candidate)
		if err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") && parsed.Host != "" && parsed.User == nil && !seen[candidate] {
			urls = append(urls, candidate)
			seen[candidate] = true
		}
	}
	for _, source := range sources.FilePaths {
		add(source.URL)
	}
	add(sources.FilePath)
	if len(urls) == 0 {
		return nil, fmt.Errorf("download metadata contains no valid URLs")
	}
	return urls, nil
}

func downloadRequest(ctx context.Context, method, source string, headers http.Header) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, source, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid download URL")
	}
	req.Header = headers.Clone()
	if req.Header == nil {
		req.Header = http.Header{}
	}
	req.Header.Del("Authorization")
	req.Header.Del("Proxy-Authorization")
	return req, nil
}

func safeDownloadError(err error) error {
	var requestErr *url.Error
	if errors.As(err, &requestErr) {
		return requestErr.Err
	}
	return err
}

func retryDownloadStatus(status int) bool {
	return status == http.StatusForbidden || status == http.StatusNotFound || status == http.StatusRequestTimeout || status >= 500
}

func selectReachableDownloadURL(ctx context.Context, client *http.Client, urls []string, headers http.Header) (string, error) {
	var failures []error
	for index, source := range urls {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		status, err := probeDownloadURL(ctx, client, source, headers)
		if err == nil && (status == http.StatusOK || status == http.StatusPartialContent) {
			return source, nil
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if err == nil {
			err = fmt.Errorf("HTTP %d", status)
		}
		failures = append(failures, fmt.Errorf("source %d: %w", index+1, err))
		if status == http.StatusTooManyRequests {
			break
		}
	}
	if len(failures) == 0 {
		return "", fmt.Errorf("no download URLs")
	}
	return "", fmt.Errorf("no reachable download source: %w", errors.Join(failures...))
}

func probeDownloadURL(ctx context.Context, client *http.Client, source string, headers http.Header) (int, error) {
	for _, method := range []string{http.MethodHead, http.MethodGet} {
		attemptCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		req, err := downloadRequest(attemptCtx, method, source, headers)
		if err != nil {
			cancel()
			return 0, err
		}
		if method == http.MethodGet {
			req.Header.Set("Range", "bytes=0-0")
		}
		resp, err := client.Do(req)
		status := 0
		if resp != nil {
			status = resp.StatusCode
			resp.Body.Close()
		}
		cancel()
		if err != nil {
			return status, safeDownloadError(err)
		}
		if method == http.MethodGet || (status != http.StatusForbidden && status != http.StatusMethodNotAllowed && status != http.StatusNotImplemented) {
			return status, nil
		}
	}
	return 0, fmt.Errorf("download probe failed")
}

func downloadFromSources(ctx context.Context, client *http.Client, urls []string, destination string, expectedSize int64, headers http.Header, progress func(int64, int64)) (string, error) {
	if len(urls) == 0 {
		return "", fmt.Errorf("no download URLs")
	}
	var failures []error
	for index, source := range urls {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		retry, err := downloadSourceAttempt(ctx, client, source, destination, expectedSize, headers, progress)
		if err == nil {
			return source, nil
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		failures = append(failures, fmt.Errorf("source %d: %w", index+1, err))
		if !retry {
			break
		}
	}
	return "", fmt.Errorf("asset download failed: %w", errors.Join(failures...))
}

func downloadSourceAttempt(ctx context.Context, client *http.Client, source, destination string, expectedSize int64, headers http.Header, progress func(int64, int64)) (retry bool, resultErr error) {
	file, err := os.CreateTemp(filepath.Dir(destination), ".blendkit-download-*")
	if err != nil {
		return false, err
	}
	defer func() {
		file.Close()
		if err := os.Remove(file.Name()); err != nil && !os.IsNotExist(err) {
			retry = false
			resultErr = errors.Join(resultErr, fmt.Errorf("cleaning partial download: %w", err))
		}
	}()
	attemptCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	idleTimer := time.AfterFunc(30*time.Second, cancel)
	defer idleTimer.Stop()
	req, err := downloadRequest(attemptCtx, http.MethodGet, source, headers)
	if err != nil {
		return true, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return true, fmt.Errorf("request failed: %w", safeDownloadError(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return retryDownloadStatus(resp.StatusCode), fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	total := resp.ContentLength
	if total < 0 && !resp.Uncompressed && expectedSize > 0 {
		total = expectedSize
	}
	if progress != nil {
		progress(0, total)
	}
	var downloaded int64
	buffer := make([]byte, 32*1024)
	for {
		if err := attemptCtx.Err(); err != nil {
			return true, err
		}
		count, readErr := resp.Body.Read(buffer)
		if count > 0 {
			idleTimer.Reset(30 * time.Second)
			written, writeErr := file.Write(buffer[:count])
			if writeErr != nil {
				return false, writeErr
			}
			if written != count {
				return false, io.ErrShortWrite
			}
			downloaded += int64(count)
			if progress != nil {
				progress(downloaded, total)
			}
		}
		if readErr != nil {
			if readErr != io.EOF {
				return true, fmt.Errorf("reading download: %w", readErr)
			}
			break
		}
	}
	if total >= 0 && downloaded != total {
		return true, fmt.Errorf("download size mismatch: got %d, expected %d", downloaded, total)
	}
	if err := attemptCtx.Err(); err != nil {
		return true, err
	}
	if err := file.Close(); err != nil {
		return false, err
	}
	if err := os.Rename(file.Name(), destination); err != nil {
		return false, err
	}
	return false, nil
}
