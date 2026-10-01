import assert from 'node:assert/strict';
// Private stable row digests. Never return the rows or authentication values themselves.
export function managementInventorySQL(tables){
 assert.ok(Array.isArray(tables)&&tables.length>0&&tables.length<=100);assert.equal(new Set(tables).size,tables.length);tables.forEach(table=>assert.match(table,/^[a-z][a-z0-9_]{0,62}$/));
 return `SELECT json_object_agg(table_name,inventory ORDER BY table_name) FROM (VALUES ${tables.map(table=>`('${table}',json_build_object('count',(SELECT count(*) FROM ${table}),'sha256',(SELECT encode(sha256(convert_to(coalesce(string_agg(row_to_json(snapshot)::text, E'\\n' ORDER BY row_to_json(snapshot)::text),''),'UTF8')),'hex') FROM ${table} snapshot)))`).join(',')}) AS entries(table_name,inventory)`;
}

export const managementSequenceSQL="SELECT coalesce(json_agg(json_build_object('name',sequencename,'lastValue',last_value) ORDER BY sequencename),'[]'::json) FROM pg_sequences WHERE schemaname='public'";
