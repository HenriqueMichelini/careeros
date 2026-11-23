-- Prepare only the dedicated disposable database, as its fixture administrator.
-- This script deliberately fails if the named roles or schema already exist.
BEGIN;
DO $fixture$
BEGIN
 IF current_database() <> 'careeros_pilot_runtime_fixture' THEN
  RAISE EXCEPTION 'dedicated pilot fixture database required';
 END IF;
END $fixture$;

CREATE ROLE careeros_pilot_migrator LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE
 NOINHERIT NOREPLICATION NOBYPASSRLS CONNECTION LIMIT 2;
CREATE ROLE careeros_pilot_runtime LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE
 NOINHERIT NOREPLICATION NOBYPASSRLS CONNECTION LIMIT 8;
REVOKE ALL ON DATABASE careeros_pilot_runtime_fixture FROM PUBLIC;
GRANT CONNECT ON DATABASE careeros_pilot_runtime_fixture TO careeros_pilot_migrator,careeros_pilot_runtime;
REVOKE ALL ON SCHEMA public FROM PUBLIC;
CREATE SCHEMA careeros_pilot_runtime AUTHORIZATION careeros_pilot_migrator;
GRANT USAGE ON SCHEMA careeros_pilot_runtime TO careeros_pilot_runtime;
ALTER ROLE careeros_pilot_migrator IN DATABASE careeros_pilot_runtime_fixture SET search_path = careeros_pilot_runtime, pg_catalog;
ALTER ROLE careeros_pilot_runtime IN DATABASE careeros_pilot_runtime_fixture SET search_path = careeros_pilot_runtime, pg_catalog;
COMMIT;
