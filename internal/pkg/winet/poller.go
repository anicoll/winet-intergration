package winet

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"go.uber.org/zap"

	"github.com/anicoll/winet-integration/internal/pkg/model"
	ws "github.com/anicoll/winet-integration/pkg/sockets"
)

// runPollLoop is the single goroutine responsible for the device polling cycle.
// It waits for the login handshake to complete, then repeatedly:
//  1. Requests the device list
//  2. For each device, queries every data stage (Real, RealBattery, Direct)
//  3. Sleeps for cfg.PollInterval before the next cycle
//
// Cancelling ctx is the only way to stop it — no goroutine accumulation on reconnect
// because Connect() cancels the previous poll context before starting a new one.
func (s *service) runPollLoop(ctx context.Context) {
	// Block until the login handshake completes or the context is cancelled.
	select {
	case <-ctx.Done():
		return
	case <-s.loginReady:
	}

	s.logger.Debug("poll loop started")

	for {
		conn := s.getConn()
		if conn == nil {
			return
		}
		s.sendDeviceListRequest(ctx, conn)

		v, err := s.pending.wait(ctx)
		if err != nil {
			s.logger.Error("poll loop: device list wait failed", zap.Error(err))
			return
		}

		devices, ok := v.([]model.DeviceListObject)
		if !ok {
			s.logger.Warn("poll loop: unexpected device list response type", zap.String("type", fmt.Sprintf("%T", v)))
		} else {
			s.queryDevices(ctx, devices)
		}

		// Wait for the next poll interval.
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second * s.cfg.PollInterval):
		}
	}
}

// queryDevices iterates the device list, registers each device, and serially
// queries each data stage, waiting for a response before moving to the next.
func (s *service) queryDevices(ctx context.Context, devices []model.DeviceListObject) {
	for _, device := range devices {
		if len(model.DeviceStages[device.DevType]) == 0 {
			continue
		}

		dev := &model.Device{
			ID:           strconv.Itoa(device.DeviceID),
			Model:        device.DevModel,
			SerialNumber: device.DevSN,
		}

		s.deviceMu.Lock()
		s.currentDevice = dev
		s.deviceMu.Unlock()

		if err := s.publisher.RegisterDevice(ctx, dev); err != nil {
			if ctx.Err() == nil {
				s.sendIfErr(err)
			}
		}

		s.logger.Debug("polling device", zap.String("sn", dev.SerialNumber))

		for _, qs := range model.DeviceStages[device.DevType] {
			if s.getConn() == nil {
				return
			}
			if err := s.sendQueryRequest(qs, device.DeviceID); err != nil {
				if ctx.Err() == nil {
					s.sendIfErr(err)
				}
				return
			}
			if _, err := s.pending.wait(ctx); err != nil {
				s.logger.Error("poll loop: query stage wait failed",
					zap.String("stage", qs.String()), zap.Error(err))
				return
			}
		}
	}
}

// sendQueryRequest marshals and sends a Real/Direct query for the given device stage.
func (s *service) sendQueryRequest(qs model.QueryStage, deviceID int) error {
	data, err := json.Marshal(model.RealRequest{
		DeviceID: fmt.Sprintf("%d", deviceID),
		Time:     fmt.Sprintf("%d", time.Now().UnixMilli()),
		Request: model.Request{
			Lang:    EnglishLang,
			Service: qs.String(),
			Token:   s.token,
		},
	})
	if err != nil {
		return err
	}
	conn := s.getConn()
	if conn == nil {
		return fmt.Errorf("connection is nil, cannot send query")
	}
	s.logger.Debug("sending query", zap.String("stage", qs.String()))
	return conn.Send(ws.Msg{Body: data})
}
