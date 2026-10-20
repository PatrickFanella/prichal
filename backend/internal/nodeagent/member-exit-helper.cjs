// Native member deactivation primitive. The host broker must enforce the migration
// window and resolve a retained accepted invitation before invoking this helper.
const [action, did, ...extra] = process.argv.slice(1);
const didPattern = /^did:plc:[a-z2-7]{24}$/;
async function call(method, body) {
  const response = await fetch("http://127.0.0.1:2583/xrpc/" + method, {
    method: body === undefined ? "GET" : "POST",
    headers: { Authorization: "Basic " + Buffer.from("admin:" + process.env.PDS_ADMIN_PASSWORD).toString("base64"), ...(body === undefined ? {} : { "Content-Type": "application/json" }) },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }), signal: AbortSignal.timeout(5000),
  });
  if (!response.ok) throw Error("Native member status unavailable");
  const text = await response.text();
  if (text.length > 8192) throw Error("Native member status exceeds bound");
  return text ? JSON.parse(text) : undefined;
}
function inventory() {
  const { DatabaseSync } = require("node:sqlite");
  const db = new DatabaseSync("/pds/account.sqlite", { readOnly: true });
  try {
    db.exec("BEGIN");
    const owner = db.prepare("SELECT did FROM actor ORDER BY createdAt,did LIMIT 1").get();
    const member = db.prepare("SELECT did,handle,deactivatedAt,deleteAfter FROM actor WHERE did=?").get(did);
    const invitation = db.prepare("SELECT usedBy FROM invite_code_use WHERE usedBy=? LIMIT 1").get(did);
    if (!owner || owner.did === did || !member || !invitation || invitation.usedBy !== did || typeof member.handle !== "string" || !member.handle || member.handle.length > 253) throw Error("Invited non-owner account required");
    return member;
  } finally { db.close(); }
}
async function main() {
  if (process.env.PDS_DEV_MODE !== "true" || process.env.PDS_DID_PLC_URL !== "http://pds-ass-protocol-fixtures:2582" || !["native_status", "deactivate"].includes(action) || !didPattern.test(did ?? "") || extra.length) throw Error("Fixed isolated member operation required");
  let member = inventory();
  const status = () => call("com.atproto.admin.getSubjectStatus?did=" + encodeURIComponent(did));
  const valid = value => value?.subject?.$type === "com.atproto.admin.defs#repoRef" && value.subject.did === did && typeof value.deactivated?.applied === "boolean" && typeof value.takedown?.applied === "boolean";
  let current = await status();
  if (!valid(current) || current.deactivated.applied !== (member.deactivatedAt !== null)) throw Error("Native member identity/status differs");
  // Preserve existing user deactivation/deletion scheduling. Never reactivate,
  // clear moderation or schedule deletion. Takedown needs separate review.
  if (action === "deactivate" && !current.deactivated.applied) {
    if (current.takedown.applied || member.deleteAfter !== null) throw Error("Existing native policy requires review");
    await call("com.atproto.admin.updateSubjectStatus", { subject: { $type: "com.atproto.admin.defs#repoRef", did }, deactivated: { applied: true } });
    current = await status(); member = inventory();
    if (!valid(current) || !current.deactivated.applied || current.takedown.applied || member.deactivatedAt === null || member.deleteAfter !== null) throw Error("Native member deactivation unconfirmed");
  }
  return { did, handle: member.handle, deactivated: current.deactivated.applied };
}
main().then(value => process.stdout.write(JSON.stringify(value)), () => { process.stderr.write("Native member deactivation could not be confirmed"); process.exitCode = 1; });
