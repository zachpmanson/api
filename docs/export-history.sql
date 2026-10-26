-- OFFLINE REFERENCE ONLY: run only after separate owner approval for export.
-- Legacy Prisma DateTime is PostgreSQL timestamp(3) without time zone in UTC.
-- Verify the actual column type/UTC convention before using this query.
-- A timestamptz column is also rendered in UTC by the session setting below.
SET TIME ZONE 'UTC';
BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT COALESCE(json_agg(json_build_object(
  'id', id,
  'date', to_char("date", 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
  'deployment', deployment::text
) ORDER BY "date", id), '[]'::json)
FROM public."BrokenDeployment";
COMMIT;
