-- This additive bootstrap has no Account write authority or test grant setup.
BEGIN;
DO $draft_bootstrap$
BEGIN
 IF current_database() <> 'careeros_294' OR current_user <> session_user
   OR current_user IN ('careeros_294_runtime','careeros_294_migrator','careeros_294_draft_writer','careeros_294_draft_attestor')
   OR NOT EXISTS (SELECT 1 FROM pg_namespace n JOIN pg_roles r ON r.oid=n.nspowner
                  WHERE n.nspname='careeros_294' AND r.rolname='careeros_294_migrator') THEN
  RAISE EXCEPTION 'isolated hosted Draft administrator required';
 END IF;
 IF (SELECT count(*) FROM pg_roles r
     WHERE r.rolname IN ('careeros_294_draft_writer','careeros_294_draft_attestor')
       AND r.rolcanlogin AND r.rolconnlimit BETWEEN 1 AND 2
       AND NOT (r.rolsuper OR r.rolcreaterole OR r.rolcreatedb OR r.rolinherit OR r.rolreplication OR r.rolbypassrls)
       AND NOT EXISTS (SELECT 1 FROM pg_auth_members m WHERE m.member=r.oid OR m.roleid=r.oid)
       AND NOT has_database_privilege(r.oid,current_database(),'CREATE,TEMP')
       AND NOT has_schema_privilege(r.oid,'careeros_294','USAGE,CREATE')) <> 2 THEN
  RAISE EXCEPTION 'independent restricted hosted Draft roles required';
 END IF;
END $draft_bootstrap$;
GRANT CONNECT ON DATABASE careeros_294 TO careeros_294_draft_writer,careeros_294_draft_attestor;
CREATE SCHEMA careeros_294_draft AUTHORIZATION careeros_294_migrator;
REVOKE ALL ON SCHEMA careeros_294_draft FROM PUBLIC;
GRANT USAGE ON SCHEMA careeros_294_draft TO careeros_294_draft_writer,careeros_294_draft_attestor;
COMMIT;
