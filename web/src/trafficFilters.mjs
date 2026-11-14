// Route scope and selection are independent. Opening a session from “all” must
// not silently enable a service filter.
export function serviceScope(value) {return value && value !== 'all' ? String(value) : '';}
export function streamLocation(route, stream) {
 return {name:'stream',params:{servicePort:serviceScope(route.params?.servicePort)||'all',streamId:stream.id},query:{...route.query}};
}
export function filterLocation(route, changes={}) {
 const query={...route.query};
 if(Object.hasOwn(changes,'pattern')){if(changes.pattern)query.pattern=String(changes.pattern);else delete query.pattern;}
 const service=Object.hasOwn(changes,'service')?changes.service:serviceScope(route.params?.servicePort);
 return {name:'stream',params:service?{servicePort:String(service)}:{},query};
}
