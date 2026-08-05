We are in development at the moment. Never worry about backwards compatibility.

Always follow the results of `go fix ./...`

Never add business logic in the database layer, i.e. in migrations. That means no:
- CHECK
- CONSTRAINT (except if it's for a foreign key)
- INDEXES for performance
