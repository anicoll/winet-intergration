package winet

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"strings"
)

func (s *service) getProperties(ctx context.Context) error {
	if s.properties != nil {
		return nil // already loaded
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+s.cfg.Host+"/i18n/en_US.properties", nil)
	if err != nil {
		return err
	}
	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
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
