package nodeherder

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"llm-proxy/utils"
	"net/http"
)

// Node Herder
type NodeHerderService interface {
	GetDeviceContext() (*LLMDeviceContext, error)
	QueryMetrics(ctx context.Context, request *QueryRequest) (*QueryResponse, error)
}

type nodeHerder struct {
	fetcher NodeHerderFetcher
	cache   *DeviceContextCache
}

func NewNodeHerder(fetcher NodeHerderFetcher, cache *DeviceContextCache) NodeHerderService {
	return &nodeHerder{fetcher: fetcher, cache: cache}
}

func (p *nodeHerder) GetDeviceContext() (*LLMDeviceContext, error) {

	if ctx, ok := p.cache.Get(); ok {
		return ctx, nil
	}

	response, err := p.fetcher.FetchDeviceContext()
	if err != nil {
		return nil, err
	}

	llmCtx := transformToLLMDeviceContext(response)
	p.cache.Set(llmCtx)
	return llmCtx, nil
}

func (p *nodeHerder) QueryMetrics(ctx context.Context, request *QueryRequest) (*QueryResponse, error) {
	return nil, nil
}

// Http Node Herder Fetcher
type NodeHerderFetcher interface {
	FetchDeviceContext() (*DeviceContextResponse, error)
}

type HttpNodeHerderFetcher struct {
	deviceContextURL string

	client *http.Client
}

func NewHttpNodeHerderFetcher(baseUrl string, client *http.Client) NodeHerderFetcher {

	deviceContextURL := utils.SanitiseUrl(baseUrl) + "/api/context/devices"
	return &HttpNodeHerderFetcher{
		deviceContextURL: deviceContextURL,
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
