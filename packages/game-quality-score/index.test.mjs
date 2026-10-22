import test from 'node:test';
import assert from 'node:assert/strict';
import { METRICS, scoreGame } from './index.mjs';
import { readFile } from 'node:fs/promises';
const checks = [
 {checkId:'playtest.boot_clean',status:'pass',evidence:'no errors'},
 {checkId:'playtest.render_nonblank',status:'pass',evidence:'visible'},
 {checkId:'simulation.no_softlock',status:'pass',evidence:'progressed'},
 {checkId:'hook.time_to_input',status:'pass',evidence:'input'},
 {checkId:'hook.first_reward',status:'pass',evidence:'reward'},
 {checkId:'budgets.cold_start',status:'pass',evidence:'startup',actual:{coldStartMs:1200}}
];
const review={metrics:METRICS.filter(m=>m.source==='ai').map(m=>({id:m.id,score:85,rationale:'Clear and coherent.',evidence:'Observed in game.'}))};
test('weights sum to 100 and metric IDs are unique',()=>{assert.equal(METRICS.reduce((s,m)=>s+m.weight,0),100);assert.equal(new Set(METRICS.map(m=>m.id)).size,METRICS.length)});
test('partial score does not disguise missing evidence',()=>{const r=scoreGame(checks);assert.equal(r.overallScore,null);assert.equal(r.deliveryReady,true);assert.equal(r.coveragePercent,48);assert.equal(r.target.targetMet,false)});
test('complete review computes target status',()=>{const r=scoreGame(checks,review);assert.equal(r.overallScore,92.2);assert.equal(r.grade,'excellent');assert.equal(r.target.targetMet,true)});
test('any metric below 70 fails target and requests a suggestion',()=>{const low={...review,metrics:review.metrics.map(m=>m.id==='visual_design'?{...m,score:65}:m),improvementSuggestions:[{metricId:'visual_design',priority:'high',problem:'Weak contrast',suggestion:'Increase foreground/background contrast.'}]};const r=scoreGame(checks,low);assert.equal(r.target.targetMet,false);assert.deepEqual(r.belowFloor,[{metricId:'visual_design',score:65,minimum:70}]);assert.equal(r.missingSuggestions.length,0)});
test('missing suggestion is explicitly surfaced',()=>{const low={...review,metrics:review.metrics.map(m=>m.id==='animation'?{...m,score:60}:m)};const r=scoreGame(checks,low);assert.equal(r.missingSuggestions[0].metricId,'animation')});
test('failed hard gate blocks target even with high scores',()=>{const r=scoreGame(checks.map(c=>c.checkId==='playtest.render_nonblank'?{...c,status:'fail'}:c),review);assert.equal(r.deliveryReady,false);assert.equal(r.target.targetMet,false)});
test('all committed game rule files enforce thresholds and weight totals',async()=>{for(const url of [new URL('../../config/game-quality/default.json',import.meta.url),new URL('../../inventory/13ui1k/quality-rules.json',import.meta.url),new URL('../../inventory/2uowg9/quality-rules.json',import.meta.url),new URL('../../inventory/u46ex5/quality-rules.json',import.meta.url)]){const rules=JSON.parse(await readFile(url,'utf8'));assert.equal(rules.thresholds.minimumOverallScore,80);assert.equal(rules.thresholds.minimumMetricScore,70);assert.equal(Object.values(rules.weights).reduce((a,b)=>a+b,0),100)}});
