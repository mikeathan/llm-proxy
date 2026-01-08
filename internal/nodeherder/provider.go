package nodeherder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"llm-proxy/internal/logging"
	"llm-proxy/utils"
	"net/http"
	"sync"
	"time"
)

// Node Herder
type NodeHerderService interface {
	GetDeviceContext() (*LLMDeviceContext, error)
	QueryMetrics(ctx context.Context, request *MetricsQueryRequest) (*MetricsQueryResponse, error)
}

type nodeHerder struct {
	fetcher NodeHerderFetcher
	cache   *DeviceContextCache
	logger  logging.Logger
}

func NewNodeHerder(fetcher NodeHerderFetcher, cache *DeviceContextCache, logger logging.Logger) NodeHerderService {
	return &nodeHerder{fetcher: fetcher, cache: cache, logger: logger}
}

func (p *nodeHerder) GetDeviceContext() (*LLMDeviceContext, error) {

	if ctx, ok := p.cache.Get(); ok {
		p.logger.Info("device context cache hit", "version", ctx.Version, "generated_at", ctx.GeneratedAt)
		return ctx, nil
	}

	response, err := p.fetcher.FetchDeviceContext()
	if err != nil {
		return nil, err
	}

	llmCtx := transformToLLMDeviceContext(response)
	p.cache.Set(llmCtx)
	p.logger.Info("device context fetched", "version", llmCtx.Version, "generated_at", llmCtx.GeneratedAt)
	return llmCtx, nil
}

func (p *nodeHerder) QueryMetrics(ctx context.Context, request *MetricsQueryRequest) (*MetricsQueryResponse, error) {
	return p.fetcher.QueryMetrics(ctx, request)
}

// Http Node Herder Fetcher
type NodeHerderFetcher interface {
	FetchDeviceContext() (*DeviceContextResponse, error)
	QueryMetrics(ctx context.Context, request *MetricsQueryRequest) (*MetricsQueryResponse, error)
}

type HttpNodeHerderFetcher struct {
	deviceContextURL string
	queryMetricsURL  string
	client           *http.Client
	tokenManager     TokenManager
}

func NewHttpNodeHerderFetcher(baseUrl string, client *http.Client, tokenManager TokenManager) NodeHerderFetcher {

	return &HttpNodeHerderFetcher{
		deviceContextURL: utils.SanitiseUrl(baseUrl) + "/api/context/devices",
		queryMetricsURL:  utils.SanitiseUrl(baseUrl) + "/api/metrics/query",
		client:           client,
		tokenManager:     tokenManager,
	}
}

func (c *HttpNodeHerderFetcher) FetchDeviceContext() (*DeviceContextResponse, error) {

	res, err := c.client.Get(c.deviceContextURL)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("fetch device context returned %d: %s", res.StatusCode, string(body))
	}

	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	var response DeviceContextResponse
	err = json.Unmarshal(resBody, &response)
	if err != nil {
		return nil, err
	}

	return &response, nil
}

func (c *HttpNodeHerderFetcher) QueryMetrics(ctx context.Context, request *MetricsQueryRequest) (*MetricsQueryResponse, error) {

	body, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	token, err := c.tokenManager.Get(ctx)
	if err != nil {
		return nil, err
	}

	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.queryMetricsURL, bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)

	res, err := c.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("query metrics returned %d: %s", res.StatusCode, string(b))
	}

	var out []MetricsQueryResponse
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, err
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("empty metrics response")
	}

	return &out[0], nil
}

// Token manager
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
		tokenURL: utils.SanitiseUrl(baseURL) + "/api/auth/token",
	}
}

func (m *ServiceTokenManager) Get(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	clientID, clientSecret, err := utils.LoadServiceCredentials()
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

// Transform DeviceContextResponse to LLMDeviceContext
func transformToLLMDeviceContext(response *DeviceContextResponse) *LLMDeviceContext {
	llmDevices := make([]LLMDevice, 0, len(response.Devices))

	for _, device := range response.Devices {
		llmExposes := make([]LLMExpose, 0, len(device.Exposes))

		for _, expose := range device.Exposes {
			llmExpose := LLMExpose{
				Name:         expose.Name,
				Type:         expose.Type,
				Unit:         expose.Unit,
				States:       expose.Values,
				On:           expose.ValueOn,
				Off:          expose.ValueOff,
				Toggle:       expose.ValueToggle,
				Aggregations: make([]string, len(expose.Aggregations)),
			}

			for i, agg := range expose.Aggregations {
				llmExpose.Aggregations[i] = string(agg)
			}

			llmExposes = append(llmExposes, llmExpose)
		}

		llmDevices = append(llmDevices, LLMDevice{
			ID:      device.ID,
			Name:    device.Name,
			Desc:    device.Description,
			Exposes: llmExposes,
		})
	}

	return &LLMDeviceContext{
		Version:     response.Version,
		GeneratedAt: response.GeneratedAt.Unix(),
		Devices:     llmDevices,
	}
}
