CREATE DATABASE my_local;

CREATE USER api_admin WITH PASSWORD 'password_for_dev_only';
CREATE USER api_worker WITH PASSWORD 'password_for_dev_only';
REVOKE CONNECT ON DATABASE my_local FROM PUBLIC;
GRANT CONNECT ON DATABASE my_local TO api_admin;
GRANT CONNECT ON DATABASE my_local TO api_worker;

\c my_local;
--geo extensions
CREATE EXTENSION postgis;
CREATE EXTENSION postgis_topology;

--some separate schema to isolate access at db level
CREATE SCHEMA api;
CREATE SCHEMA administrative;

--administrative access including schema changes
--geo access in public
GRANT USAGE ON SCHEMA public TO api_admin;
GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO api_admin;
GRANT SELECT ON TABLE public.spatial_ref_sys TO api_admin;
----
GRANT USAGE, CREATE ON SCHEMA api TO api_admin;
GRANT ALL ON ALL TABLES IN SCHEMA api TO api_admin;
GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA api TO api_admin;
GRANT USAGE, CREATE ON SCHEMA administrative TO api_admin;
GRANT ALL ON ALL TABLES IN SCHEMA administrative TO api_admin;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA api TO api_admin;
GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA administrative TO api_admin;
ALTER USER api_admin SET search_path = api, administrative, public;
ALTER USER postgres SET search_path = api, administrative, public;

--api worker CRUD access
--geo access in public
GRANT USAGE ON SCHEMA public TO api_worker;
GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO api_worker;
GRANT SELECT ON TABLE public.spatial_ref_sys TO api_worker;
----
GRANT USAGE ON SCHEMA api TO api_worker;
GRANT SELECT, INSERT, UPDATE ON ALL TABLES IN SCHEMA api TO api_worker;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA api TO api_worker;
GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA api TO api_worker;

--access on schema changes that occur after the above is executed
ALTER DEFAULT PRIVILEGES IN SCHEMA api GRANT ALL ON TABLES TO api_admin;
ALTER DEFAULT PRIVILEGES IN SCHEMA api GRANT USAGE, SELECT ON SEQUENCES TO api_admin;
ALTER DEFAULT PRIVILEGES IN SCHEMA api GRANT SELECT, INSERT, UPDATE ON TABLES TO api_worker;
ALTER DEFAULT PRIVILEGES IN SCHEMA api GRANT USAGE, SELECT ON SEQUENCES TO api_worker;

ALTER USER api_worker SET search_path = api, public;
SET search_path = "$user", api, administrative, public;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'subscriber_type') THEN
        CREATE TYPE api.subscriber_type AS ENUM ('shopper', 'business', 'driver', 'champion', 'donor', 'developer');
    END IF;
END$$;

CREATE TABLE IF NOT EXISTS api.subscribers (
    id SERIAL PRIMARY KEY,
    email VARCHAR(255) NOT NULL,
    name VARCHAR(255),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);


CREATE TABLE IF NOT EXISTS api.subscriber_types (
    id SERIAL PRIMARY KEY,
    subscriber_id INT NOT NULL REFERENCES subscribers(id),
    name subscriber_type NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
