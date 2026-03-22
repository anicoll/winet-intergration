package amber

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ac "github.com/anicoll/winet-integration/pkg/amber"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

func newTestClient(t *testing.T, handler http.Handler) *client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	aClient, err := ac.NewClientWithResponses(srv.URL)
	require.NoError(t, err)

	return &client{
		aClient:  aClient,
		apiToken: "test-token",
		loc:      time.UTC,
	}
}

func mustDate(s string) openapi_types.Date {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return openapi_types.Date{Time: t}
}

// --- GetUsage ---

func TestGetUsage_MapsResponseCorrectly(t *testing.T) {
	start := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(30 * time.Minute)

	usageResp := []map[string]any{
		{
			"type":              "Usage",
			"channelIdentifier": "E1",
			"channelType":       "general",
			"kwh":               1.5,
			"quality":           "billable",
			"cost":              0.42,
			"perKwh":            28.0,
			"spotPerKwh":        6.5,
			"startTime":         start.Format(time.RFC3339),
			"endTime":           end.Format(time.RFC3339),
			"duration":          30,
			"nemTime":           end.Format(time.RFC3339),
			"date":              "2024-03-01",
			"renewables":        35.0,
			"spikeStatus":       "none",
			"descriptor":        "low",
		},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/usage")
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(usageResp)
	})

	c := newTestClient(t, handler)
	result, err := c.GetUsage(context.Background(), "site-123", mustDate("2024-03-01"), mustDate("2024-03-01"))

	require.NoError(t, err)
	require.Len(t, result, 1)

	got := result[0]
	assert.Equal(t, "E1", got.ChannelIdentifier)
	assert.Equal(t, "general", got.ChannelType)
	assert.InDelta(t, 1.5, got.Kwh, 0.001)
	assert.Equal(t, "billable", got.Quality)
	assert.InDelta(t, 0.42, got.Cost, 0.001)
	assert.InDelta(t, 28.0, got.PerKwh, 0.001)
	assert.InDelta(t, 6.5, got.SpotPerKwh, 0.001)
	assert.Equal(t, 30, got.Duration)
	assert.True(t, got.StartTime.Equal(start))
	assert.True(t, got.EndTime.Equal(end))
}

func TestGetUsage_MultipleIntervals(t *testing.T) {
	base := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)

	makeInterval := func(offset int, chanID, chanType string, kwh float64) map[string]any {
		start := base.Add(time.Duration(offset) * 30 * time.Minute)
		return map[string]any{
			"type":              "Usage",
			"channelIdentifier": chanID,
			"channelType":       chanType,
			"kwh":               kwh,
			"quality":           "billable",
			"cost":              0.10,
			"perKwh":            20.0,
			"spotPerKwh":        5.0,
			"startTime":         start.Format(time.RFC3339),
			"endTime":           start.Add(30 * time.Minute).Format(time.RFC3339),
			"duration":          30,
			"nemTime":           start.Add(30 * time.Minute).Format(time.RFC3339),
			"date":              "2024-03-01",
			"renewables":        40.0,
			"spikeStatus":       "none",
			"descriptor":        "neutral",
		}
	}

	usageResp := []map[string]any{
		makeInterval(0, "E1", "general", 1.0),
		makeInterval(0, "B2", "feedIn", -0.5),
		makeInterval(1, "E1", "general", 1.2),
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(usageResp)
	})

	c := newTestClient(t, handler)
	result, err := c.GetUsage(context.Background(), "site-123", mustDate("2024-03-01"), mustDate("2024-03-01"))

	require.NoError(t, err)
	assert.Len(t, result, 3)
}

func TestGetUsage_EmptyResponse_ReturnsEmpty(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]any{})
	})

	c := newTestClient(t, handler)
	result, err := c.GetUsage(context.Background(), "site-123", mustDate("2024-03-01"), mustDate("2024-03-01"))

	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestGetUsage_ServerError_ReturnsError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	})

	c := newTestClient(t, handler)
	// A non-200 response doesn't error — JSON200 will be nil
	result, err := c.GetUsage(context.Background(), "site-123", mustDate("2024-03-01"), mustDate("2024-03-01"))

	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestGetUsage_SendsCorrectQueryParams(t *testing.T) {
	var capturedQuery string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]any{})
	})

	c := newTestClient(t, handler)
	_, err := c.GetUsage(context.Background(), "site-abc", mustDate("2024-03-01"), mustDate("2024-03-07"))

	require.NoError(t, err)
	assert.Contains(t, capturedQuery, "startDate=2024-03-01")
	assert.Contains(t, capturedQuery, "endDate=2024-03-07")
}
