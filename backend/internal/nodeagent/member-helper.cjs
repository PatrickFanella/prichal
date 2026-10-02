// Fixed member invitation helper runs inside the isolated PDS; its administrator key stays there.
// Native account creation checks and records a code use in one transaction, so a
// code disabled here cannot create an account afterwards.
const [action, ...rest] = process.argv.slice(1);
const codePattern = /^[A-Za-z0-9._:-]{1,256}$/;
async function call(method, body) {
  const response = await fetch('http://127.0.0.1:2583/xrpc/' + method, {
    method: 'POST',
    headers: { Authorization: 'Basic ' + Buffer.from('admin:' + process.env.PDS_ADMIN_PASSWORD).toString('base64'), 'Content-Type': 'application/json' },
    body: JSON.stringify(body), signal: AbortSignal.timeout(5000),
  });
  if (!response.ok) throw new Error('Native member operation unavailable');
  const text = await response.text(); return text ? JSON.parse(text) : undefined;
}
function read(tracked) {
  const { DatabaseSync } = require('node:sqlite');
  const database = new DatabaseSync('/pds/account.sqlite', { readOnly: true });
  try {
    database.exec('BEGIN');
    const accounts = database.prepare('SELECT count(*) AS count FROM actor').get().count;
    const owner = database.prepare('SELECT did FROM actor ORDER BY createdAt,did LIMIT 1').get();
    const usable = database.prepare('SELECT c.code FROM invite_code c WHERE c.disabled=0 AND c.availableUses>(SELECT count(*) FROM invite_code_use u WHERE u.code=c.code) LIMIT 1001').all().map(row => row.code);
    if (usable.length > 1000) throw new Error('Native invitation inventory limit exceeded');
    const codes = {};
    for (const code of tracked) {
      const row = database.prepare('SELECT disabled FROM invite_code WHERE code=?').get(code);
      const use = database.prepare('SELECT u.usedBy AS did,a.handle AS handle FROM invite_code_use u LEFT JOIN actor a ON a.did=u.usedBy WHERE u.code=? ORDER BY u.usedAt LIMIT 1').get(code);
      codes[code] = { known: !!row, disabled: !!row?.disabled, did: use?.did ?? '', handle: use?.handle ?? '' };
    }
    return { accounts, owner: owner?.did ?? '', usable, codes };
  } finally { database.close(); }
}
async function sweep(state, tracked) {
  const orphans = state.usable.filter(code => !tracked.includes(code));
  if (orphans.length) await call('com.atproto.admin.disableInviteCodes', { codes: orphans });
  const swept = read(tracked);
  if (swept.usable.some(code => !tracked.includes(code))) throw new Error('Untracked native invitation remains usable');
  return swept;
}
function summary(state, tracked) {
  return { accounts: state.accounts, owner: state.owner, usable: state.usable.filter(code => tracked.includes(code)).length, codes: state.codes };
}
(async () => {
  if (process.env.PDS_DEV_MODE !== 'true' || process.env.PDS_DID_PLC_URL !== 'http://pds-ass-protocol-fixtures:2582') throw new Error('Isolated native PDS required');
  if (action === 'sweep') {
    const tracked = rest;
    if (tracked.length > 25 || !tracked.every(code => codePattern.test(code))) throw new Error('Tracked member codes required');
    const state = read(tracked);
    if (state.accounts < 1) return { ...summary(state, tracked), ownerRequired: true };
    return summary(await sweep(state, tracked), tracked);
  }
  if (action === 'inventory' || action === 'disable') {
    const tracked = rest;
    if (tracked.length > 50 || !tracked.every(code => codePattern.test(code))) throw new Error('Tracked member codes required');
    if (action === 'disable' && tracked.length) await call('com.atproto.admin.disableInviteCodes', { codes: tracked });
    return summary(read(tracked), tracked);
  }
  if (action !== 'issue') throw new Error('Unsupported native member action');
  const [ceilingText, ...tracked] = rest;
  const ceiling = Number(ceilingText);
  if (!Number.isSafeInteger(ceiling) || ceiling < 1 || ceiling > 25 || tracked.length > 25 || !tracked.every(code => codePattern.test(code))) throw new Error('Member ceiling and tracked codes required');
  // Without an owner account, any usable code belongs to first-account enrollment.
  let state = read(tracked);
  if (state.accounts < 1) return { ...summary(state, tracked), ownerRequired: true };
  // Untracked usable codes would bypass the ceiling; disable them before counting.
  state = await sweep(state, tracked);
  if (state.accounts - 1 + state.usable.length >= ceiling) return { ...summary(state, tracked), ceilingReached: true };
  const invitation = await call('com.atproto.server.createInviteCode', { useCount: 1 });
  if (!codePattern.test(invitation?.code ?? '')) throw new Error('Native invitation response invalid');
  return { ...summary(state, tracked), code: invitation.code };
})().then(value => process.stdout.write(JSON.stringify(value)), () => { process.stderr.write('Native member operation failed'); process.exitCode = 1; });
