-- Pre-install pgcrypto (committed) so the system migrations
-- "CREATE EXTENSION IF NOT EXISTS pgcrypto" on the aux and data
-- connections become instant no-ops. Without this, the nested
-- transactions (aux wraps data) deadlock on a fresh database
-- because the aux tx inserts an uncommitted pg_extension row that
-- the data tx's CREATE EXTENSION blocks on. See tests/app.go.
CREATE EXTENSION IF NOT EXISTS pgcrypto;
