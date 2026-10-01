// Offline checks only. Existing local toolchains/images; no release or account authorization.
import assert from 'node:assert/strict';
import {createHash,randomUUID} from 'node:crypto';
import {lstatSync,mkdirSync,readFileSync,readdirSync,writeFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {spawnSync} from 'node:child_process';
assert.equal(process.platform,'darwin');assert.equal(process.arch,'arm64');
const id=randomUUID(),root='.local/cli-runtimes/'+id;mkdirSync(root,{recursive:true,mode:0o700});
const checksums=readFileSync('.local/cli-build/SHA256SUMS','utf8');
const expected=['pds-ass-darwin-amd64','pds-ass-darwin-arm64','pds-ass-linux-amd64','pds-ass-linux-arm64','pds-ass-windows-amd64.exe'];
const hashes=new Map();
for(const line of checksums.trim().split('\n')) {const match=/^([a-f0-9]{64})  (pds-ass-(?:darwin|linux|windows)-(?:amd64|arm64)(?:\.exe)?)$/.exec(line);assert.ok(match);assert.ok(!hashes.has(match[2]));hashes.set(match[2],match[1]);}
assert.deepEqual([...hashes.keys()].sort(),expected);
function digest(path){const info=lstatSync(path);assert.ok(info.isFile()&&!info.isSymbolicLink());return createHash('sha256').update(readFileSync(path)).digest('hex');}
for(const [name,hash] of hashes)assert.equal(digest('.local/cli-build/'+name),hash);
const image='sha256:18cfe3ef5e6815560c98237d6216d1e5119702fb0f3894c8785dd58b8bbe5d73';
const inspect=spawnSync('docker',['image','inspect',image,'--format','{{.Architecture}}'],{encoding:'utf8',timeout:10000});assert.equal(inspect.status,0);assert.equal(inspect.stdout.trim(),'arm64');
function resources(){const ps=spawnSync('docker',['ps','-aq'],{encoding:'utf8',timeout:10000}),volumes=spawnSync('docker',['volume','ls','--format','{{.Name}}'],{encoding:'utf8',timeout:10000});assert.equal(ps.status,0);assert.equal(volumes.status,0);return {containers:ps.stdout.trim().split('\n').filter(Boolean).sort(),volumes:volumes.stdout.trim().split('\n').filter(Boolean).sort()};}
const before=resources(),matrix=[];
for(const platform of ['darwin-arm64','darwin-amd64','linux-arm64']) {
 const binary=resolve('.local/cli-build/pds-ass-'+platform),config=resolve(root+'/config-'+platform);mkdirSync(config,{mode:0o700});
 const prefix=platform==='linux-arm64'?['run','--rm','--pull','never','--network','none','--read-only','--cap-drop','ALL','--security-opt','no-new-privileges','--pids-limit','32','--memory','64m','--cpus','.25','--user','65534:65534','--label','pds-ass.disposable-cli=true','--tmpfs','/tmp:rw,noexec,nosuid,size=1048576,mode=1777','--mount','type=bind,source='+binary+',target=/usr/local/bin/pds-ass,readonly','--entrypoint','/usr/local/bin/pds-ass',image]:platform==='darwin-amd64'?['-x86_64',binary]:[];
 const command=platform==='linux-arm64'?'docker':platform==='darwin-amd64'?'arch':binary;
 const cases=[{args:['--version'],exit:0,stdout:/^0\.1\.0-local\n$/},{args:['--help'],exit:0,stdout:/No public API endpoint is configured/},{args:['deploy','--help'],exit:0,stdout:/idempotency-key/},{args:['completion','bash'],exit:0,stdout:/complete/},{args:['completion','zsh'],exit:0,stdout:/compdef/},{args:['completion','fish'],exit:0,stdout:/complete/},{args:['unknown-command'],exit:2,stderr:/Unknown command/},{args:['status','--config-dir',platform==='linux-arm64'?'/tmp/qualification':config],exit:1,stderr:/No saved CLI sign-in\. Run login first\./}];
 const passed=[];
 for(const test of cases) {
  const started=performance.now(),result=spawnSync(command,[...prefix,...test.args],{encoding:'utf8',input:'',timeout:15000,maxBuffer:1024*1024});
  assert.equal(result.status,test.exit,platform+' '+test.args[0]+' exit');
  if(test.stdout)assert.match(result.stdout,test.stdout);
  if(test.stderr){assert.match(result.stderr,test.stderr);assert.equal(result.stdout,'');}else assert.equal(result.stderr,'');
  passed.push({command:test.args[0],exit:result.status,milliseconds:Math.round(performance.now()-started)});
 }
 assert.equal(lstatSync(config).mode&0o777,0o700);assert.deepEqual(readdirSync(config),[]);
 matrix.push({platform,runtime:platform==='darwin-amd64'?'Rosetta translation':platform==='linux-arm64'?'existing isolated Docker image':'native Apple Silicon',cases:passed});
}
assert.deepEqual(resources(),before);
assert.equal(readFileSync('.local/cli-build/SHA256SUMS','utf8'),checksums);for(const [name,hash] of hashes)assert.equal(digest('.local/cli-build/'+name),hash);
const report={id,completedAt:new Date().toISOString(),matrix,allFiveChecksumsMatched:true,binariesUnchanged:true,offlineOnly:true,existingContainersAndVolumesUnchanged:true,accountAuthorizationQualified:false,releasePublished:false,remaining:['Intel hardware','Linux AMD64 runtime','Windows runtime and credential ACLs','signed publication and installation','live onboarding']};writeFileSync(root+'/report.json',JSON.stringify(report,null,2),{mode:0o600,flag:'wx'});console.log(JSON.stringify(report));
