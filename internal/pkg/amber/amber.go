package amber

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"

	ac "github.com/anicoll/winet-integration/pkg/amber"
	"github.com/anicoll/winet-integration/pkg/utils"
	"go.uber.org/zap"
)

type client struct {
	aClient  ac.ClientWithResponsesInterface
	logger   *zap.Logger
	apiToken string
	sites    []ac.Site
}

func withToken(token string) func(ctx context.Context, req *http.Request) error {
	return func(ctx context.Context, req *http.Request) error {
		req.Header.Add("Authorization", "Bearer "+token)
		return nil
	}
}

func New(ctx context.Context, server, token string) (*client, error) {
	c, err := ac.NewClientWithResponses(server)
	if err != nil {
		return nil, err
	}

	amberClient := &client{
		logger:   zap.L(),
		aClient:  c,
		apiToken: token,
	}
	siteResponse, err := c.GetSitesWithResponse(ctx, withToken(token))
	if err != nil {
		return nil, err
	}

	if siteResponse.JSON200 != nil {
		for _, site := range *siteResponse.JSON200 {
			zap.L().Info("received site from amber", zap.Any("site", site))
		}
		amberClient.sites = *siteResponse.JSON200
	}

	return amberClient, nil
}

func (c *client) GetSites() []ac.Site {
	return c.sites
}

func (c *client) GetPrices(ctx context.Context, siteID string) error {
	response, err := c.aClient.GetCurrentPricesWithResponse(ctx, siteID, &ac.GetCurrentPricesParams{
		Next:     utils.ToPtr(10),
		Previous: utils.ToPtr(5),
	}, withToken(c.apiToken))
	if err != nil {
		return err
	}
	historicalIntervals := []ac.ActualInterval{}
	forecastIntervals := []ac.ForecastInterval{}
	currentIntervals := []ac.CurrentInterval{}
	if response.JSON200 != nil {
		for _, interval := range *response.JSON200 {
			data, err := interval.MarshalJSON()
			if err != nil {
				return err
			}
			base := ac.BaseInterval{}
			if err := json.Unmarshal(data, &base); err != nil {
				return err
			}
			switch base.Type {
			case string(ac.ActualIntervalTypeActualInterval):
				actual, err := interval.AsActualInterval()
				if err != nil {
					return err
				}
				historicalIntervals = append(historicalIntervals, actual)
			case string(ac.ForecastIntervalTypeForecastInterval):
				forecast, err := interval.AsForecastInterval()
				if err != nil {
					return err
				}
				forecastIntervals = append(forecastIntervals, forecast)
			case string(ac.CurrentIntervalTypeCurrentInterval):
				current, err := interval.AsCurrentInterval()
				if err != nil {
					return err
				}
				currentIntervals = append(currentIntervals, current)
			}
		}
	}
	_ = historicalIntervals
	_ = forecastIntervals
	_ = currentIntervals
	slices.SortFunc(historicalIntervals, func(a, b ac.ActualInterval) int {
		return a.StartTime.Compare(b.StartTime)
	})
	slices.SortFunc(forecastIntervals, func(a, b ac.ForecastInterval) int {
		return a.StartTime.Compare(b.StartTime)
	})
	slices.SortFunc(currentIntervals, func(a, b ac.CurrentInterval) int {
		return a.StartTime.Compare(b.StartTime)
	})
	return nil
}
