You are operating within a andurel project. You always have access to the andure cli through `andurel`.

The project is scaffolded with inertia/svelte5. Make sure to use the svelte related skills whenever inside .svelte files.

We are in development at the moment. Never worry about backwards compatibility.

Always follow the results of `go fix ./...`

Never add business logic in the database layer, i.e. in migrations. That means no:
- CHECK
- CONSTRAINT (except if it's for a foreign key)
- INDEXES for performance
