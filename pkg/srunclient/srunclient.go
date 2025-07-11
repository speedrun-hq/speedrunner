// Package srunclient provides a client for interacting with the Speedrun API.
package srunclient

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/speedrun-hq/speedrunner/pkg/logger"
	"github.com/speedrun-hq/speedrunner/pkg/models"
)

// APIResponse represents the structure of the API response
type APIResponse struct {
	Intents    []models.Intent `json:"intents,omitempty"`
	Data       []models.Intent `json:"data,omitempty"`    // Some APIs use "data" as the key
	Results    []models.Intent `json:"results,omitempty"` // Some APIs use "results" as the key
	Page       int             `json:"page"`
	PageSize   int             `json:"page_size"`
	TotalCount int             `json:"total_count"`
	TotalPages int             `json:"total_pages"`
}

// Client represents a Speedrun API client
type Client struct {
	endpoint   string
	httpClient *http.Client
	logger     logger.Logger
}

// New creates a new Speedrun API client
func New(endpoint string, logger logger.Logger) *Client {
	return &Client{
		endpoint:   endpoint,
		httpClient: createHTTPClient(),
		logger:     logger,
	}
}

// FetchPendingIntents gets pending intents from the API
func (c *Client) FetchPendingIntents() ([]models.Intent, error) {
	resp, err := c.httpClient.Get(c.endpoint + "/api/v1/intents?status=pending")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch pending intents: %v", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			c.logger.Error("Failed to close response body: %v", err)
		}
	}(resp.Body)

	// Read the response body regardless of status code
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	// Try to unmarshal into our wrapper struct first
	var apiResp APIResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		// If that fails, try directly as an array
		var intents []models.Intent
		if err := json.Unmarshal(bodyBytes, &intents); err != nil {
			return nil, fmt.Errorf("failed to decode intents: %v, body: %s", err, string(bodyBytes))
		}
		return intents, nil
	}

	// Handle paginated response with no data
	if apiResp.TotalCount == 0 {
		c.logger.Debug("No pending intents found (page %d/%d, total count: %d)",
			apiResp.Page, apiResp.TotalPages, apiResp.TotalCount)
		return []models.Intent{}, nil
	}

	// Get intents from whatever field is populated
	var intents []models.Intent
	if len(apiResp.Intents) > 0 {
		intents = apiResp.Intents
	} else if len(apiResp.Data) > 0 {
		intents = apiResp.Data
	} else if len(apiResp.Results) > 0 {
		intents = apiResp.Results
	} else {
		// Try one more thing - maybe it's in a top level array with a different name
		// Parse as generic map and look for any array field
		var genericResp map[string]interface{}
		if err := json.Unmarshal(bodyBytes, &genericResp); err != nil {
			return nil, fmt.Errorf("failed to decode response: %v", err)
		}

		for key, value := range genericResp {
			if arrayValue, ok := value.([]interface{}); ok && len(arrayValue) > 0 {
				// Found an array, try to convert it to intents
				arrayJSON, err := json.Marshal(arrayValue)
				if err != nil {
					continue
				}
				if err := json.Unmarshal(arrayJSON, &intents); err == nil && len(intents) > 0 {
					c.logger.Debug("Found intents in field: %s", key)
					break
				}
			}
		}

		if len(intents) == 0 {
			// This is a normal case when there are no pending intents
			c.logger.Debug("No pending intents found in API response")
			return []models.Intent{}, nil
		}
	}
	return intents, nil
}

// Helper function to create an HTTP client with timeouts
func createHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
		},
	}
}
