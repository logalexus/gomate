import test from 'node:test';
import assert from 'node:assert/strict';
import {resourceLoad} from '../web/src/monitor.mjs';

test('load levels use actual capacity and include the threshold boundary',()=>{
 assert.equal(resourceLoad(0,100).level,'normal');
 assert.equal(resourceLoad(69.9,100).level,'normal');
 assert.equal(resourceLoad(70,100).level,'warning');
 assert.equal(resourceLoad(89.9,100).level,'warning');
 assert.equal(resourceLoad(90,100).level,'danger');
 assert.equal(resourceLoad(700,1000).level,'warning');
 assert.equal(resourceLoad(900,1000).level,'danger');
});

test('Docker percentages normalize to all host cores without losing the actual usage',()=>{
 const oneCore=resourceLoad(100,8*100);
 assert.equal(oneCore.percent,12.5);
 assert.equal(oneCore.level,'normal');
 assert.equal(resourceLoad(560,8*100).level,'warning');
 assert.equal(resourceLoad(720,8*100).level,'danger');
 const overLimit=resourceLoad(850,8*100);
 assert.equal(overLimit.level,'danger');
 assert.equal(overLimit.percent,106.25);
 assert.equal(overLimit.width,100);
});

test('missing samples and absent limits remain unknown rather than green',()=>{
 for(const [value,capacity] of [[null,100],[undefined,100],[NaN,100],[Infinity,100],[-1,100],[1,null],[1,0],[1,-100],[1,Infinity],[1,undefined]]){
  const load=resourceLoad(value,capacity);
  assert.equal(load.level,'unknown',`${value}/${capacity}`);
  assert.equal(load.percent,null);
 }
});
