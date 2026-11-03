package utils

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/goccy/go-json"
)

// FetchCA fetches a PEM-encoded CA certificate from the given discovery URL.
// The URL is expected to return JSON with a "certs" field containing the PEM data.
func FetchCA(ctx context.Context, url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var result struct {
		Certs string `json:"certs"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	if result.Certs == "" {
		return nil, fmt.Errorf("no certs field in response from %s", url)
	}
	return []byte(result.Certs), nil
}
