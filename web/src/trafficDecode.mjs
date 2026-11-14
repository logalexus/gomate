export const MAX_INPUT = 65536;
export const MAX_RESULTS = 16;
const MAX_OUTPUT = 262144;
export const OPERATIONS = [
 {value:'base64',text:'Base64 / Base64URL'}, {value:'jwt',text:'JWT (header + payload)'},
 {value:'url',text:'URL (%XX)'}, {value:'form',text:'Form URL (+ → пробел)'},
 {value:'hex',text:'HEX / \\xNN'}, {value:'json',text:'JSON / JSON-строка'},
];
const utf8 = new TextDecoder('utf-8',{fatal:true});
const encoder = new TextEncoder();
function checkInput(input) {
 if(typeof input!=='string'||!input.trim())throw Error('Введите или выделите данные для декодирования.');
 if(input.length>MAX_INPUT)throw Error('Лимит — 65 536 символов. Выделите меньший фрагмент.');
}
function b64(input) {
 const value=input.replace(/\s/g,'');
 if(!value||!/^[A-Za-z0-9+/_-]+={0,2}$/.test(value))throw Error('Некорректный Base64: проверьте символы и выделите только значение.');
 const bare=value.replace(/=+$/,'');
 if(bare.length%4===1||(value.includes('=')&&value.length%4!==0))throw Error('Некорректная длина или padding Base64.');
 const normalized=bare.replace(/-/g,'+').replace(/_/g,'/');
 const binary=atob(normalized+'='.repeat((4-normalized.length%4)%4));
 // Reject silently discarded trailing bits, which otherwise produce many
 // false positives when ordinary words are guessed as Base64.
 if(btoa(binary).replace(/=+$/,'')!==normalized)throw Error('Неканонические завершающие биты Base64.');
 return Uint8Array.from(binary,c=>c.charCodeAt(0));
}
function textBytes(bytes) {
 try {const text=utf8.decode(bytes);return /[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]/.test(text)?null:text;}catch{return null;}
}
function result(operation, bytes) {
 if(bytes.length>MAX_OUTPUT)throw Error('Результат превышает лимит 256 KiB.');
 const text=textBytes(bytes);
 return {operation,label:OPERATIONS.find(o=>o.value===operation)?.text||operation,text:text??'',binary:text===null,byteLength:bytes.length,
  hex:Array.from(bytes.subarray(0,4096),b=>b.toString(16).padStart(2,'0')).join(' '),hexTruncated:bytes.length>4096};
}
function jsonObject(encoded, name) {
 let value;
 try {value=JSON.parse(utf8.decode(b64(encoded)));}catch{throw Error(`JWT: ${name} должен содержать JSON в UTF-8 / Base64URL.`);}
 if(!value||typeof value!=='object'||Array.isArray(value))throw Error(`JWT: ${name} должен быть JSON-объектом.`);
 return value;
}
function pretty(value) {
 // Bound indentation expansion before stringify allocates its output.
 const stack=[[value,0]];
 while(stack.length){const [node,depth]=stack.pop();if(node&&typeof node==='object'){
  if(depth>32)throw Error('JSON вложен глубже 32 уровней. Выберите меньший фрагмент.');
  for(const child of Object.values(node))stack.push([child,depth+1]);
 }}
 const text=JSON.stringify(value,null,2);
 if(text.length>MAX_OUTPUT)throw Error('Форматированный JSON превышает лимит 256 KiB.');
 return text;
}
export function decode(input, operation) {
 checkInput(input);
 let bytes, jwt;
 switch(operation) {
 case 'base64': bytes=b64(input);break;
 case 'hex': {
  let value=input.trim();
  if(value.startsWith('0x'))value=value.slice(2);
  if(value.includes('\\x')){if(!/^(?:\s*\\x[\da-f]{2})+\s*$/i.test(value))throw Error('Ожидаются пары \\xNN.');value=value.replace(/\\x/gi,'');}
  value=value.replace(/\s/g,'');
  if(!/^(?:[\da-f]{2})+$/i.test(value))throw Error('HEX: нужны пары 00–FF, без адресов и ASCII-колонки дампа.');
  bytes=Uint8Array.from(value.match(/../g),v=>parseInt(v,16));break;
 }
 case 'url': case 'form':
  try {bytes=encoder.encode(decodeURIComponent(operation==='form'?input.replace(/\+/g,' '):input));}
  catch {throw Error('Некорректное %XX-кодирование или UTF-8. Проверьте выделение.');}break;
 case 'json': {
  let value;try{value=JSON.parse(input);}catch{throw Error('Ожидается JSON. Для \\uXXXX / \\n заключите строку в двойные кавычки.');}
  bytes=encoder.encode(typeof value==='string'?value:pretty(value));break;
 }
 case 'jwt': {
  const token=input.trim().replace(/^Bearer\s+/i,''),parts=token.split('.');
  if(parts.length===5)throw Error('Это JWE: payload зашифрован. Без ключа его прочитать нельзя.');
  if(parts.length!==3||!parts[0]||!parts[1]||parts.some(p=>!/^[A-Za-z0-9_-]*$/.test(p)))throw Error('JWT должен иметь три части Base64URL, разделённые точками.');
  const header=jsonObject(parts[0],'header'),payload=jsonObject(parts[1],'payload');
  if(parts[2])b64(parts[2]);
  const dates=[];
  for(const field of ['iat','nbf','exp'])if(typeof payload[field]==='number'&&Number.isFinite(payload[field])){
   const date=new Date(payload[field]*1000);if(Number.isFinite(date.getTime()))dates.push({field,value:date.toISOString()});
  }
  jwt={header:pretty(header),payload:pretty(payload),dates};
  bytes=encoder.encode(pretty({header,payload,signature:parts[2]}));break;
 }
 default: throw Error('Неизвестный декодер.');
 }
 return {...result(operation,bytes),...(jwt?{jwt,warning:'Подпись JWT не проверялась. Claims показаны как переданные данные.'}:{})};
}

// Only run this on an explicit request, inside a worker. Scan one bounded
// fragment; never enqueue work for every packet received in LIVE mode.
export function detect(input) {
 checkInput(input);
 const results=[],seen=new Set();let attempts=0,limited=false,total=0;
 function tryValue(value,offset,operations) {
  for(const operation of operations){
   if(attempts>=512||results.length>=MAX_RESULTS||total>=MAX_OUTPUT){limited=true;return;}
   const key=operation+'\0'+value;if(seen.has(key))continue;seen.add(key);attempts++;
   try {
    const decoded=decode(value,operation);
    if(decoded.binary||!decoded.text.trim()||decoded.text===value)continue;
    if(total+decoded.text.length>MAX_OUTPUT){limited=true;continue;}
    total+=decoded.text.length;results.push({...decoded,source:value,offset});
   }catch{/* A failed guess is expected; explicit decoding reports the error. */}
  }
 }
 const trimmed=input.trim(),offset=input.indexOf(trimmed);
 tryValue(trimmed,offset,['jwt','base64','hex','json']);
 // Tokenize first: an unanchored greedy "word+.word+" regexp can take
 // quadratic time on a long Base64 blob without any dots.
 for(const match of input.matchAll(/[A-Za-z0-9_.-]+/g)){
  const parts=match[0].split('.');if(parts.length===3||parts.length===5)tryValue(match[0],match.index,['jwt']);
  if(limited)break;
 }
 // Separators retain percent-encoded values, JSON strings, cookies and Basic
 // auth values. Check each complete token, never slices of an invalid token.
 const tokens=/[^\s"'<>;&?{}()[\],:=]+={0,2}/g;
 for(const match of input.matchAll(tokens)){
  if(limited)break;
  const value=match[0],ops=[];
  if(/%[\da-f]{2}/i.test(value)){ops.push('url');if(value.includes('+'))ops.push('form');}
  if(value.length>=8&&/^[A-Za-z0-9+/_-]+={0,2}$/.test(value))ops.push('base64');
  if(value.length>=8&&/^(?:[\da-f]{2})+$/i.test(value))ops.push('hex');
  tryValue(value,match.index,ops);
 }
 return {results,limited,attempts};
}
