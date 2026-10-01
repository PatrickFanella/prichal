// Local portable-export custody, separate from enrollment and native host keys.
import assert from 'node:assert/strict';import {mkdirSync,lstatSync,existsSync,writeFileSync,realpathSync} from 'node:fs';import {resolve} from 'node:path';import {randomBytes} from 'node:crypto';
const directory=resolve('.local/exports');mkdirSync(directory,{recursive:true,mode:0o700});const info=lstatSync(directory);assert.ok(info.isDirectory()&&!(info.mode&0o077)&&realpathSync(directory)===directory,'Private nonsymlink export directory required');
const key=resolve('.local/export.key');if(!existsSync(key))writeFileSync(key,randomBytes(32),{flag:'wx',mode:0o600});const keyInfo=lstatSync(key);assert.ok(keyInfo.isFile()&&!(keyInfo.mode&0o077)&&keyInfo.size===32,'Private export key required');
