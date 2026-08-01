package models

import (
	"encoding/json"
	"errors"
)

type ServerCapabilities struct {
	Build      bool `json:"build"`
	Runtime    bool `json:"runtime"`
	Resource   bool `json:"resource"`
	Database   bool `json:"database"`
	Repository bool `json:"repository"`
	Telemetry  bool `json:"telemetry"`
}

func (capabilities ServerCapabilities) Validate() error {
	if !capabilities.Telemetry {
		return errors.New("managed nodes must collect telemetry")
	}
	if !capabilities.Build && !capabilities.Runtime && !capabilities.Resource && !capabilities.Database && !capabilities.Repository {
		return errors.New("select at least one node workload capability")
	}
	return nil
}

func (capabilities ServerCapabilities) JSON() (json.RawMessage, error) {
	if err := capabilities.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(capabilities)
}

func ParseServerCapabilities(value json.RawMessage) (ServerCapabilities, error) {
	var capabilities ServerCapabilities
	if err := json.Unmarshal(value, &capabilities); err != nil {
		return ServerCapabilities{}, err
	}
	return capabilities, capabilities.Validate()
}
