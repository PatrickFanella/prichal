// Optional local-only browser qualification. Requires the ignored Playwright setup in the runbook.
import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { mkdirSync, writeFileSync } from 'node:fs';
import { chromium } from '../.local/browser-qualification/node_modules/playwright/index.mjs';
const id=randomUUID(),root='.local/browser-qualification/runs/'+id;
mkdirSync(root,{recursive:true,mode:0o700});
const origin='http://localhost:5173',issuer='http://localhost:5556',org='a982ab3f-19d0-4bc6-a6b1-7fc7a16c193c';
const context=await chromium.launchPersistentContext(root+'/profile',{channel:'chrome',headless:true,viewport:{width:1440,height:900}});
let blockedSourceMutations=0,dnsReads=0,currentStatus=200,dnsStatus=200;
const page=await context.newPage();page.setDefaultTimeout(8000);page.setDefaultNavigationTimeout(15000);
const pattern=origin+'/api/organizations/'+org+'/endpoint-choice**';
const hostname=[...Array(3).fill('a'.repeat(63)),'b'.repeat(47),'test'].join('.'),choiceId='55555555-5555-4555-8555-555555555555';
const choice={id:choiceId,organizationId:org,hostname,applicationHosts:[],state:'awaiting_ownership',proofName:'_pdsaas.'+hostname,proofValue:'pdsaas-domain-v1='+'a'.repeat(43),createdAt:new Date().toISOString(),expiresAt:new Date(Date.now()+604800000).toISOString(),canActivate:false};
const results={};
try {
 await context.route('**/*',async route=>{const url=new URL(route.request().url());if([origin,issuer].includes(url.origin))await route.continue();else await route.abort();});
 await context.route('**/api/**',async route=>{if(['POST','PUT','PATCH','DELETE'].includes(route.request().method())){blockedSourceMutations++;await route.abort();}else await route.fallback();});
 await page.goto(origin+'/api/auth/login');assert.equal(new URL(page.url()).origin,issuer);
 await page.getByRole('button',{name:'Continue as Bob'}).click();await page.waitForURL(origin+'/');
 await page.locator('#endpoint-host-'+org).waitFor({state:'attached'});
 await context.route(pattern,async route=>{
  const request=route.request(),path=new URL(request.url()).pathname;
  if(path.endsWith('/dns-observation')){assert.equal(request.method(),'POST');assert.deepEqual(request.postDataJSON(),{});dnsReads++;await route.fulfill({status:dnsStatus,json:dnsStatus===200?{choiceId,hostname,state:'challenge_match',matched:true,claimReserved:false,canActivate:false,checkedAt:new Date().toISOString(),message:'Local read-only fixture returned the exact challenge. Exclusive claims and activation remain unqualified.'}:{error:'Local response fixture'}});return;}
  if(request.method()!=='GET'){blockedSourceMutations++;await route.abort();return;}
  assert.equal(path,'/api/organizations/'+org+'/endpoint-choice');
  await route.fulfill({status:currentStatus,json:currentStatus===200?{choice}:{error:'Local denied fixture'}});
 });
 await page.reload();
 const panel=page.locator('details.endpoint-preflight').filter({has:page.locator('#endpoint-host-'+org)}),workspace=panel.locator('.endpoint-choice');
 await panel.locator('summary').click();
 await workspace.locator('.endpoint-choice-current').waitFor({state:'visible'});
 const dns=workspace.locator('.endpoint-dns-observation');
 await dns.getByRole('button',{name:'Check saved TXT challenge'}).focus();await page.keyboard.press('Enter');
 await dns.getByRole('status').waitFor({state:'visible'});
 assert.equal(await dns.getByRole('status').evaluate(el=>document.activeElement===el),true);
 assert.match(await dns.getByRole('status').textContent(),/No exclusive claim or activation was recorded/);
 results.keyboardResultFocus=true;
 dnsStatus=503;await dns.getByRole('button').click();await dns.getByRole('alert').waitFor({state:'visible'});
 assert.equal(await dns.getByRole('status').count(),0);assert.equal(await dns.getByRole('alert').evaluate(el=>document.activeElement===el),true);
 results.unavailableClearsResultAndFocusesError=true;
 dnsStatus=200;await dns.getByRole('button').click();await dns.getByRole('status').waitFor({state:'visible'});
 await workspace.getByRole('button',{name:'Review cancellation of this choice'}).click();
 const review=workspace.locator('.endpoint-choice-review');await review.waitFor({state:'visible'});
 assert.equal(await review.locator('input').isChecked(),false);assert.equal(await dns.getByRole('button').isDisabled(),true);
 assert.equal(await dns.getByRole('status').count(),0);results.reviewDisablesAndClearsObservation=true;
 const retained=await page.evaluate(org=>{const key=Object.keys(sessionStorage).find(k=>k.startsWith('pds-endpoint-intent:')&&k.endsWith(':'+org));return sessionStorage.getItem(key);},org);
 await page.reload();await review.waitFor({state:'visible'});await page.waitForFunction(org=>!document.querySelector('#endpoint-host-'+org).closest('details').querySelector('.endpoint-choice-review input').disabled,org);
 assert.equal(await review.locator('input').isChecked(),false);assert.equal(await review.getByRole('button',{name:'Confirm or retry address request'}).isDisabled(),true);
 assert.equal(await page.evaluate(org=>{const key=Object.keys(sessionStorage).find(k=>k.startsWith('pds-endpoint-intent:')&&k.endsWith(':'+org));return sessionStorage.getItem(key);},org),retained);
 results.retainedReviewReloadsUnchecked=true;
 const widths=[];
 for(const [width,height] of [[320,568],[375,812],[812,375],[768,1024],[1440,900]]) {
  await page.setViewportSize({width,height});await page.evaluate(()=>new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r))));
  const measured=await panel.evaluate(p=>{const c=p.querySelector('.endpoint-choice'),b=p.getBoundingClientRect();return {width:innerWidth,height:innerHeight,panelWidth:Math.round(b.width),documentOverflow:document.documentElement.scrollWidth>innerWidth,planOverflow:p.scrollWidth>p.clientWidth,choiceOverflow:c.scrollWidth>c.clientWidth,controlsOutside:[...c.querySelectorAll('input,button')].filter(el=>{const r=el.getBoundingClientRect();return r.left<b.left-1||r.right>b.right+1}).length};});
  if(width<=600)assert.ok(measured.panelWidth>=width-100,'Phone form remains cramped');assert.equal(measured.documentOverflow,false);assert.equal(measured.planOverflow,false);assert.equal(measured.choiceOverflow,false);assert.equal(measured.controlsOutside,0);widths.push(measured);
  if(width===375)await panel.screenshot({path:root+'/endpoint-375.png'});
 }
 results.widths=widths;
 await review.locator('input').check();await review.getByRole('button',{name:'Clear browser request after checking current state'}).click();await review.waitFor({state:'detached'});
 dnsStatus=403;await dns.getByRole('button').click();await workspace.getByRole('alert').waitFor({state:'visible'});
 assert.equal(await workspace.locator('.endpoint-choice-current').count(),0);assert.equal(await workspace.getByRole('alert').evaluate(el=>document.activeElement===el),true);results.deniedDNSClearsChoiceAndFocusesError=true;
 dnsStatus=200;await workspace.getByRole('button',{name:'Refresh saved address choice'}).click();await workspace.locator('.endpoint-choice-current').waitFor({state:'visible'});
 currentStatus=404;await workspace.getByRole('button',{name:'Refresh saved address choice'}).click();await workspace.getByRole('alert').waitFor({state:'visible'});
 assert.equal(await workspace.locator('.endpoint-choice-current').count(),0);assert.equal(await workspace.getByRole('alert').evaluate(el=>document.activeElement===el),true);results.deniedChoiceReadClearsCacheAndFocusesError=true;
 await context.unroute(pattern);await workspace.getByRole('button',{name:'Refresh saved address choice'}).click();
 await page.waitForFunction(org=>document.querySelector('#endpoint-host-'+org).closest('details').querySelector('.endpoint-choice [role=status]')?.textContent==='No pending address choice was found.',org);
 assert.equal(await workspace.locator('.endpoint-choice-current').count(),0);assert.equal(await page.evaluate(()=>Object.keys(sessionStorage).some(k=>k.startsWith('pds-endpoint-intent:'))),false);
 assert.equal(blockedSourceMutations,0);results.fixturesRemovedActualStateReloaded=true;
 const report={id,completedAt:new Date().toISOString(),playwrightVersion:'1.63.0',browserVersion:context.browser()?.version(),isolatedProfile:true,results,dnsFixtureReads:dnsReads,sourceMutationAttempts:blockedSourceMutations,positivePublicDNSUnqualified:true,report:root+'/report.json'};
 writeFileSync(report.report,JSON.stringify(report,null,2),{mode:0o600,flag:'wx'});console.log(JSON.stringify(report));
}catch(error){await page.screenshot({path:root+'/failure.png'}).catch(()=>{});throw error;}finally{await context.close();}
