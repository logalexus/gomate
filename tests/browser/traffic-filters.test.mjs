import {test} from 'node:test';
import assert from 'node:assert/strict';
import {serviceScope,streamLocation,filterLocation,patternSelection,trafficSearch} from '../../web/src/trafficFilters.mjs';
test('opening a session keeps all-service scope and pattern filter',()=>{
 const route={params:{},query:{pattern:'42'}};
 assert.deepEqual(streamLocation(route,{id:7,service:8080}),{name:'stream',params:{servicePort:'all',streamId:7},query:{pattern:'42'}});
 assert.equal(serviceScope('all'),'');
 assert.equal(streamLocation({params:{servicePort:'8080'},query:{}},{id:7,service:8080}).params.servicePort,'8080');
});

test('traffic search persists across filters and session links, and clears its mode',()=>{
 const route={params:{servicePort:'8080',streamId:7},query:{patterns:'1,2'}};
 const target=filterLocation(route,{search:'token=[a-z]+',searchMode:'regex'});
 assert.deepEqual(trafficSearch(target.query),{query:'token=[a-z]+',mode:'regex'});
 assert.equal(streamLocation(target,{id:8}).query.q,'token=[a-z]+');
 assert.equal(filterLocation(target,{service:'9090'}).query.q,'token=[a-z]+');
 assert.equal(filterLocation(target,{patterns:[3]}).query.searchMode,'regex');
 assert.deepEqual(filterLocation(target,{search:''}).query,{patterns:'1,2'});
 assert.deepEqual(trafficSearch({q:['x'],searchMode:'bad'}),{query:'',mode:'text'});
});
test('changing a filter drops stale selected session but preserves independent constraints',()=>{
 const route={params:{servicePort:'8080',streamId:7},query:{pattern:'42',other:'kept'}};
 assert.deepEqual(filterLocation(route,{service:'9090'}),{name:'stream',params:{servicePort:'9090'},query:{pattern:'42',other:'kept'}});
 assert.deepEqual(filterLocation(route,{pattern:''}),{name:'stream',params:{servicePort:'8080'},query:{other:'kept'}});
 assert.deepEqual(filterLocation(route,{service:'',pattern:''}),{name:'stream',params:{},query:{other:'kept'}});
 assert.deepEqual(route,{params:{servicePort:'8080',streamId:7},query:{pattern:'42',other:'kept'}});
});

test('multiple pattern labels survive navigation, legacy links and independent service changes',()=>{
 assert.deepEqual(patternSelection({pattern:'42'}),[42]);
 assert.deepEqual(patternSelection({patterns:['12,42','12','bad','0','-2','2147483648']}),[12,42]);
 const route={params:{servicePort:'8080',streamId:7},query:{pattern:'42',other:'kept'}};
 const target=filterLocation(route,{patterns:[42,12]});
 assert.deepEqual(target,{name:'stream',params:{servicePort:'8080'},query:{patterns:'42,12',other:'kept'}});
 assert.equal(streamLocation(target,{id:8}).query.patterns,'42,12');
 assert.equal(filterLocation(target,{service:'9090'}).query.patterns,'42,12');
 assert.deepEqual(filterLocation(target,{patterns:[]}).query,{other:'kept'});
 assert.deepEqual(filterLocation(target,{pattern:'2'}).query,{pattern:'2',other:'kept'});
});
