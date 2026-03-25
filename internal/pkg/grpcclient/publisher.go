// Package grpcclient provides a publisher.Publisher implementation that forwards
// data to the Azure ingestion function over the Connect protocol (connectrpc.com).
//
// The Publisher type is the sole client of the IngestionService and
// CommandService RPCs defined in proto/winet/v1/. It is injected into the
// local service in place of the direct database backend.
package grpcclient

import (
	"context"
	"net/http"
	"slices"
	"sync"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	winetv1 "github.com/anicoll/winet-integration/gen/winet/v1"
	"github.com/anicoll/winet-integration/gen/winet/v1/winetv1connect"
	dbpkg "github.com/anicoll/winet-integration/internal/pkg/database/db"
	"github.com/anicoll/winet-integration/internal/pkg/model"
	"github.com/anicoll/winet-integration/internal/pkg/publisher"
)

// Publisher forwards data to the Azure ingestion function via Connect RPCs.
// It implements publisher.Publisher and additionally exposes methods for
// writing Amber data and polling/acknowledging inverter commands.
type Publisher struct {
	ingestion winetv1connect.IngestionServiceClient
	commands  winetv1connect.CommandServiceClient
	mu        sync.RWMutex
	deviceIDs []string
}

// New creates a Publisher that calls the ingestion function at baseURL,
// authenticating each request with the provided API key as a Bearer token.
func New(baseURL, apiKey string) *Publisher {
	authInterceptor := connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			req.Header().Set("Authorization", "Bearer "+apiKey)
			return next(ctx, req)
		}
	})
	opts := connect.WithInterceptors(authInterceptor)
	httpClient := &http.Client{}
	return &Publisher{
		ingestion: winetv1connect.NewIngestionServiceClient(httpClient, baseURL, opts),
		commands:  winetv1connect.NewCommandServiceClient(httpClient, baseURL, opts),
	}
}

// Write implements publisher.Publisher. It forwards a batch of normalised
// sensor readings to the ingestion function via the IngestData RPC.
func (p *Publisher) Write(ctx context.Context, data []publisher.DataPoint) error {
	pts := make([]*winetv1.DataPoint, len(data))
	for i, dp := range data {
		pts[i] = &winetv1.DataPoint{
			Value:             dp.Value,
			Slug:              dp.Slug,
			Timestamp:         timestamppb.New(dp.Timestamp),
			Identifier:        dp.Identifier,
			UnitOfMeasurement: dp.UnitOfMeasurement,
		}
	}
	_, err := p.ingestion.IngestData(ctx, connect.NewRequest(&winetv1.IngestDataRequest{
		DataPoints: pts,
	}))
	return err
}

// RegisterDevice implements publisher.Publisher. It upserts a device record
// in the cloud database via the RegisterDevice RPC and tracks the device ID
// for command polling.
func (p *Publisher) RegisterDevice(ctx context.Context, device *model.Device) error {
	_, err := p.ingestion.RegisterDevice(ctx, connect.NewRequest(&winetv1.RegisterDeviceRequest{
		Id:           device.ID,
		Model:        device.Model,
		SerialNumber: device.SerialNumber,
	}))
	if err == nil {
		p.mu.Lock()
		if !slices.Contains(p.deviceIDs, device.ID) {
			p.deviceIDs = append(p.deviceIDs, device.ID)
		}
		p.mu.Unlock()
	}
	return err
}

// DeviceIDs returns the IDs of all devices successfully registered so far.
// Used by the command polling loop to know which devices to poll.
func (p *Publisher) DeviceIDs() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]string, len(p.deviceIDs))
	copy(out, p.deviceIDs)
	return out
}

// WriteAmberPrices forwards Amber electricity price intervals to the ingestion
// function via the IngestAmberPrices RPC.
func (p *Publisher) WriteAmberPrices(ctx context.Context, prices []dbpkg.Amberprice) error {
	proto := make([]*winetv1.AmberPrice, len(prices))
	for i, price := range prices {
		proto[i] = &winetv1.AmberPrice{
			PerKwh:      price.PerKwh,
			SpotPerKwh:  price.SpotPerKwh,
			StartTime:   timestamppb.New(price.StartTime),
			EndTime:     timestamppb.New(price.EndTime),
			Duration:    int32(price.Duration),
			Forecast:    price.Forecast,
			ChannelType: price.ChannelType,
		}
	}
	_, err := p.ingestion.IngestAmberPrices(ctx, connect.NewRequest(&winetv1.IngestAmberPricesRequest{
		Prices: proto,
	}))
	return err
}

// WriteAmberUsage forwards Amber energy usage intervals to the ingestion
// function via the IngestAmberUsage RPC.
func (p *Publisher) WriteAmberUsage(ctx context.Context, usage []dbpkg.Amberusage) error {
	proto := make([]*winetv1.AmberUsage, len(usage))
	for i, u := range usage {
		proto[i] = &winetv1.AmberUsage{
			PerKwh:            u.PerKwh,
			SpotPerKwh:        u.SpotPerKwh,
			StartTime:         timestamppb.New(u.StartTime),
			EndTime:           timestamppb.New(u.EndTime),
			Duration:          int32(u.Duration),
			ChannelType:       u.ChannelType,
			ChannelIdentifier: u.ChannelIdentifier,
			Kwh:               u.Kwh,
			Quality:           u.Quality,
			Cost:              u.Cost,
		}
	}
	_, err := p.ingestion.IngestAmberUsage(ctx, connect.NewRequest(&winetv1.IngestAmberUsageRequest{
		Usage: proto,
	}))
	return err
}

// GetPendingCommands polls the command service for unacknowledged inverter
// commands for the given device. Called on each winet poll cycle.
func (p *Publisher) GetPendingCommands(ctx context.Context, deviceID string) ([]*winetv1.InverterCommand, error) {
	resp, err := p.commands.GetPendingCommands(ctx, connect.NewRequest(&winetv1.GetPendingCommandsRequest{
		DeviceId: deviceID,
	}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.Commands, nil
}

// AckCommand reports the outcome of a previously fetched command back to the
// cloud. success=true means the inverter accepted and applied the command.
func (p *Publisher) AckCommand(ctx context.Context, commandID string, success bool) error {
	_, err := p.commands.AckCommand(ctx, connect.NewRequest(&winetv1.AckCommandRequest{
		CommandId: commandID,
		Success:   success,
	}))
	return err
}
