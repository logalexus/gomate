import {test} from 'node:test';
import assert from 'node:assert/strict';
import {serviceScope,streamLocation,filterLocation} from '../../web/src/trafficFilters.mjs';
test('opening a session keeps all-service scope and pattern filter',()=>{
 const route={params:{},query:{pattern:'42'}};
 assert.deepEqual(streamLocation(route,{id:7,service:8080}),{name:'stream',params:{servicePort:'all',streamId:7},query:{pattern:'42'}});
 assert.equal(serviceScope('all'),'');
 assert.equal(streamLocation({params:{servicePort:'8080'},query:{}},{id:7,service:8080}).params.servicePort,'8080');
});
test('changing a filter drops stale selected session but preserves independent constraints',()=>{
 const route={params:{servicePort:'8080',streamId:7},query:{pattern:'42',other:'kept'}};
 assert.deepEqual(filterLocation(route,{service:'9090'}),{name:'stream',params:{servicePort:'9090'},query:{pattern:'42',other:'kept'}});
 assert.deepEqual(filterLocation(route,{pattern:''}),{name:'stream',params:{servicePort:'8080'},query:{other:'kept'}});
 assert.deepEqual(filterLocation(route,{service:'',pattern:''}),{name:'stream',params:{},query:{other:'kept'}});
 assert.deepEqual(route,{params:{servicePort:'8080',streamId:7},query:{pattern:'42',other:'kept'}});
});
