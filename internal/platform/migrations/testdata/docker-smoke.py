"""Smoke the built image against an isolated PostgreSQL 15 container.

Set MIGRATIONS_TEST_DATABASE_URL for host psql, POSTGRES_CONTAINER for docker
network sharing, and optionally MIGRATIONS_TEST_IMAGE (default below).
"""
import json
import os
from pathlib import Path
import queue
import subprocess
import threading
import time
from urllib.parse import urlsplit, urlunsplit

ROOT = Path(__file__).resolve().parents[4]
ADMIN = os.environ['MIGRATIONS_TEST_DATABASE_URL']
PG_CONTAINER = os.environ['POSTGRES_CONTAINER']
IMAGE = os.environ.get('MIGRATIONS_TEST_IMAGE', 'okrs:goose-validation')
active = set()
databases = []


def sql(dsn, query):
    return subprocess.check_output(
        ['psql', dsn, '-v', 'ON_ERROR_STOP=1', '-At', '-c', query], text=True)


def database(suffix, fixture=None):
    name = f'docker_smoke_{os.getpid()}_{suffix}'
    sql(ADMIN, 'CREATE DATABASE ' + name)
    databases.append(name)
    parsed = urlsplit(ADMIN)
    dsn = urlunsplit((parsed.scheme, parsed.netloc, '/' + name,
                     parsed.query, parsed.fragment))
    if fixture:
        subprocess.run(['psql', dsn, '-v', 'ON_ERROR_STOP=1', '-f', str(
            ROOT / 'internal/platform/migrations/testdata/legacy' / fixture)],
            stdout=subprocess.DEVNULL, check=True)
    return name, dsn


def dump(dsn, schema=False):
    args = ['pg_dump', dsn, '--no-owner', '--no-privileges',
            '--exclude-table=schema_migrations', '--exclude-table=goose_db_version',
            '--exclude-table=goose_db_version_id_seq']
    if schema:
        args.append('--schema-only')
    text = subprocess.check_output(args, text=True)
    return '\n'.join(line for line in text.splitlines()
                     if line.strip() and not line.startswith(
                         ('--', '\\restrict', '\\unrestrict')))


def run_ready(dbname, seed=False):
    name = f'okrs-goose-smoke-{os.getpid()}-{dbname}'
    parsed = urlsplit(ADMIN)
    inside = urlunsplit((parsed.scheme, parsed.netloc.rsplit('@', 1)[0] +
                        '@127.0.0.1:5432', '/' + dbname,
                        parsed.query, parsed.fragment))
    args = ['docker', 'run', '--rm', '--name', name, '--network',
            'container:' + PG_CONTAINER, '-e', 'DATABASE_URL=' + inside,
            '-e', 'PORT=8080', '-e', 'TZ=UTC', '-e', 'AUTH_MODE=disabled', IMAGE]
    if seed:
        args.append('-seed')
    process = subprocess.Popen(args, stdout=subprocess.PIPE,
                               stderr=subprocess.STDOUT, text=True)
    active.add(name)
    messages = queue.Queue()

    def read():
        for line in process.stdout:
            messages.put(line)

    threading.Thread(target=read, daemon=True).start()
    deadline = time.monotonic() + 30
    records = []
    ready = False
    while time.monotonic() < deadline:
        try:
            line = messages.get(timeout=.25)
        except queue.Empty:
            if process.poll() is not None:
                raise RuntimeError('image exited before readiness')
            continue
        try:
            record = json.loads(line)
        except ValueError:
            continue
        records.append(record)
        if record.get('event') == 'app_ready':
            ready = True
            break
    if not ready:
        raise RuntimeError('image did not announce readiness')
    events = [record.get('event') for record in records]
    assert events.index('migration') < events.index('app_ready')
    assert not any(record.get('level') == 'ERROR' for record in records)
    subprocess.run(['docker', 'stop', '-t', '10', name],
                   stdout=subprocess.DEVNULL, check=True)
    assert process.wait(timeout=20) == 0
    active.remove(name)


LATEST = str(len(list((ROOT / 'migrations').glob('*.sql'))))
LEGACY = '49'  # last version of the golang-migrate history

try:
    name, dsn = database('new')
    run_ready(name)
    assert sql(dsn, 'SELECT max(version_id) FROM goose_db_version').strip() == LATEST
    schema = dump(dsn, True)
    before = dump(dsn)
    run_ready(name)
    assert dump(dsn) == before
    print('PASS image: clean PostgreSQL 15, /app/migrations, repeat startup')

    name, dsn = database('legacy', 'v' + LEGACY + '.sql')
    before = dump(dsn)
    run_ready(name)
    run_ready(name)
    assert dump(dsn) == before
    assert sql(dsn, 'SELECT version FROM schema_migrations WHERE NOT dirty').strip() == LEGACY
    print('PASS image: legacy ' + LEGACY + ' import and repeat, exact data/sequence preservation')

    name, dsn = database('seed')
    run_ready(name, True)
    assert dump(dsn, True) == schema
    assert int(sql(dsn, 'SELECT count(*) FROM goals').strip()) > 0
    print('PASS image: -seed creates demo goals and preserves application schema')
finally:
    for name in active:
        subprocess.run(['docker', 'rm', '-f', name], stdout=subprocess.DEVNULL)
    for name in databases:
        sql(ADMIN, 'DROP DATABASE ' + name + ' WITH (FORCE)')
