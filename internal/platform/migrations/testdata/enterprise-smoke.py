"""Reproduce external migrations + a DML-only service on isolated PostgreSQL 15.

Run from the repo root after building okrs:goose-validation. Requires Docker,
Go, OpenSSL and Python 3 (stdlib only). All credentials and databases are disposable.
No existing database/container is modified. MIGRATIONS_TEST_IMAGE overrides image.
"""
import datetime
import json
import http.cookiejar
import os
from pathlib import Path
import re
import subprocess
import shutil
import tempfile
import time
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parents[4]
IMAGE = os.environ.get('MIGRATIONS_TEST_IMAGE', 'okrs:goose-validation')
PREFIX = f'okrs-enterprise-{os.getpid()}'
PG = PREFIX + '-pg'
containers = []
cookies = http.cookiejar.CookieJar()
http = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(cookies))


def command(*args, **kwargs):
    result = subprocess.run(args, text=True, capture_output=True, **kwargs)
    if result.returncode:
        raise RuntimeError(f'{args[0]} exited {result.returncode}: {result.stdout}\n{result.stderr}')
    return result.stdout.strip()


def sql(database, query, role='postgres', app='harness', check=True):
    result = subprocess.run([
        'docker', 'exec', '-e', 'PGPASSWORD=disposable', '-e', 'PGAPPNAME=' + app,
        '-e', 'PGSSLMODE=verify-full', '-e', 'PGSSLROOTCERT=/tmp/enterprise.crt',
        PG, 'psql', '-X', '-h', '127.0.0.1', '-U', role, '-d', database,
        '-v', 'ON_ERROR_STOP=1', '-At', '-c', query,
    ], text=True, capture_output=True)
    if check and result.returncode:
        raise RuntimeError(result.stderr)
    return result.stdout.strip() if check else result


def dsn(database, role, app):
    return f'postgres://{role}:disposable@127.0.0.1:5432/{database}?sslmode=verify-full&sslrootcert=/run/enterprise-ca.crt&application_name={app}'


def provision(database, schema='public'):
    migrator, service = database + '_migrator', database + '_service'
    sql('postgres', f"CREATE ROLE {migrator} LOGIN PASSWORD 'disposable' NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;")
    sql('postgres', f"CREATE ROLE {service} LOGIN PASSWORD 'disposable' NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS NOINHERIT;")
    sql('postgres', f'CREATE DATABASE {database} OWNER {migrator};')
    sql(database, f'''REVOKE ALL ON DATABASE {database} FROM PUBLIC;
        GRANT CONNECT ON DATABASE {database} TO {service};
        REVOKE ALL ON SCHEMA public FROM PUBLIC;''')
    if schema == 'public':
        sql(database, f'ALTER SCHEMA public OWNER TO {migrator}')
    else:
        sql(database, f'CREATE SCHEMA {schema} AUTHORIZATION {migrator}')
    sql(database, f'''GRANT USAGE ON SCHEMA {schema} TO {service};
        ALTER ROLE {migrator} IN DATABASE {database} SET search_path = {schema}, public;
        ALTER ROLE {service} IN DATABASE {database} SET search_path = {schema}, public;
        ALTER DEFAULT PRIVILEGES FOR ROLE {migrator} IN SCHEMA {schema}
            GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO {service};
        ALTER DEFAULT PRIVILEGES FOR ROLE {migrator} IN SCHEMA {schema}
            GRANT USAGE, SELECT ON SEQUENCES TO {service};''')
    return migrator, service


def history_grants(database, schema='public'):
    service = database + '_service'
    sql(database, f'''REVOKE ALL ON {schema}.goose_db_version FROM {service};
        GRANT SELECT ON {schema}.goose_db_version TO {service};
        REVOKE ALL ON SEQUENCE {schema}.goose_db_version_id_seq FROM {service};''')
    if sql(database, f"SELECT to_regclass('{schema}.schema_migrations') IS NOT NULL") == 't':
        sql(database, f'''REVOKE ALL ON {schema}.schema_migrations FROM {service};
            GRANT SELECT ON {schema}.schema_migrations TO {service};''')


def migrate(database, mode='app', target=None):
    args = ['docker', 'run', '--rm', '--network', 'container:' + PG,
            '--read-only', '--cap-drop=ALL', '--security-opt=no-new-privileges',
            '--user', '65534:65534', '-v', str(binary) + ':/migration-runner:ro',
            '-v', str(certificate) + ':/run/enterprise-ca.crt:ro',
            '-e', 'DATABASE_URL=' + dsn(database, database + '_migrator', 'migration_job'),
            '--entrypoint', '/migration-runner', IMAGE, mode]
    if target is not None:
        args.append(str(target))
    command(*args)


def history(database, schema='public'):
    return sql(database, f'SELECT row_to_json(v) FROM {schema}.goose_db_version v ORDER BY id')


def schema_dump(database):
    dump = command('docker', 'exec', PG, 'pg_dump', '-U', 'postgres', '-d', database,
                   '--schema-only', '--no-owner', '--no-privileges')
    return '\n'.join(line for line in dump.splitlines() if not line.startswith(('\\restrict', '\\unrestrict')))


def api(port, path, body=None, method=None, form=False):
    if not any(c.name == 'okr_csrf_token' for c in cookies):
        with http.open(f'http://127.0.0.1:{port}/api/v1/admin/teams', timeout=10) as response:
            response.read()
    if form:
        boundary = 'enterprise-smoke-boundary'
        data = ''.join(f'--{boundary}\r\nContent-Disposition: form-data; name="{k}"\r\n\r\n{v}\r\n'
                       for k, v in body.items()) + f'--{boundary}--\r\n'
        content_type = 'multipart/form-data; boundary=' + boundary
    else:
        data = json.dumps(body) if body is not None else None
        content_type = 'application/json'
    request = urllib.request.Request(f'http://127.0.0.1:{port}' + path,
                                     data=data.encode() if data is not None else None,
                                     headers={'Content-Type': content_type, 'X-CSRF-Token': next(c.value for c in cookies if c.name == 'okr_csrf_token')}, method=method)
    try:
        with http.open(request, timeout=10) as response:
            data = response.read()
            return json.loads(data) if data else None
    except urllib.error.HTTPError as error:
        raise RuntimeError(f'{request.get_method()} {path}: {error.code} {error.read().decode()}') from error


def start(database, replica=0, expect_ready=True):
    name = f'{PREFIX}-{database}-{replica}'
    port = 8080 + replica
    command('docker', 'run', '-d', '--name', name, '--network', 'container:' + PG,
            '--read-only', '--tmpfs', '/tmp:rw,noexec,nosuid,size=16m', '--cap-drop=ALL',
            '--security-opt=no-new-privileges', '--user', '65534:65534',
            '-v', str(certificate) + ':/run/enterprise-ca.crt:ro',
            '-e', 'DATABASE_URL=' + dsn(database, database + '_service', 'service' if expect_ready else 'negative_startup'),
            '-e', f'PORT={port}', '-e', 'TZ=UTC', '-e', 'AUTH_MODE=disabled', IMAGE)
    containers.append(name)
    deadline = time.monotonic() + 35
    while time.monotonic() < deadline:
        logs = command('docker', 'logs', name)
        records = []
        for line in logs.splitlines():
            try:
                records.append(json.loads(line))
            except ValueError:
                pass
        ready = any(r.get('event') == 'app_ready' for r in records)
        if ready:
            assert expect_ready, logs
            assert not any(r.get('level') == 'ERROR' for r in records), logs
            host_port = command('docker', 'port', PG, f'{port}/tcp').rsplit(':', 1)[1]
            return name, int(host_port)
        state = command('docker', 'inspect', '-f', '{{.State.Status}}', name)
        if state == 'exited':
            assert not expect_ready, logs
            assert command('docker', 'inspect', '-f', '{{.State.ExitCode}}', name) != '0', logs
            assert any(r.get('event') == 'migration' and r.get('level') == 'ERROR' for r in records), logs
            return name, None
        time.sleep(.2)
    raise RuntimeError(f'service startup timed out: {logs}')


def stop(name):
    command('docker', 'rm', '-f', name)
    containers.remove(name)


def privileges(database, schema='public'):
    service = database + '_service'
    assert sql(database, f'''SELECT NOT rolsuper AND NOT rolcreatedb AND NOT rolcreaterole
        AND NOT rolreplication AND NOT rolbypassrls FROM pg_roles WHERE rolname='{service}' ''') == 't'
    assert sql(database, f"SELECT NOT has_schema_privilege('{service}','{schema}','CREATE') AND NOT has_database_privilege('{service}','{database}','CREATE') AND NOT has_database_privilege('{service}','{database}','TEMP')") == 't'
    assert sql(database, f"SELECT count(*) FROM pg_auth_members WHERE member=(SELECT oid FROM pg_roles WHERE rolname='{service}')") == '0'
    assert sql(database, f"SELECT count(*) FROM pg_class WHERE relowner=(SELECT oid FROM pg_roles WHERE rolname='{service}')") == '0'
    for query in [f'CREATE TABLE {schema}.forbidden(id int)', f'ALTER TABLE {schema}.goals ADD COLUMN forbidden int',
                  'CREATE TEMP TABLE forbidden(id int)', f'DELETE FROM {schema}.goose_db_version',
                  f'UPDATE {schema}.goose_db_version SET version_id=version_id', f'INSERT INTO {schema}.goose_db_version(version_id,is_applied) VALUES(999,true)',
                  f'SET ROLE {database}_migrator']:
        result = sql(database, query, service, 'acl_probe', check=False)
        assert result.returncode != 0 and ('permission denied' in result.stderr or 'must be owner' in result.stderr), result.stderr


def crud(port, suffix):
    team = api(port, '/api/v1/admin/teams', {'name': 'Enterprise ' + suffix, 'type': 'team'})['id']
    today = datetime.date.today()
    period = api(port, '/api/v1/admin/periods', {'name': 'Enterprise ' + suffix,
                 'start_date': str(today - datetime.timedelta(days=1)),
                 'end_date': str(today + datetime.timedelta(days=90))})['id']
    goal = api(port, f'/api/v1/teams/{team}/goals', {'period_id': period, 'title': 'External migrations',
                 'priority': 'P1', 'weight': 100, 'work_type': 'Delivery', 'direction': 'TECH'})['id']
    result = api(port, f'/api/v1/goals/{goal}')
    assert result['goal']['title'] == 'External migrations' and result['goal']['direction'] == 'TECH'
    kr = api(port, f'/api/v1/goals/{goal}/key-results', {'title': 'DML only', 'weight': 100, 'kind': 'BOOLEAN'}, form=True)['id']
    disposable_kr = api(port, f'/api/v1/goals/{goal}/key-results', {'title': 'Delete probe', 'weight': 0, 'kind': 'BOOLEAN'}, form=True)['id']
    api(port, f'/api/v1/periods/{period}/teams/activate?scope=org', {})
    api(port, f'/api/v1/krs/{kr}/progress/boolean', {'done': True, 'note': 'Written by restricted service'})
    assert api(port, f'/api/v1/goals/{goal}')['goal']['progress'] == 100
    api(port, f'/api/v1/krs/{disposable_kr}', method='DELETE')
    disposable_team = api(port, '/api/v1/admin/teams', {'name': 'Delete probe', 'type': 'team'})['id']
    api(port, f'/api/v1/admin/teams/{disposable_team}', method='DELETE')
    return goal


def exercise(database, schema='public'):
    history_grants(database, schema)
    privileges(database, schema)
    before_history, before_schema = history(database, schema), schema_dump(database)
    first, port = start(database)
    goal = crud(port, database)
    second, second_port = start(database, 1)
    assert api(second_port, f'/api/v1/goals/{goal}')['goal']['progress'] == 100
    stop(first)
    first, port = start(database)
    persisted = api(port, f'/api/v1/goals/{goal}')['goal']
    assert persisted['title'] == 'External migrations' and persisted['direction'] == 'TECH' and persisted['progress'] == 100
    assert sql(database, f"SELECT count(*) > 0 AND bool_and(s.ssl) FROM pg_stat_activity a JOIN pg_stat_ssl s ON s.pid=a.pid WHERE a.usename='{database}_service'") == 't'
    stop(first)
    stop(second)
    assert history(database, schema) == before_history, 'service modified migration history'
    assert schema_dump(database) == before_schema, 'service modified schema'
    print(f'PASS {database}: external migration, DML CRUD, two replicas, restart; schema/history unchanged', flush=True)


with tempfile.TemporaryDirectory(prefix='okrs-enterprise-') as temporary:
    # The migration job shares only the migrations directory/image with the server.
    binary = Path(temporary) / 'migration-runner'
    command('go', 'build', '-o', str(binary), './internal/platform/migrations/testdata/enterprise-runner',
            cwd=ROOT, env={**os.environ, 'CGO_ENABLED': '0', 'GOOS': 'linux'})
    # Other users in the containers can traverse this disposable build directory.
    os.chmod(temporary, 0o755)
    certificate = Path(temporary) / 'server.crt'
    openssl = os.environ.get('OPENSSL', shutil.which('openssl'))
    if not openssl:
        raise RuntimeError('OpenSSL is required (or set OPENSSL to its executable path)')
    command(openssl, 'req', '-x509', '-nodes', '-newkey', 'rsa:2048',
            '-keyout', str(Path(temporary) / 'server.key'), '-out', str(certificate),
            '-days', '1', '-subj', '/CN=localhost', '-addext', 'subjectAltName=IP:127.0.0.1,DNS:localhost')
    try:
        command('docker', 'run', '-d', '--rm', '--name', PG, '-e', 'POSTGRES_PASSWORD=disposable',
                '-e', 'POSTGRES_HOST_AUTH_METHOD=scram-sha-256',
                '-p', '127.0.0.1::5432', '-p', '127.0.0.1::8080', '-p', '127.0.0.1::8081',
                '-v', temporary + ':/certificates:ro', '--entrypoint', '/bin/sh', 'postgres:15', '-c',
                'cp /certificates/server.key /tmp/enterprise.key && '
                'cp /certificates/server.crt /tmp/enterprise.crt && '
                'chown postgres:postgres /tmp/enterprise.key && chmod 600 /tmp/enterprise.key && '
                'exec /usr/local/bin/docker-entrypoint.sh postgres '
                '-c ssl=on -c ssl_cert_file=/tmp/enterprise.crt -c ssl_key_file=/tmp/enterprise.key '
                '-c log_statement=all -c "log_line_prefix=%m user=%u db=%d app=%a "')
        containers.append(PG)
        for _ in range(150):
            try:
                if sql('postgres', 'SELECT 1') == '1':
                    break
            except RuntimeError:
                time.sleep(.2)
        else:
            raise RuntimeError('PostgreSQL did not become ready')
        # Reject unencrypted TCP connections, including from the local container network.
        command('docker', 'exec', PG, 'sh', '-c',
                'printf "local all all trust\\nhostssl all all 0.0.0.0/0 scram-sha-256\\n'
                'hostssl all all ::/0 scram-sha-256\\nhostnossl all all 0.0.0.0/0 reject\\n'
                'hostnossl all all ::/0 reject\\n" > /var/lib/postgresql/data/pg_hba.conf')
        sql('postgres', 'SELECT pg_reload_conf()')
        assert sql('postgres', 'SELECT ssl FROM pg_stat_ssl WHERE pid=pg_backend_pid()') == 't'
        insecure = subprocess.run(['docker', 'exec', '-e', 'PGPASSWORD=disposable', '-e', 'PGSSLMODE=disable', PG,
                    'psql', '-h', '127.0.0.1', '-U', 'postgres', '-d', 'postgres', '-c', 'SELECT 1'], capture_output=True, text=True)
        assert insecure.returncode != 0 and 'reject' in insecure.stderr, insecure.stderr
        bad_password = subprocess.run(['docker', 'exec', '-e', 'PGPASSWORD=wrong', '-e', 'PGSSLMODE=verify-full',
                        '-e', 'PGSSLROOTCERT=/tmp/enterprise.crt', PG, 'psql', '-h', '127.0.0.1',
                        '-U', 'postgres', '-d', 'postgres', '-c', 'SELECT 1'], capture_output=True, text=True)
        assert bad_password.returncode != 0 and 'password authentication failed' in bad_password.stderr, bad_password.stderr
        assert sql('postgres', "SELECT count(*) FROM pg_hba_file_rules WHERE type <> 'local' AND auth_method='trust'") == '0'
        print(sql('postgres', 'SELECT version()') + '; TLS verify-full, SCRAM; plaintext/wrong password rejected', flush=True)

        provision('enterprise_clean')
        migrate('enterprise_clean')
        exercise('enterprise_clean')

        provision('enterprise_goose')
        migrate('enterprise_goose', 'goose')
        exercise('enterprise_goose')

        provision('enterprise_schema', 'okr')
        migrate('enterprise_schema')
        exercise('enterprise_schema', 'okr')

        for legacy_version in (49, 50):
            database = f'enterprise_legacy{legacy_version}'
            migrator, service = provision(database)
            fixture = ROOT / 'internal/platform/migrations/testdata/legacy/v49.sql'
            # Restore as migrator to preserve real object ownership (no superuser).
            command('docker', 'exec', '-i', '-e', 'PGPASSWORD=disposable', '-e', 'PGSSLMODE=verify-full',
                    '-e', 'PGSSLROOTCERT=/tmp/enterprise.crt', PG, 'psql', '-X',
                    '-h', '127.0.0.1', '-U', migrator, '-d', database,
                    '-v', 'ON_ERROR_STOP=1', input=fixture.read_text())
            if legacy_version == 50:
                original = command('git', 'show', '8dd5904:migrations/050_goal_direction.up.sql', cwd=ROOT)
                sql(database, original, migrator)
                sql(database, 'UPDATE schema_migrations SET version=50', migrator)
            # Only the migration job may create goose history, even on legacy databases.
            sql(database, f'REVOKE ALL ON schema_migrations FROM {service}; GRANT SELECT ON schema_migrations TO {service}')
            before_schema = schema_dump(database)
            negative, _ = start(database, expect_ready=False)
            stop(negative)
            assert schema_dump(database) == before_schema
            assert sql(database, "SELECT to_regclass('public.goose_db_version') IS NULL") == 't'
            migrate(database)
            assert sql(database, 'SELECT version FROM schema_migrations') == str(legacy_version)
            exercise(database)
        print('PASS legacy 049/050: restricted service cannot bootstrap history; external migration job imports it', flush=True)

        provision('enterprise_pending')
        migrate('enterprise_pending', target=49)
        history_grants('enterprise_pending')
        before = history('enterprise_pending')
        before_schema = schema_dump('enterprise_pending')
        negative, _ = start('enterprise_pending', expect_ready=False)
        stop(negative)
        assert history('enterprise_pending') == before
        assert schema_dump('enterprise_pending') == before_schema
        assert sql('enterprise_pending', "SELECT count(*) FROM information_schema.columns WHERE table_name='goals' AND column_name='direction'") == '0'
        migrate('enterprise_pending')
        exercise('enterprise_pending')
        print('PASS pending 049: restricted startup fails without readiness or history/schema changes; external 050 recovers service', flush=True)

        # postgres sends logs to stderr; inspect both streams.
        result = subprocess.run(['docker', 'logs', PG], capture_output=True, text=True, check=True)
        logs = result.stdout + result.stderr
        service_lines = [line for line in logs.splitlines() if 'app=service ' in line]
        assert service_lines, 'no server-side audit evidence for service sessions'
        ddl = re.compile(r':\s*(?:CREATE|ALTER|DROP|TRUNCATE|GRANT|REVOKE|COMMENT|REINDEX|VACUUM)\b', re.I)
        assert not any(ddl.search(line) for line in service_lines), 'service issued DDL'
        assert any('goose_db_version' in line for line in service_lines), 'history was not checked'
        assert any('INSERT INTO' in line.upper() for line in service_lines), 'no service DML captured'
        print('PASS PostgreSQL statement audit: ready service sessions issue no DDL; migration jobs own all schema changes', flush=True)
    finally:
        for container in reversed(containers):
            subprocess.run(['docker', 'rm', '-f', container], capture_output=True)
