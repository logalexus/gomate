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
 if (typeof encoded !== 'string' || !encoded.length) fail('No saved bytes in this block.');
 if (encoded.length > Math.ceil(MAX_EXPORT_BYTES / 3) * 4 + 4096) fail('Block exceeds the 256 KiB export limit.');
 const compact = encoded.replace(/\s/g, '');
 if (!/^[A-Za-z0-9+/]*={0,2}$/.test(compact) || compact.length % 4 !== 0) fail('Invalid Base64 block content.');
 let raw; try { raw = atob(compact); } catch { fail('Invalid Base64 block content.'); }
 if (btoa(raw) !== compact) fail('Non-canonical Base64 block content.');
 if (raw.length > MAX_EXPORT_BYTES) fail('Block exceeds the 256 KiB export limit.');
 return raw;
}
function headerLines(lines) {
 if (lines.length > 512) fail('Too many HTTP headers (limit: 512).');
 return lines.map(line => {
  const colon = line.indexOf(':');
  if (colon < 1 || !TOKEN.test(line.slice(0, colon))) fail('Invalid or folded HTTP header. Choose raw/pwntools.');
  const value = line.slice(colon + 1).replace(/^[ \t]+|[ \t]+$/g, '');
  if (CONTROL.test(value)) fail('HTTP header contains control characters. Choose raw/pwntools.');
  return {name:line.slice(0,colon),value};
 });
}
function requestHead(raw, start = 0) {
 const end = raw.indexOf('\r\n\r\n', start);
 if (end < 0) fail('Truncated HTTP headers: missing final CRLF CRLF.');
 if (end - start > MAX_HEADERS) fail('HTTP headers exceed the 64 KiB limit.');
 const lines = raw.slice(start,end).split('\r\n');
 const first = lines.shift();
 if (/^HTTP\//.test(first)) fail('This is an HTTP response. Select an incoming request.');
 const match = /^([^ ]+) ([^ ]+) HTTP\/(1\.[01])$/.exec(first);
 if (!match || !TOKEN.test(match[1]) || !/^[\x21-\x7e]+$/.test(match[2])) fail('No complete, valid HTTP/1.x request. Select a raw block containing the start of the request.');
 if (match[1] === 'PRI' && match[2] === '*') fail('An HTTP/2 preface cannot be exported as an HTTP/1 request.');
 return {method:match[1],target:match[2],version:match[3],headers:headerLines(lines),bodyStart:end+4};
}
function chunkedBody(raw, start, warnings) {
 let cursor=start, size=0;const chunks=[];
 for(let count=0;count<MAX_EXPORT_BYTES;count++){
  const end=raw.indexOf('\r\n',cursor);
  if(end<0 || end-cursor>8192)fail('Truncated or oversized HTTP chunk header.');
  const line=raw.slice(cursor,end),match=/^([\da-f]+)(?:;[\t\x20-\x7e]*)?$/i.exec(line);
  if(!match || match[1].length>12)fail('Invalid HTTP chunk size.');
  const length=parseInt(match[1],16);cursor=end+2;
  if(length===0){
   const trailers=[];const trailerStart=cursor;
   for(;;){
    const trailerEnd=raw.indexOf('\r\n',cursor);
    if(trailerEnd<0)fail('Truncated end of chunked body.');
    if(trailerEnd-trailerStart>MAX_HEADERS || trailers.length>512)fail('HTTP trailer limit exceeded.');
    const trailer=raw.slice(cursor,trailerEnd);cursor=trailerEnd+2;
    if(!trailer)break;trailers.push(trailer);
   }
   headerLines(trailers);
   if(trailers.length)warn(warnings,'HTTP trailers are not sent by requests/curl; choose pwntools for exact bytes.');
   return {body:chunks.join(''),end:cursor};
  }
  size+=length;
  if(size>MAX_EXPORT_BYTES)fail('HTTP body exceeds the 256 KiB limit.');
  if(cursor+length+2>raw.length)fail('Truncated HTTP chunk. Select a complete raw request.');
  if(raw.slice(cursor+length,cursor+length+2)!=='\r\n')fail('Missing CRLF after HTTP chunk.');
  chunks.push(raw.slice(cursor,cursor+length));cursor+=length+2;
 }
 fail('HTTP chunk limit exceeded.');
}
function parseRawHTTP(raw,warnings){
 const requests=[];let cursor=0;
 while(cursor<raw.length){
  if(requests.length>=128)fail('Block contains more than 128 HTTP requests.');
  const head=requestHead(raw,cursor),cl=values(head.headers,'content-length'),te=values(head.headers,'transfer-encoding');
  if(cl.length>1 || te.length>1 || cl.length&&te.length)fail('Ambiguous HTTP framing: multiple Content-Length/Transfer-Encoding headers. Use pwntools for raw bytes.');
  let body='',end=head.bodyStart;
  if(te.length){
   if(te[0].toLowerCase()!=='chunked')fail('Only Transfer-Encoding: chunked is supported. Choose raw/pwntools.');
   ({body,end}=chunkedBody(raw,end,warnings));
   warn(warnings,'The chunked body is reassembled for requests/curl; the library recalculates its length and framing.');
  }else if(cl.length){
   if(!/^\d+$/.test(cl[0]))fail('Invalid Content-Length.');
   const length=Number(cl[0]);
   if(!Number.isSafeInteger(length)||length>MAX_EXPORT_BYTES)fail('Content-Length exceeds the 256 KiB limit.');
   if(end+length>raw.length)fail('HTTP body is shorter than Content-Length.');
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
  if(!TOKEN.test(method)||!/^[\x21-\x7e]+$/.test(target))fail('HTTP request metadata contains an invalid method/path. Select the original raw block.');
  // The displayed target may contain decoded spaces or CRLF. Match its full
  // known metadata-derived prefix, rather than treating those bytes as headers.
  const targets=[target,percentBytes(target)];
  let matched=false;
  for(const displayed of targets)for(const version of ['1.1','1.0']){
   const prefix=`${method} ${displayed} HTTP/${version}\r\n`;
   if(raw.startsWith(prefix)){view=`${method} ${target} HTTP/${version}\r\n`+raw.slice(prefix.length);matched=true;break;}
  }
  if(!matched)fail('Displayed HTTP request does not match its metadata. Select the original raw block.');
 }else if(service.urldecodeHttpRequests){fail('Original method/path metadata is required to reconstruct the URL. Select a raw block.');}
 const head=requestHead(view);
 if(service.urldecodeHttpRequests&&values(head.headers,'content-type').some(v=>/^application\/x-www-form-urlencoded(?:\s*;|$)/i.test(v)))fail('The form body is already URL-decoded and cannot be reconstructed. Select a raw block.');
 const body=view.slice(head.bodyStart),headers=normalizedHeaders(head.headers,!!packet.ungzipped);
 warn(warnings,'Export reconstructed from parsed HTTP: displayed bytes are normalized and may differ from the wire. Select a raw block for exact replay.');
 if(packet.ungzipped)warn(warnings,'Body is already decompressed; Content-Encoding and the original length were removed.');
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
  if(packet.incoming!==true)fail('Request export is available for incoming blocks only.');
  if(packet.incomplete||stream.incomplete)fail('Capture has gaps. HTTP export is disabled; raw bytes are available through pwntools.');
  if(String(stream.protocol||'tcp').toLowerCase()==='udp')fail('This is a UDP block: requests/curl cannot replay UDP. Choose pwntools.');
  const kind=packet.kind||'';
  if(!HTTP_KINDS.has(kind))fail('This is '+kind+', not a complete HTTP/1 request. Select the original raw block.');
  requests=kind==='http'?parseDecodedHTTP(raw,packet,service,warnings):parseRawHTTP(raw,warnings);
 }catch(error){httpError=error.message||'Could not parse the request.';requests=[];}
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
 if(!['http','https'].includes(scheme))fail('Choose http or https.');
 if(typeof host!=='string'||!host.trim())fail('Enter the service address.');
 host=host.trim();if(host.startsWith('[')&&host.endsWith(']'))host=host.slice(1,-1);
 if(host.includes(':')){
  const [ip,zone,...rest]=host.split('%');if(rest.length||zone&&!/^[A-Za-z0-9_.-]+$/.test(zone))fail('Invalid IPv6 zone.');
  try{new URL('http://['+ip+']/');}catch{fail('Invalid IPv6 address.');}
 }else if(!/^[A-Za-z0-9_.-]+$/.test(host)||host.startsWith('-'))fail('Enter a hostname or IP without a scheme, path or control characters.');
 if(!/^\d{1,5}$/.test(String(port))||Number(port)<1||Number(port)>65535)fail('Port must be an integer from 1 to 65535.');
 return {host,port:Number(port),url:scheme+'://'+(host.includes(':')?'['+host.replace('%','%25')+']':host)+':'+Number(port)};
}
function requestHeaders(request,warnings){
 const headers=normalizedHeaders(request.headers);
 if(request.headers.some(h=>['content-length','transfer-encoding','trailer'].includes(h.name.toLowerCase())))warn(warnings,'Content-Length/Transfer-Encoding are not copied: the HTTP client recalculates body framing.');
 return headers;
}
export function generateExport(options={}){
 let {packet,stream={},service={},format,host,port,scheme='http',requestIndex=0}=options&&typeof options==='object'?options:{};
 stream=stream&&typeof stream==='object'?stream:{};service=service&&typeof service==='object'?service:{};
 const prepared=prepareExport(packet,stream,service),warnings=[...prepared.warnings];
 try{
  if(!['requests','curl','pwntools'].includes(format))fail('Unknown export format.');
  if(!prepared.raw)fail(prepared.httpError||'No data to export.');
  if(packet.incoming!==true)fail('Select an incoming request block.');
  const target=destination(host===undefined?prepared.endpoint.host:host,port===undefined?prepared.endpoint.port:port,scheme);
  if(!Number.isInteger(requestIndex)||requestIndex<0)fail('Invalid request number.');
  if(prepared.requests.length&&requestIndex>=prepared.requests.length)fail('This request does not exist in the selected block.');
  if(!prepared.requests.length&&requestIndex!==0)fail('This request does not exist in the selected block.');
  const request=prepared.requests[requestIndex];
  if(format==='pwntools'){
   const kind=packet.kind||'';
   if(kind&&kind!=='raw'&&(!request||kind!=='http'))fail('Parsed content is not the original wire bytes. Select the original raw block.');
   const raw=request?.raw||prepared.raw,protocol=String(stream.protocol||'tcp').toLowerCase();
   if(!['tcp','udp'].includes(protocol))fail('pwntools export supports TCP and UDP.');
   warn(warnings,'Only the selected block/request is sent over a new connection, not the entire conversation. Check the order of preceding messages.');
   if(packet.incomplete||stream.incomplete)warn(warnings,'Capture has gaps; export includes only available bytes.');
   if(raw.startsWith('PRI * HTTP/2.0')||raw.charCodeAt(0)===0x16&&raw.charCodeAt(1)===3)warn(warnings,'HTTP/2/TLS bytes are not a standalone request: connection state and previous messages are required.');
   warn(warnings,'A single block is usually insufficient for HTTP/2/TLS: connection state and previous messages are required.');
   const ssl=request&&scheme==='https'&&protocol==='tcp'?', ssl=True':'';
   const payload=pythonWire(raw,!!request);
   const code=`${payload.base64?'import base64\n':''}import sys\nimport time\nfrom pwn import remote\n\nhost = ${python(target.host)}\nport = ${target.port}\npayload = ${payload.expression}\n\nio = remote(host, port, typ=${python(protocol)}, timeout=15${ssl})\ntry:\n    io.send(payload)\n    reply = bytearray()\n    deadline = time.monotonic() + 10\n    while len(reply) < 256 * 1024 and time.monotonic() < deadline:\n        try:\n            chunk = io.recv(min(4096, 256 * 1024 - len(reply)), timeout=min(1, max(0.01, deadline - time.monotonic())))\n        except EOFError:\n            break\n        if not chunk:\n            break\n        reply.extend(chunk)\n    sys.stdout.buffer.write(reply)\nfinally:\n    io.close()\n`;
   return {code,warnings,error:''};
  }
  if(prepared.httpError)fail(prepared.httpError);
  if(!request)fail('No complete HTTP request found.');
  const headers=requestHeaders(request,warnings);
  warn(warnings,'Captured Host, Authorization and Cookie headers are preserved. Changing the connection address does not update them automatically.');
  if(format==='requests'){
   const names=new Set();for(const header of headers){const name=header.name.toLowerCase();if(names.has(name))fail('requests does not preserve duplicate headers. Choose curl or pwntools.');names.add(name);}
   if(!request.target.startsWith('/')||request.target.startsWith('//')||request.method==='CONNECT')fail('requests cannot reproduce this request-target. Choose curl or pwntools.');
   warn(warnings,'requests may normalize the URL and add headers. Use curl for an exact request-target or pwntools for exact wire bytes.');
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
 }catch(error){return {code:'',warnings,error:error.message||'Could not generate the export.'};}
}

// Whole-session exports always use the transport representation, never decoded
// copies. Limits fail closed: a prefix must not look like the complete session.
export const MAX_SESSION_BYTES = 1024 * 1024;
export const MAX_SESSION_PACKETS = 4096;
export function prepareSessionExport(packets=[],stream={},service={}){
 const warnings=[],events=[];let requests=[],httpError='',error='',bytes=0;
 try{
  if(stream.payloadPurged)fail('Session content was deleted. Full script unavailable.');
  if(stream.incomplete)fail('Session capture has gaps. Full script unavailable.');
  if(String(stream.protocol||'').toLowerCase()!=='tcp')fail('Full session export supports HTTP over TCP and TCP.');
  if(!Array.isArray(packets)||packets.length>MAX_SESSION_PACKETS)fail('Session exceeds the 4096-block export limit.');
  const rawPackets=packets.filter(p=>!p.kind||p.kind==='raw');
  if(!rawPackets.length)fail('Raw session bytes are unavailable. Parsed messages cannot replace the full TCP stream.');
  for(const packet of rawPackets){
   if(packet.incomplete)fail('One of the blocks has gaps. Full script unavailable.');
   if(typeof packet.incoming!=='boolean')fail('Unknown session block direction.');
   const raw=packetBytes(packet);bytes+=raw.length;
   if(bytes>MAX_SESSION_BYTES)fail('Session exceeds the 1 MiB raw byte export limit.');
   const last=events[events.length-1];
   if(last&&last.incoming===packet.incoming)last.raw+=raw;
   else events.push({incoming:packet.incoming,raw});
  }
  if(!events.some(e=>e.incoming))fail('No saved client data in this session.');
 }catch(e){error=e.message;}
 if(!error)try{requests=parseRawHTTP(events.filter(e=>e.incoming).map(e=>e.raw).join(''),warnings)}catch(e){httpError=e.message;}
 return {events,requests,httpError,error,warnings,bytes,endpoint:endpointFor(stream,service,requests,events.find(e=>e.incoming)?.raw||'')};
}
export function generateSessionExport({packets=[],stream={},service={},format='requests',host,port,scheme='http'}={}){
 const prepared=prepareSessionExport(packets,stream,service),warnings=[...prepared.warnings];
 try{
  if(prepared.error)fail(prepared.error);
  const target=destination(host===undefined?prepared.endpoint.host:host,port===undefined?prepared.endpoint.port:port,scheme);
  warn(warnings,'Script includes all saved raw blocks at the time of loading. New data from an active session is not added.');
  if(format==='tcp'){
   const first=prepared.events.find(e=>e.incoming).raw;
   if(first.charCodeAt(0)===0x16&&first.charCodeAt(1)===3)fail('TLS handshake captured. An encrypted session cannot be replayed over a new connection without TLS keys and state.');
   if(scheme==='https')warn(warnings,'TLS wraps the saved plaintext bytes. Use it only if the new service expects TLS.');
   warn(warnings,'TCP preserves send order and bytes; each response is read to its captured length. If the response length changes, adjust recv_exact in the script.');
   warn(warnings,'Capture timing and dynamic values are not replayed. Server responses are printed but are not used in subsequent messages.');
   const steps=prepared.events.map((e,i)=>e.incoming?`    # ${i+1}. Client → server: ${e.raw.length} bytes\n    connection.sendall(base64.b64decode(${python(btoa(e.raw))}, validate=True))`:`    # ${i+1}. Server → client: ${e.raw.length} bytes\n    sys.stdout.buffer.write(recv_exact(connection, ${e.raw.length}))\n    sys.stdout.buffer.flush()`).join('\n');
   return {error:'',warnings,code:`import base64\nimport socket\nimport sys\nimport time\n${scheme==='https'?'import ssl\n':''}\nHOST = ${python(target.host)}\nPORT = ${target.port}\nTIMEOUT = 15\n\ndef recv_exact(connection, size):\n    reply = bytearray()\n    deadline = time.monotonic() + TIMEOUT\n    while len(reply) < size:\n        remaining = deadline - time.monotonic()\n        if remaining <= 0:\n            raise TimeoutError(f"Expected {size} bytes, received {len(reply)}")\n        connection.settimeout(remaining)\n        chunk = connection.recv(min(65536, size - len(reply)))\n        if not chunk:\n            raise EOFError(f"Expected {size} bytes, received {len(reply)}")\n        reply.extend(chunk)\n    connection.settimeout(TIMEOUT)\n    return bytes(reply)\n\nwith socket.create_connection((HOST, PORT), timeout=TIMEOUT) as transport:\n${scheme==='https'?'    connection = ssl.create_default_context().wrap_socket(transport, server_hostname=HOST)':'    connection = transport'}\n    try:\n${steps.split('\n').map(line=>'    '+line).join('\n')}\n    finally:\n        connection.close()\n`};
  }
  if(format!=='requests')fail('Choose HTTP · requests or TCP · socket.');
  if(prepared.httpError)fail(prepared.httpError+' For TCP, choose the TCP · socket template.');
  if(!prepared.requests.length)fail('No complete HTTP requests found.');
  const rows=prepared.requests.map(request=>{
   if(!request.target.startsWith('/')||request.target.startsWith('//')||request.method==='CONNECT'||values(request.headers,'upgrade').length)fail('Tunnels, Upgrade and this request-target are unsupported in HTTP session export. Choose TCP · socket.');
   const headers=requestHeaders(request,warnings),names=new Set();
   for(const header of headers){const name=header.name.toLowerCase();if(names.has(name))fail('requests does not preserve duplicate headers. Choose TCP · socket.');names.add(name);}
   return `    (${python(request.method)}, ${python(request.target)}, {${headers.filter(h=>h.name.toLowerCase()!=='cookie').map(h=>python(h.name)+': '+python(h.value)).join(', ')}}, base64.b64decode(${python(btoa(request.body))}, validate=True)),`;
  });
  const initialCookie=values(prepared.requests[0].headers,'cookie')[0]||'';
  warn(warnings,'Cookies from the first request set the initial state. Subsequent cookies are updated from Set-Cookie responses; captured Cookie headers from later requests do not override new values.');
  warn(warnings,'Host and Authorization are preserved. Tokens in URLs, headers and bodies keep their captured values; add response extraction if needed.');
  warn(warnings,'requests recalculates framing, may normalize URLs and sends requests sequentially without captured delays. Use TCP · socket for wire bytes.');
  return {error:'',warnings,code:`import base64\nfrom http.cookies import SimpleCookie\nfrom urllib.parse import urlsplit\nimport requests\n\nHOST = ${python(target.host)}\nPORT = ${target.port}\nSCHEME = ${python(scheme)}\nTIMEOUT = 15\nbase_url = f"{SCHEME}://{'[' + HOST + ']' if ':' in HOST else HOST}:{PORT}"\n\n# method, path, headers, body — in captured request order\nsteps = [\n${rows.join('\n')}\n]\n\nwith requests.Session() as session:\n    session.trust_env = False\n    session.headers.clear()\n    initial = SimpleCookie()\n    initial.load(${python(initialCookie)})\n    cookie_host = next((v for k, v in steps[0][2].items() if k.lower() == "host"), HOST)\n    cookie_domain = urlsplit("http://" + cookie_host).hostname or HOST\n    if "." not in cookie_domain and ":" not in cookie_domain:\n        cookie_domain += ".local"\n    for name, morsel in initial.items():\n        session.cookies.set(name, morsel.value, domain=cookie_domain, path="/")\n    for index, (method, path, headers, body) in enumerate(steps, 1):\n        response = session.request(\n            method, base_url + path, headers=headers, data=body,\n            timeout=TIMEOUT, allow_redirects=False,\n        )\n        print(f"[{index}/{len(steps)}] {method} {path}: {response.status_code}")\n        print(response.content)\n`};
 }catch(e){return {code:'',warnings,error:e.message||'Could not generate the session script.'};}
}
