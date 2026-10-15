import argparse,gzip,hashlib,json,pathlib,subprocess,tempfile
root=pathlib.Path(__file__).resolve().parent
p=argparse.ArgumentParser();p.add_argument('label');a=p.parse_args()
manifest=json.loads((root/'manifest.json').read_text())
entry=next(x for x in manifest['entries'] if x['label']==a.label)
run_id=entry['workflow_run'].rsplit('/',1)[1]
metadata=json.loads(subprocess.check_output(['gh','run','view',run_id,'--repo','batchstream/weir-tests','--json','databaseId,headSha,headBranch,status,conclusion,url,createdAt,updatedAt,event,workflowName'],text=True))
if metadata['status']!='completed':raise SystemExit('workflow still running')
out=root/'raw'/a.label
out.mkdir(parents=True,exist_ok=False)
temporary=pathlib.Path(tempfile.mkdtemp(prefix='weir-coverage-download-'+a.label+'-'))
meta=temporary/'workflow.json';meta.write_text(json.dumps(metadata,indent=2)+'\n')
result=subprocess.run(['gh','run','download',run_id,'--repo','batchstream/weir-tests','--dir',str(temporary/'artifacts')],timeout=300)
if result.returncode:raise SystemExit(result.returncode)
inventory=[]
for source in sorted(temporary.rglob('*')):
    if source.is_symlink() or not source.is_file():continue
    if source.name.startswith('.env') or source.suffix in ('.key','.pem') or source.name=='credentials' or '.ssh' in source.parts:
        raise SystemExit('forbidden file in artifact; not read')
    relative=source.relative_to(temporary).as_posix()+'.gz'
    target=out/relative;target.parent.mkdir(parents=True,exist_ok=True)
    digest=hashlib.sha256();length=0
    with source.open('rb') as incoming,target.open('wb') as destination:
        with gzip.GzipFile(filename='',fileobj=destination,mode='wb',mtime=0,compresslevel=6) as packed:
            while chunk:=incoming.read(1<<20):
                digest.update(chunk);length+=len(chunk);packed.write(chunk)
    compressed=hashlib.sha256()
    with target.open('rb') as stream:
        while chunk:=stream.read(1<<20):compressed.update(chunk)
    inventory.append(dict(path=relative,gzip_sha256=compressed.hexdigest(),sha256=digest.hexdigest(),bytes=length))
(out/'files.json').write_text(json.dumps(inventory,indent=2)+'\n')
print(json.dumps(dict(label=a.label,run_id=run_id,files=len(inventory),head=metadata['headSha'],conclusion=metadata['conclusion'],compressed_bytes=sum((out/x['path']).stat().st_size for x in inventory))))
if metadata['headSha']!=manifest['client_source']:raise SystemExit('workflow source mismatch retained')
