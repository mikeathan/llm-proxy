package nodeherder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"llm-proxy/internal/config"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Token manager defined here for future use with authenticated MCP or other services.

type TokenManager interface {
	Get(ctx context.Context) (string, error)
}

type ServiceTokenManager struct {
	client   *http.Client
	tokenURL string

	mu      sync.Mutex
	token   string
	expires time.Time
}

func NewServiceTokenManager(client *http.Client, baseURL string) TokenManager {
	return &ServiceTokenManager{
		client:   client,
		tokenURL: strings.TrimSuffix(baseURL, "/") + "/api/auth/token",
	}
}

func (m *ServiceTokenManager) Get(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	clientID, clientSecret, err := config.RequireServiceCredentials()
	if err != nil {
		return "", err
	}

	if m.token != "" && time.Now().Before(m.expires.Add(-time.Minute)) {
		return m.token, nil
	}

	body, _ := json.Marshal(map[string]string{
		"client_id":     clientID,
		"client_secret": clientSecret,
	})

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, m.tokenURL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	res, err := m.client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		return "", fmt.Errorf("token request failed: %s", string(b))
	}

	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}

	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return "", err
	}

	m.token = out.AccessToken
	m.expires = time.Now().Add(time.Duration(out.ExpiresIn) * time.Second)

	return m.token, nil
}
