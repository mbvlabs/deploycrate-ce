WITH
  if(
    JSONExtractString(Body, 'MESSAGE') != '',
    JSONExtractString(Body, 'MESSAGE'),
    Body
  ) AS access_log,
  JSONExtract(
    access_log,
    'request',
    'headers',
    'Cf-Connecting-Ip',
    'Array(String)'
  ) AS cloudflare_addresses,
  JSONExtract(
    access_log,
    'request',
    'headers',
    'X-Forwarded-For',
    'Array(String)'
  ) AS forwarded_addresses,
  multiIf(
    notEmpty(cloudflare_addresses), cloudflare_addresses[1],
    notEmpty(forwarded_addresses), trim(splitByChar(',', forwarded_addresses[1])[1]),
    JSONExtractString(access_log, 'request', 'remote_ip')
  ) AS client_address
SELECT
  toString(toUnixTimestamp64Nano(Timestamp)) AS timestamp_nanoseconds,
  toString(sipHash64(toString(Timestamp), access_log)) AS fingerprint,
  JSONExtractString(access_log, 'request', 'host') AS host,
  JSONExtractString(access_log, 'request', 'method') AS method,
  JSONExtractString(access_log, 'request', 'uri') AS uri,
  toUInt16(JSONExtractUInt(access_log, 'status')) AS status_code,
  JSONExtractFloat(access_log, 'duration') * 1000 AS duration_ms,
  client_address
FROM otel_logs
WHERE startsWith(JSONExtractString(access_log, 'logger'), 'http.log.access')
  AND Timestamp >= fromUnixTimestamp64Nano({since_nanoseconds:Int64})
  AND Timestamp < fromUnixTimestamp64Nano({before_nanoseconds:Int64})
ORDER BY Timestamp ASC
LIMIT {limit:UInt64}
FORMAT JSONEachRow
