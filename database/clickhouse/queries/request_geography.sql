WITH
  if(
    JSONExtractString(Body, 'MESSAGE') != '',
    JSONExtractString(Body, 'MESSAGE'),
    Body
  ) AS access_log,
  JSONExtractString(access_log, 'request', 'host') AS request_host,
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
  client_address,
  count() AS request_count
FROM otel_logs
WHERE startsWith(JSONExtractString(access_log, 'logger'), 'http.log.access')
  AND lower(splitByChar(':', request_host)[1]) = lower({domain:String})
  AND client_address != ''
  AND Timestamp >= toDateTime({since_seconds:UInt32})
GROUP BY client_address
ORDER BY request_count DESC, client_address
LIMIT 100
FORMAT JSONEachRow
