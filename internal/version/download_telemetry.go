package version

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const downloadTelemetryInterval = 500 * time.Millisecond

type downloadTelemetryRequest struct {
	URL, Dest               string
	MaxBytes, ExpectedBytes int64
	MaxAttempts             int
	Report                  func(DownloadProgress)
}

type downloadProgressWriter struct {
	report func(DownloadProgress)
	state  DownloadProgress
	last   time.Time
}

func (w *downloadProgressWriter) emit(complete bool) {
	w.state.Complete = complete
	w.state.Percent = -1
	if w.state.TotalBytes > 0 {
		w.state.Percent = int(w.state.DownloadedBytes * 100 / w.state.TotalBytes)
		if w.state.Percent > 99 {
			w.state.Percent = 99
		}
	}
	if complete {
		w.state.Percent = 100
	}
	w.last = time.Now()
	if w.report != nil {
		w.report(w.state)
	}
}

func (w *downloadProgressWriter) Write(p []byte) (int, error) {
	w.state.DownloadedBytes += int64(len(p))
	if time.Since(w.last) >= downloadTelemetryInterval {
		w.emit(false)
	}
	return len(p), nil
}

func downloadWithProgress(ctx context.Context, url, dest string, maxBytes, expectedBytes int64, report func(DownloadProgress)) error {
	return downloadTelemetryAttempts(ctx, downloadTelemetryRequest{URL: url, Dest: dest, MaxBytes: maxBytes, ExpectedBytes: expectedBytes, MaxAttempts: downloadMaxAttempts, Report: report})
}

func downloadTelemetryAttempts(ctx context.Context, request downloadTelemetryRequest) error {
	if request.MaxAttempts < 1 {
		request.MaxAttempts = 1
	}
	var lastErr error
	attempts := 0
	for attempt := 1; attempt <= request.MaxAttempts; attempt++ {
		attempts = attempt
		tracker := &downloadProgressWriter{report: request.Report, state: DownloadProgress{TotalBytes: request.ExpectedBytes, Attempt: attempt}}
		tracker.emit(false)
		attemptCtx := ctx
		cancel := func() {}
		if downloadAttemptTimeout > 0 {
			attemptCtx, cancel = context.WithTimeout(ctx, downloadAttemptTimeout)
		}
		retryable, err := downloadTelemetryOnce(attemptCtx, request, tracker)
		if err != nil && errors.Is(attemptCtx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
			retryable = true
			err = fmt.Errorf("下载尝试超时（%s）", downloadAttemptTimeout)
		}
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		if !retryable || attempt == request.MaxAttempts {
			break
		}
		if err := waitDownloadRetry(ctx, downloadRetryDelay*time.Duration(attempt)); err != nil {
			return err
		}
	}
	return fmt.Errorf("下载失败（已尝试 %d 次）: %w", attempts, lastErr)
}

func downloadTelemetryOnce(ctx context.Context, request downloadTelemetryRequest, tracker *downloadProgressWriter) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, request.URL, nil)
	if err != nil {
		return false, fmt.Errorf("create download request: %w", err)
	}
	req.Header.Set("User-Agent", "ga-admin-updater")
	resp, err := downloadHTTPClient.Do(req)
	if err != nil {
		return retryableDownloadError(ctx, err), err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return retryableDownloadStatus(resp.StatusCode), fmt.Errorf("download failed: %s", resp.Status)
	}
	if request.MaxBytes > 0 && resp.ContentLength > request.MaxBytes {
		return false, fmt.Errorf("download too large: %d bytes exceeds limit %d", resp.ContentLength, request.MaxBytes)
	}
	if resp.ContentLength > 0 {
		tracker.state.TotalBytes = resp.ContentLength
	}
	tracker.emit(false)
	var reader io.Reader = resp.Body
	if request.MaxBytes > 0 {
		reader = http.MaxBytesReader(nil, resp.Body, request.MaxBytes)
	}
	if err := writeStreamAtomic(request.Dest, io.TeeReader(reader, tracker), 0600); err != nil {
		tracker.emit(false)
		return retryableDownloadError(ctx, err), fmt.Errorf("write download file: %w", err)
	}
	tracker.emit(true)
	return false, nil
}

func downloadReleaseAssetWithTelemetry(ctx context.Context, request downloadTelemetryRequest, report func(string, DownloadProgress)) (string, error) {
	candidates := releaseDownloadCandidates(request.URL)
	errorsBySource := make([]string, 0, len(candidates))
	for i, candidate := range candidates {
		attempts := downloadMaxAttempts
		if len(candidates) > 1 {
			attempts = 1
		}
		attemptCtx := ctx
		cancel := func() {}
		if i == 0 && len(candidates) > 1 && releaseDirectTimeout > 0 {
			attemptCtx, cancel = context.WithTimeout(ctx, releaseDirectTimeout)
		}
		attemptRequest := request
		attemptRequest.URL, attemptRequest.MaxAttempts = candidate.URL, attempts
		attemptRequest.Report = func(transfer DownloadProgress) {
			if len(candidates) > 1 {
				transfer.Attempt += i
			}
			if report != nil {
				report(candidate.Label, transfer)
			}
		}
		err := downloadTelemetryAttempts(attemptCtx, attemptRequest)
		cancel()
		if err == nil {
			return candidate.Label, nil
		}
		errorsBySource = append(errorsBySource, candidate.Label+": "+err.Error())
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
	}
	return "", fmt.Errorf("升级包下载失败；%s", strings.Join(errorsBySource, "；"))
}
