package api

import "time"

type AggregationType string

type DeviceContextResponse struct {
	Version     string          `json:"version"`
	GeneratedAt time.Time       `json:"generatedAt"`
	Devices     []DeviceContext `json:"devices"`
}

type DeviceContext struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Exposes     []ExposeInfo `json:"exposes"`
}

type ExposeInfo struct {
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	Unit         string            `json:"unit,omitempty"`
	Values       []string          `json:"values,omitempty"`
	ValueOn      any               `json:"valueOn,omitempty"`
	ValueOff     any               `json:"valueOff,omitempty"`
	ValueToggle  any               `json:"valueToggle,omitempty"`
	Aggregations []AggregationType `json:"aggregations"`
}
