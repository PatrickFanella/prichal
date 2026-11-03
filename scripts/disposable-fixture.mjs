// Teardown for disposable local qualification fixtures. Each resource is removed
// only when it carries the run's exact ownership label; absent resources are skipped.
// Set PRICHAL_KEEP_FIXTURES=1 to keep stopped containers, networks and volumes for debugging.
import { spawnSync } from 'node:child_process';

export function keepFixtures(env = process.env) {
  return env.PRICHAL_KEEP_FIXTURES === '1' || env.PRICHAL_KEEP_FIXTURES === 'true';
}

function dockerStatus(args) {
  // Docker errors can include environment values; keep no raw diagnostic output.
  const result = spawnSync('docker', args, { encoding: 'utf8', timeout: 60000, maxBuffer: 2 * 1024 * 1024 });
  return { status: result.status, stdout: result.stdout ?? '' };
}

const kinds = {
  container: {
    inspect: (key, name) => ['inspect', '--type', 'container', '--format', `{{index .Config.Labels "${key}"}}`, name],
    remove: name => ['rm', '--force', '--volumes', name],
  },
  network: {
    inspect: (key, name) => ['network', 'inspect', '--format', `{{index .Labels "${key}"}}`, name],
    remove: name => ['network', 'rm', name],
  },
  volume: {
    inspect: (key, name) => ['volume', 'inspect', '--format', `{{index .Labels "${key}"}}`, name],
    remove: name => ['volume', 'rm', name],
  },
};

// Containers go first because they hold network endpoints and volume mounts.
export function removeFixture({ owner, containers = [], networks = [], volumes = [] }, run = dockerStatus) {
  const [key, value] = owner ?? [];
  if (typeof key !== 'string' || !key || typeof value !== 'string' || !value) throw new Error('Fixture ownership label required');
  const result = { removed: [], absent: [], refused: [], failed: [] };
  for (const [kind, names] of [['container', containers], ['network', networks], ['volume', volumes]]) {
    for (const name of new Set(names)) {
      const label = run(kinds[kind].inspect(key, name));
      if (label.status !== 0) { result.absent.push(name); continue; }
      if (label.stdout.trim() !== value) { result.refused.push(name); continue; }
      (run(kinds[kind].remove(name)).status === 0 ? result.removed : result.failed).push(name);
    }
  }
  return result;
}
