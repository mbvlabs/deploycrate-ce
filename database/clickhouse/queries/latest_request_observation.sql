SELECT toString(toUnixTimestamp64Nano(max(ObservedAt))) AS timestamp_nanoseconds
FROM request_observations
FORMAT JSONEachRow
