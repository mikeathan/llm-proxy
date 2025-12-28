package device_context

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
