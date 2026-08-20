SELECT
  metric,
  argMax(`last`, observed_at) AS value,
  toString(toUnixTimestamp64Milli(max(observed_at))) AS observed_at_milliseconds
FROM metric_rollups
WHERE scope = 'host'
  AND server = {server:String}
  AND bucket_start >= now() - INTERVAL 10 MINUTE
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
GROUP BY metric
FORMAT JSONEachRow
