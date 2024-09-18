package mqtt

import (
	"errors"
	"time"

	paho_mqtt "github.com/eclipse/paho.mqtt.golang"
)

type service struct {
	client paho_mqtt.Client
}

func New(client paho_mqtt.Client) *service {
	return &service{
		client: client,
	}
}

func (s *service) Connect() error {
	token := s.client.Connect()
	res := token.WaitTimeout(time.Second * 5)
	if res {
		return nil
	}
	if err := token.Error(); err != nil {
		return err
	}
	return errors.New("unable to connect in time")
}
