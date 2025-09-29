ALTER TABLE schema_migrations ENABLE ROW LEVEL SECURITY;

CREATE POLICY allow_all_schema_migrations
ON schema_migrations
FOR ALL
TO authenticated, anon
USING (true)
WITH CHECK (true);
