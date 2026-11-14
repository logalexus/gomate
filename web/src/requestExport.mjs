// Export is local text generation only. No requests are sent by this module.
export const MAX_EXPORT_BYTES = 256 * 1024;
const MAX_HEADERS = 64 * 1024;
const TOKEN = /^[!#$%&'*+.^_`|~0-9A-Za-z-]+$/;
const CONTROL = /[\x00-\x08\x0a-\x1f\x7f]/;
const HTTP_KINDS = new Set(['', 'raw', 'http']);
const fail = message => { throw Error(message); };
const values = (headers, name) => headers.filter(h => h.name.toLowerCase() === name).map(h => h.value);
const warn = (warnings, message) => { if (!warnings.includes(message)) warnings.push(message); };
function packetBytes(packet) {
 const encoded = packet?.content;
 if (typeof encoded !== 'string' || !encoded.length) fail('В блоке нет сохранённых байтов.');
 if (encoded.length > Math.ceil(MAX_EXPORT_BYTES / 3) * 4 + 4096) fail('Блок превышает лимит экспорта 256 KiB.');
 const compact = encoded.replace(/\s/g, '');
 if (!/^[A-Za-z0-9+/]*={0,2}$/.test(compact) || compact.length % 4 !== 0) fail('Некорректное Base64 содержимого блока.');
 let raw; try { raw = atob(compact); } catch { fail('Некорректное Base64 содержимого блока.'); }
 if (btoa(raw) !== compact) fail('Неканоническое Base64 содержимого блока.');
 if (raw.length > MAX_EXPORT_BYTES) fail('Блок превышает лимит экспорта 256 KiB.');
 return raw;
}
function headerLines(lines) {
 if (lines.length > 512) fail('Слишком много HTTP-заголовков (лимит 512).');
 return lines.map(line => {
  const colon = line.indexOf(':');
  if (colon < 1 || !TOKEN.test(line.slice(0, colon))) fail('Некорректный HTTP-заголовок или перенос заголовка. Выберите raw/pwntools.');
  const value = line.slice(colon + 1).replace(/^[ \t]+|[ \t]+$/g, '');
  if (CONTROL.test(value)) fail('В HTTP-заголовке есть управляющие символы. Выберите raw/pwntools.');
  return {name:line.slice(0,colon),value};
 });
}
function requestHead(raw, start = 0) {
 const end = raw.indexOf('\r\n\r\n', start);
 if (end < 0) fail('HTTP-заголовки обрезаны: нет завершающего CRLF CRLF.');
 if (end - start > MAX_HEADERS) fail('HTTP-заголовки превышают лимит 64 KiB.');
 const lines = raw.slice(start,end).split('\r\n');
 const first = lines.shift();
 if (/^HTTP\//.test(first)) fail('Это HTTP-ответ. Выберите входящий запрос.');
 const match = /^([^ ]+) ([^ ]+) HTTP\/(1\.[01])$/.exec(first);
 if (!match || !TOKEN.test(match[1]) || !/^[\x21-\x7e]+$/.test(match[2])) fail('Нет полного корректного HTTP/1.x запроса. Выберите raw-блок с началом запроса.');
 if (match[1] === 'PRI' && match[2] === '*') fail('HTTP/2 preface нельзя экспортировать как HTTP/1 запрос.');
 return {method:match[1],target:match[2],version:match[3],headers:headerLines(lines),bodyStart:end+4};
}
function chunkedBody(raw, start, warnings) {
 let cursor=start, size=0;const chunks=[];
 for(let count=0;count<MAX_EXPORT_BYTES;count++){
  const end=raw.indexOf('\r\n',cursor);
  if(end<0 || end-cursor>8192)fail('Обрезанный или слишком длинный заголовок HTTP chunk.');
  const line=raw.slice(cursor,end),match=/^([\da-f]+)(?:;[\t\x20-\x7e]*)?$/i.exec(line);
  if(!match || match[1].length>12)fail('Некорректный размер HTTP chunk.');
  const length=parseInt(match[1],16);cursor=end+2;
  if(length===0){
   const trailers=[];const trailerStart=cursor;
   for(;;){
    const trailerEnd=raw.indexOf('\r\n',cursor);
    if(trailerEnd<0)fail('Обрезано завершение chunked-тела.');
    if(trailerEnd-trailerStart>MAX_HEADERS || trailers.length>512)fail('Превышен лимит HTTP trailers.');
    const trailer=raw.slice(cursor,trailerEnd);cursor=trailerEnd+2;
    if(!trailer)break;trailers.push(trailer);
   }
   headerLines(trailers);
   if(trailers.length)warn(warnings,'HTTP trailers не передаются через requests/curl; для точных байтов выберите pwntools.');
   return {body:chunks.join(''),end:cursor};
  }
  size+=length;
  if(size>MAX_EXPORT_BYTES)fail('HTTP-тело превышает лимит 256 KiB.');
  if(cursor+length+2>raw.length)fail('HTTP chunk обрезан. Выберите полный raw-запрос.');
  if(raw.slice(cursor+length,cursor+length+2)!=='\r\n')fail('После HTTP chunk отсутствует CRLF.');
  chunks.push(raw.slice(cursor,cursor+length));cursor+=length+2;
 }
 fail('Превышен лимит HTTP chunks.');
}
function parseRawHTTP(raw,warnings){
 const requests=[];let cursor=0;
 while(cursor<raw.length){
  if(requests.length>=128)fail('В блоке больше 128 HTTP-запросов.');
  const head=requestHead(raw,cursor),cl=values(head.headers,'content-length'),te=values(head.headers,'transfer-encoding');
  if(cl.length>1 || te.length>1 || cl.length&&te.length)fail('Неоднозначный HTTP framing: несколько Content-Length/Transfer-Encoding. Используйте pwntools для raw-байтов.');
  let body='',end=head.bodyStart;
  if(te.length){
   if(te[0].toLowerCase()!=='chunked')fail('Поддерживается только Transfer-Encoding: chunked. Выберите raw/pwntools.');
   ({body,end}=chunkedBody(raw,end,warnings));
   warn(warnings,'Для requests/curl chunked-тело собрано; длина и framing будут рассчитаны библиотекой.');
  }else if(cl.length){
   if(!/^\d+$/.test(cl[0]))fail('Некорректный Content-Length.');
   const length=Number(cl[0]);
   if(!Number.isSafeInteger(length)||length>MAX_EXPORT_BYTES)fail('Content-Length превышает лимит 256 KiB.');
   if(end+length>raw.length)fail('HTTP-тело обрезано относительно Content-Length.');
   body=raw.slice(end,end+length);end+=length;
  }
  requests.push({...head,body,raw:raw.slice(cursor,end)});cursor=end;
 }
 return requests;
}
function percentBytes(text){return text.replace(/%([\da-f]{2})/gi,(_,v)=>String.fromCharCode(parseInt(v,16)));}
function normalizedHeaders(headers,ungzipped=false){
 return headers.filter(h=>!['content-length','transfer-encoding','trailer'].includes(h.name.toLowerCase())&&!(ungzipped&&h.name.toLowerCase()==='content-encoding'));
}
function parseDecodedHTTP(raw,packet,service,warnings){
 let view=raw;
 const method=packet.metadata?.method,target=packet.metadata?.path;
 if(method&&target){
  if(!TOKEN.test(method)||!/^[\x21-\x7e]+$/.test(target))fail('Metadata HTTP-запроса содержит недопустимый method/path. Выберите исходный raw-блок.');
  // The displayed target may contain decoded spaces or CRLF. Match its full
  // known metadata-derived prefix, rather than treating those bytes as headers.
  const targets=[target,percentBytes(target)];
  let matched=false;
  for(const displayed of targets)for(const version of ['1.1','1.0']){
   const prefix=`${method} ${displayed} HTTP/${version}\r\n`;
   if(raw.startsWith(prefix)){view=`${method} ${target} HTTP/${version}\r\n`+raw.slice(prefix.length);matched=true;break;}
  }
  if(!matched)fail('Показанный HTTP-запрос не согласуется с metadata. Выберите исходный raw-блок.');
 }else if(service.urldecodeHttpRequests){fail('Для восстановленного URL нужны исходные method/path metadata. Выберите raw-блок.');}
 const head=requestHead(view);
 if(service.urldecodeHttpRequests&&values(head.headers,'content-type').some(v=>/^application\/x-www-form-urlencoded(?:\s*;|$)/i.test(v)))fail('Form-тело уже URL-декодировано: исходную форму восстановить нельзя. Выберите raw-блок.');
 const body=view.slice(head.bodyStart),headers=normalizedHeaders(head.headers,!!packet.ungzipped);
 warn(warnings,'Экспорт восстановлен из разобранного HTTP: показанные байты нормализованы и могут отличаться от wire. Для точного воспроизведения выберите raw-блок.');
 if(packet.ungzipped)warn(warnings,'Тело уже распаковано; Content-Encoding и прежняя длина удалены.');
 const wire=`${head.method} ${head.target} HTTP/${head.version}\r\n`+headers.map(h=>`${h.name}: ${h.value}\r\n`).join('')+`Content-Length: ${body.length}\r\n\r\n`+body;
 return [{...head,headers,body,raw:wire}];
}
function authority(value){
 if(typeof value!=='string'||!value)return null;
 if(value.startsWith('[')){const match=/^\[([^\]]+)\](?::(\d+))?$/.exec(value);return match?{host:match[1],port:match[2]?Number(match[2]):null}:null;}
 const match=/^([^:\s]+)(?::(\d+))?$/.exec(value);
 if(match)return {host:match[1],port:match[2]?Number(match[2]):null};
 if(/^[\da-f:]+(?:%[A-Za-z0-9_.-]+)?$/i.test(value)&&value.includes(':'))return {host:value,port:null};
 return null;
}
function endpointFor(stream,service,requests,raw){
 const header=requests[0]?.headers.find(h=>h.name.toLowerCase()==='host')?.value;
 const fallback=header||raw.slice(0,MAX_HEADERS).match(/(?:^|\r\n)Host:[ \t]*([^\r\n]+)/i)?.[1]?.trim();
 const destination=authority(stream.server)||authority(fallback)||{host:'',port:null};
 const port=destination.port||Number(stream.service)||Number(service.port)||80;
 return {host:destination.host,port,scheme:port===443?'https':'http'};
}
export function prepareExport(packet,stream={},service={}){
 stream=stream&&typeof stream==='object'?stream:{};service=service&&typeof service==='object'?service:{};
 let raw='',requests=[],httpError='';const warnings=[];
 try{
  raw=packetBytes(packet);
  if(packet.incoming!==true)fail('Экспорт запроса доступен только для входящих блоков.');
  if(packet.incomplete||stream.incomplete)fail('В захвате есть пропуски. HTTP-экспорт отключён; raw-байты доступны через pwntools.');
  if(String(stream.protocol||'tcp').toLowerCase()==='udp')fail('Это UDP-блок: HTTP-клиенты requests/curl не воспроизводят UDP. Выберите pwntools.');
  const kind=packet.kind||'';
  if(!HTTP_KINDS.has(kind))fail('Это '+kind+', а не полный HTTP/1 запрос. Выберите исходный raw-блок.');
  requests=kind==='http'?parseDecodedHTTP(raw,packet,service,warnings):parseRawHTTP(raw,warnings);
 }catch(error){httpError=error.message||'Не удалось разобрать запрос.';requests=[];}
 return {requests,raw,httpError,warnings,endpoint:endpointFor(stream,service,requests,raw)};
}
function python(value){
 return '"'+Array.from(String(value),ch=>{
  const point=ch.codePointAt(0);
  if(ch==='"')return '\\"';if(ch==='\\')return '\\\\';
  if(ch==='\n')return '\\n';if(ch==='\r')return '\\r';if(ch==='\t')return '\\t';
  if(point>=32&&point<=126||point>=160&&/[\p{L}\p{M}\p{N}\p{P}\p{S}]/u.test(ch))return ch;
  return point<=0xffff?'\\u'+point.toString(16).padStart(4,'0'):'\\U'+point.toString(16).padStart(8,'0');
 }).join('')+'"';
}
function readableText(raw){
 if(raw.length>8192)return null;
 try{const text=new TextDecoder('utf-8',{fatal:true,ignoreBOM:true}).decode(Uint8Array.from(raw,c=>c.charCodeAt(0)));return /[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]/.test(text)?null:text}catch{return null}
}
function pythonPayload(raw){
 const text=readableText(raw);
 if(text!==null){
  if(/^[\x00-\x7f]*$/.test(raw)){
   const parts=raw.match(/[^\n]*\n|[^\n]+/g)||[''];
   return {expression:parts.length>1&&parts.length<=128?'(\n'+parts.map(part=>'    b'+python(part)).join('\n')+'\n)':'b'+python(raw),base64:false};
  }
  return {expression:python(text)+'.encode("utf-8")',base64:false};
 }
 return {expression:'base64.b64decode('+python(btoa(raw))+', validate=True)',base64:true};
}
function pythonWire(raw,isHTTP){
 const end=isHTTP?raw.indexOf('\r\n\r\n'):-1;
 if(end>=0&&end+4<raw.length){const header=pythonPayload(raw.slice(0,end+4)),body=pythonPayload(raw.slice(end+4));return {expression:header.expression+' + '+body.expression,base64:header.base64||body.base64}}
 return pythonPayload(raw);
}
const shell=value=>"'"+String(value).replace(/'/g,"'\\''")+"'";
function destination(host,port,scheme){
 if(!['http','https'].includes(scheme))fail('Выберите схему http или https.');
 if(typeof host!=='string'||!host.trim())fail('Укажите адрес сервиса.');
 host=host.trim();if(host.startsWith('[')&&host.endsWith(']'))host=host.slice(1,-1);
 if(host.includes(':')){
  const [ip,zone,...rest]=host.split('%');if(rest.length||zone&&!/^[A-Za-z0-9_.-]+$/.test(zone))fail('Некорректная IPv6 zone.');
  try{new URL('http://['+ip+']/');}catch{fail('Некорректный IPv6 адрес.');}
 }else if(!/^[A-Za-z0-9_.-]+$/.test(host)||host.startsWith('-'))fail('Укажите hostname или IP без схемы, пути и управляющих символов.');
 if(!/^\d{1,5}$/.test(String(port))||Number(port)<1||Number(port)>65535)fail('Порт должен быть числом от 1 до 65535.');
 return {host,port:Number(port),url:scheme+'://'+(host.includes(':')?'['+host.replace('%','%25')+']':host)+':'+Number(port)};
}
function requestHeaders(request,warnings){
 const headers=normalizedHeaders(request.headers);
 if(request.headers.some(h=>['content-length','transfer-encoding','trailer'].includes(h.name.toLowerCase())))warn(warnings,'Content-Length/Transfer-Encoding не копируются: HTTP-клиент вычислит framing тела заново.');
 return headers;
}
export function generateExport(options={}){
 let {packet,stream={},service={},format,host,port,scheme='http',requestIndex=0}=options&&typeof options==='object'?options:{};
 stream=stream&&typeof stream==='object'?stream:{};service=service&&typeof service==='object'?service:{};
 const prepared=prepareExport(packet,stream,service),warnings=[...prepared.warnings];
 try{
  if(!['requests','curl','pwntools'].includes(format))fail('Неизвестный формат экспорта.');
  if(!prepared.raw)fail(prepared.httpError||'Нет данных для экспорта.');
  if(packet.incoming!==true)fail('Выберите входящий блок запроса.');
  const target=destination(host===undefined?prepared.endpoint.host:host,port===undefined?prepared.endpoint.port:port,scheme);
  if(!Number.isInteger(requestIndex)||requestIndex<0)fail('Некорректный номер запроса.');
  if(prepared.requests.length&&requestIndex>=prepared.requests.length)fail('Такого запроса нет в выбранном блоке.');
  if(!prepared.requests.length&&requestIndex!==0)fail('Такого запроса нет в выбранном блоке.');
  const request=prepared.requests[requestIndex];
  if(format==='pwntools'){
   const kind=packet.kind||'';
   if(kind&&kind!=='raw'&&(!request||kind!=='http'))fail('Разобранное представление не является wire-байтами. Выберите исходный raw-блок.');
   const raw=request?.raw||prepared.raw,protocol=String(stream.protocol||'tcp').toLowerCase();
   if(!['tcp','udp'].includes(protocol))fail('pwntools-экспорт поддерживает TCP и UDP.');
   warn(warnings,'Отправляется только выбранный блок/запрос в новое соединение, а не весь диалог. Проверьте порядок предыдущих сообщений.');
   if(packet.incomplete||stream.incomplete)warn(warnings,'В захвате есть пропуски; экспорт содержит только имеющиеся байты.');
   if(raw.startsWith('PRI * HTTP/2.0')||raw.charCodeAt(0)===0x16&&raw.charCodeAt(1)===3)warn(warnings,'HTTP/2/TLS-байты не являются самостоятельным запросом: нужны состояние соединения и предыдущие сообщения.');
   warn(warnings,'Для HTTP/2/TLS одного блока обычно недостаточно: нужны состояние соединения и предыдущие сообщения.');
   const ssl=request&&scheme==='https'&&protocol==='tcp'?', ssl=True':'';
   const payload=pythonWire(raw,!!request);
   const code=`${payload.base64?'import base64\n':''}import sys\nimport time\nfrom pwn import remote\n\nhost = ${python(target.host)}\nport = ${target.port}\npayload = ${payload.expression}\n\nio = remote(host, port, typ=${python(protocol)}, timeout=15${ssl})\ntry:\n    io.send(payload)\n    reply = bytearray()\n    deadline = time.monotonic() + 10\n    while len(reply) < 256 * 1024 and time.monotonic() < deadline:\n        try:\n            chunk = io.recv(min(4096, 256 * 1024 - len(reply)), timeout=min(1, max(0.01, deadline - time.monotonic())))\n        except EOFError:\n            break\n        if not chunk:\n            break\n        reply.extend(chunk)\n    sys.stdout.buffer.write(reply)\nfinally:\n    io.close()\n`;
   return {code,warnings,error:''};
  }
  if(prepared.httpError)fail(prepared.httpError);
  if(!request)fail('Полный HTTP-запрос не найден.');
  const headers=requestHeaders(request,warnings);
  warn(warnings,'Сохранены захваченные Host, Authorization и Cookie. Новый адрес подключения не заменяет их автоматически.');
  if(format==='requests'){
   const names=new Set();for(const header of headers){const name=header.name.toLowerCase();if(names.has(name))fail('requests не сохраняет повторяющиеся заголовки. Выберите curl или pwntools.');names.add(name);}
   if(!request.target.startsWith('/')||request.target.startsWith('//')||request.method==='CONNECT')fail('requests не воспроизводит этот request-target. Выберите curl или pwntools.');
   warn(warnings,'requests может нормализовать URL и добавить служебные заголовки. Для точного request-target используйте curl, для точных wire-байтов — pwntools.');
   const body=pythonPayload(request.body);
   const code=`${body.base64?'import base64\n':''}import requests\n\nurl = ${python(target.url+request.target)}\nheaders = {\n${headers.map(h=>'    '+python(h.name)+': '+python(h.value)+',').join('\n')}\n}\nbody = ${body.expression}\n\nwith requests.Session() as session:\n    session.trust_env = False\n    session.headers.clear()\n    response = session.request(\n        method=${python(request.method)}, url=url, headers=headers, data=body,\n        timeout=15, allow_redirects=False,\n    )\n    print(response.status_code)\n    print(response.content)\n`;
   return {code,warnings,error:''};
  }
  // -q must be the first curl option, so local .curlrc cannot add redirects or
  // commands. Empty names with ';' deliberately send an empty header value.
  const args=['curl -q','--globoff','--path-as-is',request.version==='1.0'?'--http1.0':'--http1.1','--noproxy '+shell('*'),'--max-time 15','--request '+shell(request.method),'--request-target '+shell(request.target)];
  for(const header of headers){const line=header.name+(header.value?': '+header.value:';');args.push('--header '+(/[^\x00-\x7f]/.test(line)?`"$(printf '%s' ${shell(btoa(line))} | base64 --decode)"`:shell(line)));}
  for(const name of ['Accept','User-Agent','Content-Type','Expect'])if(!headers.some(h=>h.name.toLowerCase()===name.toLowerCase()))args.push('--header '+shell(name+':'));
  args.push('--data-binary @-','--url '+shell(target.url+'/'));
  const body=readableText(request.body);
  const source=body===null?`printf '%s' ${shell(btoa(request.body))} | base64 --decode`:`printf '%s' ${shell(body)}`;
  const code=`${source} | \\\n  ${args.join(' \\\n  ')}\n`;
  return {code,warnings,error:''};
 }catch(error){return {code:'',warnings,error:error.message||'Не удалось создать экспорт.'};}
}
