package mocks

import "llm-proxy/internal/device_context"

// Mock HttpDeviceContextFetcher
type MockHttpDeviceContextFetcher struct {
	callCount int
	result    *device_context.DeviceContextResponse
	err       error
}

func NewMockHttpDeviceContextFetcher(result *device_context.DeviceContextResponse, err error) *MockHttpDeviceContextFetcher {
	return &MockHttpDeviceContextFetcher{
		result: result,
		err:    err,
	}
}
func (m *MockHttpDeviceContextFetcher) CallCount() int {
	return m.callCount
}

func (m *MockHttpDeviceContextFetcher) SetResult(result *device_context.DeviceContextResponse) {
	m.result = result
}

func (m *MockHttpDeviceContextFetcher) FetchDeviceContext() (*device_context.DeviceContextResponse, error) {
	m.callCount++
	return m.result, m.err
}

// Mock DeviceContextProvider
type MockDeviceContextProvider struct {
	ctx       *device_context.DeviceContextResponse
	err       error
	callCount int
}

func NewMockDeviceContextProvider(ctx *device_context.DeviceContextResponse, err error) *MockDeviceContextProvider {
	return &MockDeviceContextProvider{
		ctx: ctx,
		err: err,
	}
}
func (m *MockDeviceContextProvider) GetDeviceContext() (*device_context.DeviceContextResponse, error) {
	m.callCount++
	return m.ctx, m.err
}

func (l *MockDeviceContextProvider) CallCount() int {
	return l.callCount
}
