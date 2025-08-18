package mqtt

import (
	"errors"
	"time"

	paho_mqtt "github.com/eclipse/paho.mqtt.golang"
	"go.uber.org/zap"
)

type service struct {
	client paho_mqtt.Client
	logger *zap.Logger
}

func New(client paho_mqtt.Client) *service {
	return &service{
		client: client,
		logger: zap.L(), // returns the global logger.
	}
}

func (s *service) Connect() error {
	configuredDevices = make(map[string]struct{})
	token := s.client.Connect()
	res := token.WaitTimeout(time.Second * 5)
	if err := token.Error(); err != nil {
		return err
	}
	if res {
		return nil
	}
	return errors.New("unable to connect in time")
}
