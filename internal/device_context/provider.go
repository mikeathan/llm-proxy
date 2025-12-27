package device_context

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Device Context Provider
type DeviceContextProvider interface {
	GetDeviceContext() (*DeviceContextResponse, error)
}

type deviceContextProvider struct {
	fetcher DeviceContextFetcher
	cache   *DeviceContextCache
}

func NewDeviceContextProvider(fetcher DeviceContextFetcher, cache *DeviceContextCache) *deviceContextProvider {
	return &deviceContextProvider{fetcher: fetcher, cache: cache}
}

func (p *deviceContextProvider) GetDeviceContext() (*DeviceContextResponse, error) {

	if ctx, ok := p.cache.Get(); ok {
		return ctx, nil
	}

	ctx, err := p.fetcher.FetchDeviceContext()
	if err != nil {
		return nil, err
	}

	p.cache.Set(ctx)
	return ctx, nil
}

// Http Device Context Fetcher
type DeviceContextFetcher interface {
	FetchDeviceContext() (*DeviceContextResponse, error)
}

type HttpDeviceContextFetcher struct {
	deviceContextURL string

	client *http.Client
}

func NewHttpDeviceContextFetcher(baseUrl string, client *http.Client) *HttpDeviceContextFetcher {

	deviceContextURL := sanitiseUrl(baseUrl) + "/api/context/devices"
	return &HttpDeviceContextFetcher{
		deviceContextURL: deviceContextURL,
		client:           client,
	}
}

func (c *HttpDeviceContextFetcher) FetchDeviceContext() (*DeviceContextResponse, error) {

	res, err := c.client.Get(c.deviceContextURL)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("nodeherder returned %d: %s", res.StatusCode, string(body))
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

func sanitiseUrl(url string) string {
	if after, ok := strings.CutPrefix(url, "/"); ok {
		return after
	}
	return url
}
