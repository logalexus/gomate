import test from 'node:test';
import assert from 'node:assert/strict';
import {mergeChatMessages as merge} from '../../web/src/chatMessages.mjs';
const m=(id,revision,text='message',deleted=false)=>({id,revision,text,deleted});
test('slow snapshots cannot undo edits or restore deleted messages',()=>{
 const versions=new Map();let rows=merge([], [m(1,1),m(2,2)],versions);
 rows=merge(rows,[m(1,3,'edited')],versions,{insert:false});
 rows=merge(rows,[m(1,1),m(2,2)],versions,{replace:true});assert.equal(rows[0].text,'edited');
 rows=merge(rows,[m(1,4,'',true)],versions,{insert:false});assert.deepEqual(rows.map(m=>m.id),[2]);
 rows=merge(rows,[m(1,1),m(2,2)],versions,{replace:true});assert.deepEqual(rows.map(m=>m.id),[2]);
});
test('changes patch loaded history without appending new messages',()=>{
 const versions=new Map();let rows=merge([],[m(1,1),m(2,2)],versions);
 rows=merge(rows,[m(1,4,'updated'),m(3,3)],versions,{insert:false});assert.deepEqual(rows.map(m=>m.id),[1,2]);assert.equal(rows[0].text,'updated');
 rows=merge(rows,[m(3,3)],versions);assert.deepEqual(rows.map(m=>m.id),[1,2,3]);
});
test('history remains ordered, deduplicated and bounded',()=>{
 const versions=new Map(),page=Array.from({length:600},(_,i)=>m(i+1,i+1));
 const latest=merge([],page,versions);assert.equal(latest.length,500);assert.equal(latest[0].id,101);
 const older=merge(latest,page.slice(0,150),versions,{prepend:true});assert.equal(older.length,500);assert.equal(older[0].id,1);assert.equal(older.at(-1).id,500);
});
test('legacy zero revisions can be edited and recent tombstones stay bounded',()=>{
 const versions=new Map();let rows=merge([],[m(1,0)],versions);
 rows=merge(rows,[m(1,1,'edited')],versions,{insert:false});assert.equal(rows[0].text,'edited');
 rows=merge(rows,Array.from({length:3000},(_,i)=>m(i+2,i+2,'',true)),versions,{insert:false});assert.equal(rows.length,1);assert(versions.size<=1500);
});
