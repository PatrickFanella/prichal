import test from 'node:test';
import assert from 'node:assert/strict';
import { keepFixtures, removeFixture } from './disposable-fixture.mjs';

function fakeDocker(objects) {
  const calls = [];
  const run = args => {
    calls.push(args);
    const name = args.at(-1);
    if (args[0] === 'rm' || args[1] === 'rm') {
      const object = objects[name];
      return { status: object?.removeFails ? 1 : 0, stdout: '' };
    }
    const object = objects[name];
    if (!object) return { status: 1, stdout: '' };
    const kind = args[0] === 'inspect' ? 'container' : args[0];
    assert.equal(object.kind, kind, `inspected ${name} as the wrong kind`);
    return { status: 0, stdout: `${object.label ?? ''}\n` };
  };
  return { run, calls };
}

test('fixtures are removed by default and kept only on explicit opt-in', () => {
  assert.equal(keepFixtures({}), false);
  assert.equal(keepFixtures({ PRICHAL_KEEP_FIXTURES: '0' }), false);
  assert.equal(keepFixtures({ PRICHAL_KEEP_FIXTURES: '1' }), true);
  assert.equal(keepFixtures({ PRICHAL_KEEP_FIXTURES: 'true' }), true);
});

test('only resources carrying the exact run label are removed, containers first', () => {
  const id = '11111111-2222-4333-8444-555555555555';
  const { run, calls } = fakeDocker({
    'fx-pds': { kind: 'container', label: id },
    'fx-edge': { kind: 'container', label: 'another-run' },
    'fx-net': { kind: 'network', label: id },
    'fx-data': { kind: 'volume', label: id },
    'fx-blobs': { kind: 'volume', label: id, removeFails: true },
  });
  const result = removeFixture({
    owner: ['tv.subcult.prichal.test', id],
    volumes: ['fx-data', 'fx-blobs', 'fx-protocol'],
    networks: ['fx-net'],
    containers: ['fx-pds', 'fx-edge', 'fx-pds'],
  }, run);
  assert.deepEqual(result, { removed: ['fx-pds', 'fx-net', 'fx-data'], absent: ['fx-protocol'], refused: ['fx-edge'], failed: ['fx-blobs'] });
  const removals = calls.filter(args => args.includes('rm')).map(args => args.at(-1));
  assert.deepEqual(removals, ['fx-pds', 'fx-net', 'fx-data', 'fx-blobs']);
  assert.ok(calls.some(args => args.join(' ') === 'rm --force --volumes fx-pds'));
  assert.ok(calls.every(args => !args.includes('fx-edge') || args[0] === 'inspect'), 'foreign container must not be removed');
});

test('an ownership label is mandatory', () => {
  assert.throws(() => removeFixture({ containers: ['x'] }, () => assert.fail('no Docker call expected')));
  assert.throws(() => removeFixture({ owner: ['key', ''], containers: ['x'] }, () => assert.fail('no Docker call expected')));
});
