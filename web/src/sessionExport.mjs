import {MAX_SESSION_BYTES,MAX_SESSION_PACKETS} from './requestExport.mjs';

// A successful result is a stable, complete scan of the raw representation.
// Closing the workbench aborts both metadata and page requests via signal.
export async function loadSessionExport(http,streamId,{signal,onProgress=()=>{}}={}){
 const check=()=>{if(signal?.aborted)throw Error('Loading canceled.');};
 const metadata=async()=>{
  check();const {data}=await http.get(`/stream/${streamId}`,{signal});check();
  if(!data?.id||String(data.id)!==String(streamId))throw Error('Session was deleted or is unavailable.');
  if(data.payloadPurged)throw Error('Session content was purged. Full script unavailable.');
  if(data.incomplete)throw Error('Session capture has gaps. Full script unavailable.');
  return data;
 };
 const stream=await metadata(),packets=[];let cursor=null,bytes=0;
 for(;;){
  check();
  const {data,headers}=await http.post(`/packet/${streamId}`,{startingFrom:cursor,pageSize:50,representation:'raw'},{signal});check();
  if(!Array.isArray(data))throw Error('Server returned an invalid block list.');
  const more=headers?.['x-has-more'];
  if(more!=='true'&&more!=='false'&&more!==true&&more!==false)throw Error('Server did not confirm session completeness (missing X-Has-More).');
  if((more==='true'||more===true)&&!data.length)throw Error('Could not continue loading all session blocks.');
  for(const packet of data){
   if(!Number.isSafeInteger(packet.id)||packet.id<1||packet.id<=(cursor||0))throw Error('Block order changed. Reload the session.');
   if(packet.kind&&packet.kind!=='raw')throw Error('Server returned parsed messages instead of raw bytes.');
   if(typeof packet.content!=='string')throw Error('Session is missing block content.');
   const encoded=packet.content.replace(/\s/g,'');bytes+=Math.floor(encoded.length*3/4)-(encoded.endsWith('==')?2:encoded.endsWith('=')?1:0);
   if(bytes>MAX_SESSION_BYTES||packets.length>=MAX_SESSION_PACKETS)throw Error('Full session exceeds the export limit: 1 MiB or 4096 raw blocks.');
   packets.push(packet);cursor=packet.id;
  }
  onProgress({packets:packets.length,bytes});
  if(more==='false'||more===false)break;
 }
 const after=await metadata();
 for(const field of ['packetsCount','sizeBytes','endTimestamp'])if(stream[field]!==after[field])throw Error('Session changed while loading. Reload to obtain a complete snapshot.');
 if(!packets.length)throw Error('Raw session bytes were deleted or are unavailable.');
 return {stream,packets};
}
