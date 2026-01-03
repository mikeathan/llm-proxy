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
	"os"
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
}

func NewHttpNodeHerderFetcher(baseUrl string, client *http.Client) NodeHerderFetcher {

	queryMetricsURL := utils.SanitiseUrl(baseUrl) + "/api/metrics/query"
	deviceContextURL := utils.SanitiseUrl(baseUrl) + "/api/context/devices"

	return &HttpNodeHerderFetcher{
		deviceContextURL: deviceContextURL,
		queryMetricsURL:  queryMetricsURL,
		client:           client,
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
	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.queryMetricsURL, bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+os.Getenv("NODEHERDER_SERVICE_TOKEN"))

	res, err := c.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("query metrics returned %d: %s", res.StatusCode, string(b))
	}

	var out MetricsQueryResponse
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Transform DeviceContextResponse to LLMDeviceContext
func transformToLLMDeviceContext(response *DeviceContextResponse) *LLMDeviceContext {
	llmDevices := make([]LLMDevice, 0, len(response.Devices))
	for _, device := range response.Devices {
		llmExposes := make(map[string]LLMExpose)
		for _, expose := range device.Exposes {
			llmExpose := LLMExpose{
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
			llmExposes[expose.Name] = llmExpose
		}
		llmDevice := LLMDevice{
			ID:      device.ID,
			Name:    device.Name,
			Desc:    device.Description,
			Exposes: llmExposes,
		}
		llmDevices = append(llmDevices, llmDevice)
	}

	return &LLMDeviceContext{
		Version:     response.Version,
		GeneratedAt: response.GeneratedAt.Unix(),
		Devices:     llmDevices,
	}
}
