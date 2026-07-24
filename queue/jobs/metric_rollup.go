package jobs

type MetricRollupArgs struct{}

func (MetricRollupArgs) Kind() string { return "metric_rollup" }
