// Local Docker Desktop recovery fixtures use bounded /24s, preserving retained networks.
import assert from 'node:assert/strict';
function range(cidr){
 assert.equal(typeof cidr,'string');if(cidr.includes(':'))return undefined;
 const [address,prefix,...extra]=cidr.split('/'),parts=address.split('.');assert.ok(extra.length===0&&parts.length===4&&parts.every(p=>/^\d{1,3}$/.test(p)&&Number(p)<=255)&&/^\d{1,2}$/.test(prefix??'')&&Number(prefix)<=32,'Valid Docker IPv4 subnet required');
 const value=parts.reduce((n,p)=>n*256+Number(p),0),size=2**(32-Number(prefix)),start=Math.floor(value/size)*size;return [start,start+size-1];
}
export function recoverySubnets(existing){
 assert.ok(Array.isArray(existing));const ranges=existing.map(range).filter(Boolean),chosen=[];
 for(let i=0;i<256&&chosen.length<2;i++){const subnet=`10.252.${i}.0/24`,candidate=range(subnet);if(ranges.every(current=>candidate[1]<current[0]||candidate[0]>current[1]))chosen.push(subnet)}
 assert.equal(chosen.length,2,'Two unused local recovery subnets required');return chosen;
}
export function composeLiterals(value){return JSON.stringify(value,(_key,item)=>typeof item==='string'?item.replaceAll('$',()=> '$$'):item)}
