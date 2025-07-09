package fulfiller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/speedrun-hq/speedrunner/pkg/logger"
	"github.com/speedrun-hq/speedrunner/pkg/models"
)

// APIClient handles API interactions for fetching intents
type APIClient struct {
	httpClient *http.Client
	endpoint   string
	logger     logger.Logger
}

// NewAPIClient creates a new API client
func NewAPIClient(endpoint string, logger logger.Logger) *APIClient {
	return &APIClient{
		httpClient: createHTTPClient(),
		endpoint:   endpoint,
		logger:     logger,
	}
}

// FetchPendingIntents gets pending intents from the API
func (ac *APIClient) FetchPendingIntents() ([]models.Intent, error) {
	resp, err := ac.httpClient.Get(ac.endpoint + "/api/v1/intents?status=pending")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch pending intents: %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			ac.logger.Error("Failed to close response body: %v", closeErr)
		}
	}()

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
		ac.logger.Debug("No pending intents found (page %d/%d, total count: %d)",
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
					ac.logger.Debug("Found intents in field: %s", key)
					break
				}
			}
		}

		if len(intents) == 0 {
			// This is a normal case when there are no pending intents
			ac.logger.Debug("No pending intents found in API response")
			return []models.Intent{}, nil
		}
	}
	return intents, nil
}
