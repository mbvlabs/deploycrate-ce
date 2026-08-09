SELECT
  toString(toUInt64(toUnixTimestamp(history_bucket)) * 1000) AS bucket_start_milliseconds,
  metric,
  argMax(`last`, observed_at) AS value
FROM metric_rollups
WHERE scope = 'host'
  AND server = {server:String}
  AND bucket_start >= toDateTime({since_seconds:UInt32})
  AND metric IN (
    'cpu_cores_used',
    'cpu_cores_total',
    'memory_available_bytes',
    'memory_total_bytes',
    'root_filesystem_available_bytes',
    'root_filesystem_size_bytes',
    'disk_read_bytes_per_second',
    'disk_write_bytes_per_second',
    'network_receive_bytes_per_second',
    'network_transmit_bytes_per_second',
    'oom_events',
    'tasks'
  )
GROUP BY
  toStartOfInterval(bucket_start, toIntervalSecond({bucket_seconds:UInt32})) AS history_bucket,
  metric
ORDER BY history_bucket, metric
FORMAT JSONEachRow
