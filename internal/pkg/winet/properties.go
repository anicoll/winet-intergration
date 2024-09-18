package winet

import (
	"context"
	"io"
	"net/http"
	"strings"
)

func (s *service) getProperties(ctx context.Context) error {
	hostport := strings.Split(s.cfg.HostPort, ":")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+hostport[0]+"/i18n/en_US.properties", nil)
	if err != nil {
		return err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	properties := make(map[string]string, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		kv := strings.Split(line, "=")
		properties[kv[0]] = kv[1]
	}

	s.properties = properties
	return nil
}
