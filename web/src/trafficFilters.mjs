// Route scope and selection are independent. Opening a session from “all” must
// not silently enable a service filter.
export function serviceScope(value) {return value && value !== 'all' ? String(value) : '';}
export function patternSelection(query={}) {
 const raw=query.patterns??query.pattern??'';
 return [...new Set((Array.isArray(raw)?raw:[raw]).flatMap(value=>String(value).split(',')).filter(value=>/^[1-9]\d*$/.test(value)).map(Number).filter(id=>Number.isSafeInteger(id)&&id<=2147483647))].slice(0,100);
}
export function streamLocation(route, stream) {
 return {name:'stream',params:{servicePort:serviceScope(route.params?.servicePort)||'all',streamId:stream.id},query:{...route.query}};
}
export function filterLocation(route, changes={}) {
 const query={...route.query};
 if(Object.hasOwn(changes,'patterns')){delete query.pattern;const ids=patternSelection({patterns:changes.patterns});if(ids.length)query.patterns=ids.join(',');else delete query.patterns;}
 if(Object.hasOwn(changes,'pattern')){delete query.patterns;}
 if(Object.hasOwn(changes,'pattern')){if(changes.pattern)query.pattern=String(changes.pattern);else delete query.pattern;}
 const service=Object.hasOwn(changes,'service')?changes.service:serviceScope(route.params?.servicePort);
 return {name:'stream',params:service?{servicePort:String(service)}:{},query};
}
