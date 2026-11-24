-- Apply the bootstrap to an isolated candidate database only, after the
-- approved administrator has created the dedicated login roles and DB.
BEGIN;
DO $candidate$
BEGIN
 IF current_database() <> 'careeros_294' OR current_user <> session_user
   OR current_user IN ('careeros_294_runtime','careeros_294_migrator') THEN
  RAISE EXCEPTION 'isolated hosted candidate administrator required';
 END IF;
 IF (SELECT count(*) FROM pg_roles r
     WHERE r.rolname IN ('careeros_294_runtime','careeros_294_migrator')
       AND r.rolcanlogin AND r.rolconnlimit BETWEEN 1 AND CASE r.rolname WHEN 'careeros_294_runtime' THEN 8 ELSE 2 END
       AND NOT (r.rolsuper OR r.rolcreaterole OR r.rolcreatedb OR r.rolinherit OR r.rolreplication OR r.rolbypassrls)
       AND NOT EXISTS (SELECT 1 FROM pg_auth_members membership WHERE membership.member=r.oid)) <> 2 THEN
  RAISE EXCEPTION 'distinct restricted hosted roles required';
 END IF;
END $candidate$;

REVOKE ALL ON DATABASE careeros_294 FROM PUBLIC;
GRANT CONNECT ON DATABASE careeros_294 TO careeros_294_runtime,careeros_294_migrator;
REVOKE ALL ON SCHEMA public FROM PUBLIC;
CREATE SCHEMA careeros_294 AUTHORIZATION careeros_294_migrator;
GRANT USAGE ON SCHEMA careeros_294 TO careeros_294_runtime;
COMMIT;
