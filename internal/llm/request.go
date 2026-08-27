package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	defaultMaxRetries       = 2
	defaultRetryBaseDelay   = 500 * time.Millisecond
	defaultMaxRetryDelay    = 5 * time.Second
	maxSuccessResponseBytes = 16 << 20
	maxErrorResponseBytes   = 4 << 10
)

func (c *Client) postJSON(ctx context.Context, path string, requestBody, responseBody any) error {
	requestData, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("encode POST %s request: %w", path, err)
	}

	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(requestData))
		if err != nil {
			return fmt.Errorf("create POST %s request: %w", path, err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.apiKey)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return fmt.Errorf("POST %s canceled: %w", path, ctx.Err())
			}
			if attempt < c.maxRetries {
				if err := waitForRetry(ctx, c.retryDelay(attempt, "")); err != nil {
					return fmt.Errorf("POST %s retry canceled: %w", path, err)
				}
				continue
			}
			return fmt.Errorf("send POST %s after %d attempts: %w", path, attempt+1, err)
		}

		statusCode := resp.StatusCode
		retryAfter := resp.Header.Get("Retry-After")
		if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
			responseData, truncated, readErr := readLimitedBody(resp.Body, maxErrorResponseBytes)
			resp.Body.Close()
			if readErr != nil {
				return fmt.Errorf("read POST %s error response: %w", path, readErr)
			}

			responseErr := &responseStatusError{
				Path:       path,
				StatusCode: statusCode,
				Body:       strings.TrimSpace(string(responseData)),
				Truncated:  truncated,
				Attempts:   attempt + 1,
			}
			if isRetryableStatus(statusCode) && attempt < c.maxRetries {
				if err := waitForRetry(ctx, c.retryDelay(attempt, retryAfter)); err != nil {
					return fmt.Errorf("POST %s retry canceled: %w", path, err)
				}
				continue
			}
			return responseErr
		}

		responseData, truncated, readErr := readLimitedBody(resp.Body, maxSuccessResponseBytes)
		resp.Body.Close()
		if readErr != nil {
			return fmt.Errorf("read POST %s response: %w", path, readErr)
		}
		if truncated {
			return fmt.Errorf("POST %s response exceeds %d bytes", path, maxSuccessResponseBytes)
		}
		if err := json.Unmarshal(responseData, responseBody); err != nil {
			return fmt.Errorf("decode POST %s response: %w", path, err)
		}
		return nil
	}
}

type responseStatusError struct {
	Path       string
	StatusCode int
	Body       string
	Truncated  bool
	Attempts   int
}

func (e *responseStatusError) Error() string {
	body := e.Body
	if body == "" {
		body = "empty response body"
	}
	if e.Truncated {
		body += fmt.Sprintf("... (truncated to %d bytes)", maxErrorResponseBytes)
	}
	attempts := ""
	if e.Attempts > 1 {
		attempts = fmt.Sprintf(" after %d attempts", e.Attempts)
	}
	return fmt.Sprintf("POST %s failed with status %d%s: %s", e.Path, e.StatusCode, attempts, body)
}

func readLimitedBody(reader io.Reader, limit int64) ([]byte, bool, error) {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(data)) > limit {
		return data[:limit], true, nil
	}
	return data, false, nil
}

func isRetryableStatus(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests ||
		(statusCode >= http.StatusInternalServerError && statusCode < 600)
}

func (c *Client) retryDelay(attempt int, retryAfter string) time.Duration {
	delay := c.retryBaseDelay
	for i := 0; i < attempt && delay < c.maxRetryDelay; i++ {
		delay *= 2
	}

	if seconds, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && seconds >= 0 {
		serverDelay := time.Duration(seconds) * time.Second
		if serverDelay > delay {
			delay = serverDelay
		}
	}
	if c.maxRetryDelay > 0 && delay > c.maxRetryDelay {
		return c.maxRetryDelay
	}
	return delay
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
