import assert from 'node:assert/strict';
import { cpSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';
import test from 'node:test';

test('dev.sh keeps listener TLS and invitation encryption keys separate', () => {
  const root = mkdtempSync(join(tmpdir(), 'prichal-dev-key-'));
  try {
    mkdirSync(join(root, 'scripts'));
    mkdirSync(join(root, 'backend'));
    mkdirSync(join(root, 'bin'));
    for (const name of ['dev.sh', 'init-enrollment-key.mjs', 'init-export-store.mjs']) {
      cpSync(new URL(name, import.meta.url), join(root, 'scripts', name));
    }
    writeFileSync(join(root, 'bin', 'go'), '#!/usr/bin/env node\nconsole.log(JSON.stringify({listener:process.env.PDS_ENROLLMENT_LISTENER_KEY_FILE,invitation:process.env.PDS_ENROLLMENT_KEY_FILE}));\n', {mode: 0o755});
    const base = {...process.env, PATH: join(root, 'bin') + ':' + process.env.PATH};
    delete base.PDS_ENROLLMENT_LISTENER_KEY_FILE;
    delete base.PDS_ENROLLMENT_KEY_FILE;
    for (const env of [
      {...base, PDS_ENROLLMENT_LISTENER_KEY_FILE: '/tls/new.key'},
      {...base, PDS_ENROLLMENT_KEY_FILE: '/tls/legacy.key'},
      {...base, PDS_ENROLLMENT_LISTENER_KEY_FILE: '/tls/new.key', PDS_ENROLLMENT_KEY_FILE: '/tls/legacy.key'},
    ]) {
      const result = spawnSync('bash', ['scripts/dev.sh', 'node-admin', 'serve'], {cwd: root, env, encoding: 'utf8'});
      assert.equal(result.status, 0, result.stderr);
      const received = JSON.parse(result.stdout);
      assert.equal(received.listener, env.PDS_ENROLLMENT_LISTENER_KEY_FILE || env.PDS_ENROLLMENT_KEY_FILE);
      assert.equal(received.invitation, join(root, '.local', 'enrollment.key'));
      assert.notEqual(received.listener, received.invitation, 'node-admin serve received the invitation encryption key');
    }
    assert.equal(readFileSync(join(root, '.local', 'enrollment.key')).length, 32);
  } finally { rmSync(root, {recursive: true, force: true}); }
});
