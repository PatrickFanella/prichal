// Fixed operator helper runs inside the isolated PDS; its administrator key stays there.
const [action] = process.argv.slice(1);
async function call(method, body) {
  const response = await fetch('http://127.0.0.1:2583/xrpc/' + method, {
    method: body === undefined ? 'GET' : 'POST',
    headers: { Authorization: 'Basic ' + Buffer.from('admin:' + process.env.PDS_ADMIN_PASSWORD).toString('base64'), ...(body === undefined ? {} : { 'Content-Type': 'application/json' }) },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }), signal: AbortSignal.timeout(5000),
  });
  if (!response.ok) throw new Error('Native enrollment operation unavailable');
  const text = await response.text(); return text ? JSON.parse(text) : undefined;
}
async function inventory() {
  const { DatabaseSync } = require('node:sqlite');
  const database = new DatabaseSync('/pds/account.sqlite', { readOnly: true });
  const actors = database.prepare('SELECT did FROM actor LIMIT 2').all();
  database.close();
  if (actors.length > 1) throw new Error('First-account ceiling exceeded');
  if (!actors.length) return { count: 0 };
  const info = await call('com.atproto.admin.getAccountInfo?did=' + encodeURIComponent(actors[0].did));
  return { count: 1, did: info.did, handle: info.handle, emailConfirmed: !!info.emailConfirmedAt };
}
(async () => {
  if (process.env.PDS_DEV_MODE !== 'true' || process.env.PDS_DID_PLC_URL !== 'http://pds-ass-protocol-fixtures:2582') throw new Error('Isolated native PDS required');
  if (action === 'inventory') return inventory();
  if (action !== 'invitation' && action !== 'withdraw') throw new Error('Unsupported native enrollment action');
  const account = await inventory();
  if (account.count) return account;
  const codes = [];
  let cursor;
  for (let pageNumber = 0; pageNumber < 10; pageNumber++) {
    const page = await call('com.atproto.admin.getInviteCodes?limit=100' + (cursor ? '&cursor=' + encodeURIComponent(cursor) : ''));
    codes.push(...page.codes.filter(code => !code.disabled && code.available > code.uses.length).map(code => code.code));
    cursor = page.cursor;
    if (!cursor) break;
    if (pageNumber === 9) throw new Error('Native invitation inventory limit exceeded');
  }
  if (codes.length) await call('com.atproto.admin.disableInviteCodes', { codes });
  if (action === 'withdraw') return inventory();
  const invitation = await call('com.atproto.server.createInviteCode', { useCount: 1 });
  return { count: 0, code: invitation.code };
})().then(value => process.stdout.write(JSON.stringify(value)), () => { process.stderr.write('Native enrollment operation failed'); process.exitCode = 1; });
