#!/usr/bin/env node
import assert from 'node:assert/strict';
import { mkdirSync, writeFileSync } from 'node:fs';
import { chromium, webkit } from 'playwright';
const arg=(n,d)=>{const i=process.argv.indexOf('--'+n);return i>=0?process.argv[i+1]:d;};
const base=arg('base','http://localhost:18099'),shots=arg('shots','/tmp/penguin-quality');
const engine=arg('browser','chromium')==='webkit'?webkit:chromium;
mkdirSync(shots,{recursive:true});
const browser=await engine.launch({headless:true});
try {
 const page=await browser.newPage({viewport:{width:390,height:844},hasTouch:true});
 await page.goto(base+'/player?game=penguin-heartlight-rescue&presentation=play&probe=1');
 await page.waitForFunction(()=>window.__dqTest?.snapshot?.().t>.1);
 const snap=()=>page.evaluate(()=>window.__dqTest.snapshot());
 const first=await snap();const variants=new Set([first.entities.find(e=>e.id==='penguin').assetId]);
 const rect=await page.locator('#stage canvas').boundingBox();
 const tap=async(x)=>page.touchscreen.tap(rect.x+x/390*rect.width,rect.y+rect.height*.7);
 await tap(58);
 await page.waitForFunction(()=>window.__dqTest.snapshot().player.x<100);
 assert.ok(Math.abs((await snap()).player.y-first.player.y)<2,'tap must preserve the catch lane');
 await page.waitForFunction(()=>window.__dqTest.snapshot().entities.some(e=>e.id==='heart03'&&e.y>810),null,{timeout:12000});
 await page.screenshot({path:shots+'/missed-heart.png'});
 await page.waitForFunction(()=>window.__dqTest.snapshot().entities.some(e=>e.id==='heart03'&&e.y<100),null,{timeout:12000});
 await page.screenshot({path:shots+'/recycled-heart.png'});
 const observed=[];let poster=false;const deadline=Date.now()+60000;
 while(Date.now()<deadline) {
  const s=await snap();variants.add(s.entities.find(e=>e.id==='penguin').assetId);
  for(let i=0;i<6;i++)if(s.objectiveProgress>=(i+1)*2)assert.ok(s.entities.find(e=>e.id==='band'+i).alpha>.9,'restored aurora band must be visibly bright');
  assert.ok(s.score>=s.objectiveProgress*10,'score bonus cannot reduce heart progress');
  assert.equal((s.score-s.objectiveProgress*10)%20,0,'only 10/30 heart rewards may score');
  if(s.objectiveProgress<12)assert.equal(s.ended,false,'bonus score may not win early');
  if(observed.at(-1)!==s.objectiveProgress)observed.push(s.objectiveProgress);
  if(!poster&&s.objectiveProgress>=6&&!s.ended){await page.screenshot({path:shots+'/restoring-aurora.png'});poster=true;}
  if(s.ended)break;
  const candidates=s.entities.filter(e=>e.kind==='pickup'&&e.visible&&e.y<s.player.y+20).sort((a,b)=>b.y-a.y);
  if(candidates[0])await tap(candidates[0].x);
  await page.waitForTimeout(90);
 }
 const end=await snap();assert.equal(end.won,true);assert.equal(end.objectiveProgress,12);
 assert.ok(end.score>=120 && (end.score-120)%20===0,'12 hearts must earn 10/30 point rewards');
 assert.deepEqual([...variants].sort(),['penguin_classic','penguin_holiday','penguin_rainbow']);
 await page.screenshot({path:shots+'/win.png'});
 await page.evaluate(()=>window.__penguinReloadMarker='kept');
 await page.locator('#shell-again-btn').click();
 await page.waitForFunction(()=>window.__dqTest?.snapshot?.().ended===false&&window.__dqTest.snapshot().objectiveProgress===0);
 assert.equal(await page.evaluate(()=>window.__penguinReloadMarker),'kept');
 const report={passed:true,orientation:'portrait',missedHeartRecycled:true,shortTapMovesHorizontally:true,observedHeartCounts:observed,variants:[...variants],won:end.won,hearts:end.objectiveProgress,score:end.score,goldenHeartCatches:(end.score-120)/20,brightBands:end.entities.filter(e=>e.id.startsWith('band')).map(e=>({id:e.id,alpha:e.alpha})),restartInPlace:true};
 writeFileSync(shots+'/report.json',JSON.stringify(report,null,2));console.log(JSON.stringify(report));
}finally{await browser.close();}
