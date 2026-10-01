#!/usr/bin/env bash
# Prepare binaries and checksums locally. This does not publish a release.
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p .local/cli-build
for target in darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64; do
  platform=${target%/*}
  architecture=${target#*/}
  suffix=
  if [[ $platform == windows ]]; then suffix=.exe; fi
  (cd backend; CGO_ENABLED=0 GOOS="$platform" GOARCH="$architecture" go build -trimpath -o "../.local/cli-build/pds-ass-$platform-$architecture$suffix" ./cmd/pds-ass)
done
node --input-type=module <<'JS'
import {createHash} from 'node:crypto';
import {readdirSync,readFileSync,writeFileSync} from 'node:fs';
const root='.local/cli-build';
const binaries=readdirSync(root).filter(name=>/^pds-ass-(darwin|linux|windows)-/.test(name)).sort();
const checksums=binaries.map(name=>createHash('sha256').update(readFileSync(root+'/'+name)).digest('hex')+'  '+name).join('\n')+'\n';
writeFileSync(root+'/SHA256SUMS',checksums);
console.log(JSON.stringify({binaries:binaries.length,checksumsPrepared:true,published:false}));
JS
