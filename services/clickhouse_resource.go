package services

import (
	"context"

	clickhouseclient "deploycrate-ce/clients/clickhouse"
	"deploycrate-ce/config"
)

type ClickHouseResource struct {
	configuration config.ClickHouse
}

func NewClickHouseResource(configuration config.Config) *ClickHouseResource {
	return &ClickHouseResource{
		configuration: configuration.ClickHouse,
	}
}

func (resource *ClickHouseResource) Client(_ context.Context) (clickhouseclient.Client, error) {
	return clickhouseclient.New(
		resource.configuration.GetURL(),
		resource.configuration.Database,
		resource.configuration.User,
		resource.configuration.Password,
	), nil
}
