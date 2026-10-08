"""Run new/previous/new servers against isolated PostgreSQL 15 databases.

Set MIGRATIONS_TEST_DATABASE_URL. CURRENT_SERVER, PREVIOUS_SERVER and
PREVIOUS_ROOT optionally override the temporary binaries/source checkout.
"""
import subprocess,os,json,time,threading,queue,signal,socket
from pathlib import Path
from urllib.parse import urlsplit,urlunsplit
root=str(Path(__file__).resolve().parents[4])
admin=os.environ['MIGRATIONS_TEST_DATABASE_URL']
current=os.environ.get('CURRENT_SERVER','/tmp/okrs-goose-current-server')
previous=os.environ.get('PREVIOUS_SERVER','/tmp/okrs-goose-previous-server')
previous_root=os.environ.get('PREVIOUS_ROOT','/tmp/okrs-goose-old-release')
def sql(dsn,query):return subprocess.check_output(['psql',dsn,'-v','ON_ERROR_STOP=1','-At','-c',query],text=True)
def database(name,fixture=None):
 sql(admin,'CREATE DATABASE '+name)
 u=urlsplit(admin);dsn=urlunsplit((u.scheme,u.netloc,'/'+name,u.query,u.fragment))
 if fixture:subprocess.run(['psql',dsn,'-v','ON_ERROR_STOP=1','-f',root+'/internal/platform/migrations/testdata/legacy/'+fixture],stdout=subprocess.DEVNULL,check=True)
 return dsn

def snapshot(dsn):
 out=subprocess.check_output(['pg_dump',dsn,'--no-owner','--no-privileges','--exclude-table=schema_migrations','--exclude-table=goose_db_version','--exclude-table=goose_db_version_id_seq'],text=True)
 return '\n'.join(line for line in out.splitlines() if not line.startswith(('--','\\restrict','\\unrestrict')) and line.strip())

def start(binary,dsn,seed=False):
 sock=socket.socket();sock.bind(('127.0.0.1',0));port=sock.getsockname()[1];sock.close()
 env=os.environ.copy();env.update(DATABASE_URL=dsn,PORT=str(port),TZ='UTC',LOG_FORMAT='json',AUTH_MODE='disabled')
 p=subprocess.Popen([binary]+(['-seed'] if seed else []),cwd=(previous_root if binary==previous else root),env=env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
 processes.append(p)
 messages=queue.Queue()
 def read():
  for line in p.stdout:messages.put(line)
 threading.Thread(target=read,daemon=True).start()
 deadline=time.monotonic()+30;logs=[]
 while time.monotonic()<deadline:
  try:line=messages.get(timeout=.25)
  except queue.Empty:
   if p.poll() is not None:raise RuntimeError('server exited: '+''.join(logs))
   continue
  logs.append(line)
  try:record=json.loads(line)
  except ValueError:continue
  if record.get('event')=='app_ready':return p,messages,logs
 p.terminate();p.wait();raise RuntimeError('no readiness: '+''.join(logs))

def stop(server):
 p,messages,logs=server;p.send_signal(signal.SIGTERM);code=p.wait(timeout=20)
 if code!=0:raise RuntimeError('server shutdown '+str(code))
 return logs

legacy='49'  # last version of the golang-migrate history
latest=str(len(list(Path(root,'migrations').glob('*.sql'))))
names=[]
processes=[]
try:
 name='release_smoke_'+str(os.getpid());names.append(name);dsn=database(name,'v'+legacy+'.sql');before=snapshot(dsn)
 for binary in [current,previous,current]:
  stop(start(binary,dsn))
  assert snapshot(dsn)==before,'application state changed after '+binary
 print('PASS actual server new -> previous revision -> new; all ready, all data/sequences preserved')
 newer=start(current,dsn);older=start(previous,dsn);stop(newer);stop(older)
 assert snapshot(dsn)==before
 print('PASS mixed actual servers on legacy version '+legacy)
 for seed in [False,True]:
  name='new_smoke_'+str(os.getpid())+('_seed' if seed else '');names.append(name);dsn=database(name)
  stop(start(current,dsn,seed))
  assert sql(dsn,'SELECT max(version_id) FROM goose_db_version').strip()==latest
  print('PASS actual new database startup'+(' with -seed' if seed else ''))
finally:
 for p in processes:
  if p.poll() is None:p.terminate();p.wait(timeout=20)
 for name in names:sql(admin,'DROP DATABASE '+name+' WITH (FORCE)')
