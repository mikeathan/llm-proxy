package app

import (
	"llm-proxy/internal/device_context"
	"llm-proxy/utils"
	"net/http"
	"time"
)

func buildDeviceContextProvider(clock utils.Clock) device_context.DeviceContextProvider {
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
	}

	fetcher := device_context.NewHttpDeviceContextFetcher(
		utils.Require("DEVICE_CONTEXT_BASE_URL"),
		httpClient,
	)

	cache := device_context.NewDeviceContextCache(1*time.Minute, clock)
	return device_context.NewDeviceContextProvider(fetcher, cache)
}
