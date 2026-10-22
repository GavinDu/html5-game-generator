import test from 'node:test';
import assert from 'node:assert/strict';
import { METRICS, scoreGame } from './index.mjs';
const checks = [
{checkId:'playtest.boot_clean',status:'pass',evidence:'no errors'},
{checkId:'playtest.render_nonblank',status:'pass',evidence:'visible'},
{checkId:'simulation.no_softlock',status:'pass',evidence:'progressed'},
{checkId:'hook.time_to_input',status:'pass',evidence:'input'},
{checkId:'hook.first_reward',status:'pass',evidence:'reward'},
{checkId:'budgets.cold_start',status:'pass',evidence:'startup',actual:{coldStartMs:1200}}
];
const review={metrics:METRICS.filter(m=>m.source==='ai').map(m=>({id:m.id,score:85,rationale:'Clear and coherent.',evidence:'Observed in game.'}))};
test('weights sum to 100; IDs unique',()=>{assert.equal(METRICS.reduce((s,m)=>s+m.weight,0),100);assert.equal(new Set(METRICS.map(m=>m.id)).size,METRICS.length)});
test('partial score does not disguise missing evidence',()=>{const r=scoreGame(checks);assert.equal(r.overallScore,null);assert.equal(r.deliveryReady,true);assert.equal(r.coveragePercent,48)});
test('complete review yields complete score',()=>{const r=scoreGame(checks,review);assert.equal(r.overallScore,92.2);assert.equal(r.grade,'excellent')});
test('hard gate failure blocks delivery',()=>{const r=scoreGame(checks.map(c=>c.checkId==='playtest.render_nonblank'?{...c,status:'fail'}:c),review);assert.equal(r.deliveryReady,false);assert.equal(r.deliveryGate,'fail')});
test('AI cannot override objective scores or submit invalid scores',()=>{const r=scoreGame(checks,{metrics:[{id:'reliability',score:100,rationale:'great',evidence:'claim'},{id:'visual_design',score:1000,rationale:'great',evidence:'invalid'}]});assert.equal(r.dimensions.find(d=>d.id==='reliability').score,100);assert.equal(r.dimensions.find(d=>d.id==='visual_design').score,null)});
