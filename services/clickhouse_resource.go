package services

import (
	"context"

	clickhouseclient "deploycrate-ce/clients/clickhouse"
	"deploycrate-ce/config"
	clickhouse "deploycrate-ce/database/clickhouse"
)

type ClickHouseResource struct {
	client  clickhouseclient.Client
	queries clickhouse.Queries
}

func NewClickHouseResource(configuration config.Config) *ClickHouseResource {
	clickHouse := configuration.ClickHouse
	return &ClickHouseResource{
		client: clickhouseclient.New(
			clickHouse.GetURL(),
			clickHouse.Database,
			clickHouse.User,
			clickHouse.Password,
		),
		queries: clickhouse.NewQueries(
			clickHouse.GetURL(),
			clickHouse.Database,
			clickHouse.User,
			clickHouse.Password,
		),
	}
}

func (resource *ClickHouseResource) Client(_ context.Context) (clickhouseclient.Client, error) {
	return resource.client, nil
}

func (resource *ClickHouseResource) Queries(_ context.Context) (clickhouse.Queries, error) {
	return resource.queries, nil
}
