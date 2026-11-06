// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/goccy/go-json"
)

// FetchCA loads a PEM-encoded CA certificate. When caFile is non-empty the
// file is read directly. Otherwise the certificate is fetched from caURL,
// which must return JSON with a "certs" field containing the PEM data.
func FetchCA(ctx context.Context, caURL, caFile string) ([]byte, error) {
	if caFile != "" {
		data, err := os.ReadFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("reading CA file %s: %w", caFile, err)
		}
		return data, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, caURL, http.NoBody)
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
		return nil, fmt.Errorf("no certs field in response from %s", caURL)
	}
	return []byte(result.Certs), nil
}
