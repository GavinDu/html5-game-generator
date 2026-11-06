#!/usr/bin/env node
import assert from 'node:assert/strict';
import { mkdirSync, writeFileSync } from 'node:fs';
import { chromium, webkit } from 'playwright';
const arg=(n,d)=>{const i=process.argv.indexOf('--'+n);return i>=0?process.argv[i+1]:d;};
const base=arg('base','http://localhost:18099'), shots=arg('shots','/tmp/orientation-policy');
const engine=arg('browser','chromium')==='webkit'?webkit:chromium;
mkdirSync(shots,{recursive:true});
const browser=await engine.launch({headless:true});
const results=[];
try {
 const original=await (await fetch(base+'/v1/games/penguin-heartlight-rescue/package')).json();
 for(const orientation of ['portrait','landscape','any']) {
  const wide=orientation!=='portrait';
  const viewport=wide?{width:844,height:390}:{width:390,height:844};
  const page=await browser.newPage({viewport,hasTouch:true});
  if(orientation!=='portrait') {
   const pkg=structuredClone(original);pkg.orientation=orientation;pkg.ir.game.orientation=orientation;
   pkg.ir.game.entryScene='orientation-fixture';
   const penguin=structuredClone(pkg.ir.scenes[0].entities.find(e=>e.id==='penguin'));
   penguin.position=[422,300];penguin.size=[70,84];
   pkg.ir.scenes=[{id:'orientation-fixture',worldSize:[844,390],backgroundColor:'#142E50',entities:[penguin]}];
   pkg.ir.rules=[];
   await page.route('**/v1/games/penguin-heartlight-rescue/package',r=>r.fulfill({json:pkg}));
  }
  await page.goto(base+'/player?game=penguin-heartlight-rescue&presentation=play&probe=1');
  await page.waitForFunction(()=>window.__dqTest?.snapshot?.().t>.2);
  const before=await page.evaluate(()=>window.__dqTest.snapshot());
  const rect=await page.locator('#stage canvas').boundingBox();
  assert.ok(Math.abs(rect.width/rect.height-(wide?844/390:390/844))<.01,'renderer must keep the authored orientation aspect');
  await page.screenshot({path:shots+'/'+orientation+'-supported.png'});
  await page.setViewportSize({width:viewport.height,height:viewport.width});
  await page.waitForTimeout(150);
  const paused=await page.evaluate(()=>window.__dqTest.snapshot());
  await page.waitForTimeout(450);
  const later=await page.evaluate(()=>window.__dqTest.snapshot());
  const blocked=await page.evaluate(()=>document.body.dataset.orientationBlocked==='true');
  if(orientation==='any') {
   assert.equal(blocked,false);assert.ok(later.t>paused.t+.1);
  } else {
   assert.equal(blocked,true);assert.ok(Math.abs(later.t-paused.t)<.04,'unsupported orientation must pause rather than consume the session');
   assert.equal(await page.locator('#orientation-hint').isVisible(),true);
  }
  await page.screenshot({path:shots+'/'+orientation+'-rotated.png'});
  await page.setViewportSize(viewport);
  await page.waitForFunction(t=>window.__dqTest.snapshot().t>t+.1,later.t);
  assert.equal(await page.evaluate(()=>document.body.dataset.orientationBlocked),'false');
  const resumed=await page.evaluate(()=>window.__dqTest.snapshot());
  assert.ok(resumed.t>=before.t,'rotation must preserve the session');
  results.push({orientation,passed:true,blockedOnRotation:blocked,aspect:rect.width/rect.height});
  await page.close();
 }
} finally {await browser.close();}
writeFileSync(shots+'/report.json',JSON.stringify(results,null,2));
console.log(JSON.stringify(results));
