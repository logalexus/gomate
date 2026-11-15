import {MAX_SESSION_BYTES,MAX_SESSION_PACKETS} from './requestExport.mjs';

// A successful result is a stable, complete scan of the raw representation.
// Closing the workbench aborts both metadata and page requests via signal.
export async function loadSessionExport(http,streamId,{signal,onProgress=()=>{}}={}){
 const check=()=>{if(signal?.aborted)throw Error('Загрузка отменена.');};
 const metadata=async()=>{
  check();const {data}=await http.get(`/stream/${streamId}`,{signal});check();
  if(!data?.id||String(data.id)!==String(streamId))throw Error('Сессия удалена или недоступна.');
  if(data.payloadPurged)throw Error('Содержимое сессии очищено. Полный скрипт недоступен.');
  if(data.incomplete)throw Error('В захвате сессии есть пропуски. Полный скрипт недоступен.');
  return data;
 };
 const stream=await metadata(),packets=[];let cursor=null,bytes=0;
 for(;;){
  check();
  const {data,headers}=await http.post(`/packet/${streamId}`,{startingFrom:cursor,pageSize:50,representation:'raw'},{signal});check();
  if(!Array.isArray(data))throw Error('Сервер вернул некорректный список блоков.');
  const more=headers?.['x-has-more'];
  if(more!=='true'&&more!=='false'&&more!==true&&more!==false)throw Error('Сервер не подтвердил полноту сессии (нет X-Has-More).');
  if((more==='true'||more===true)&&!data.length)throw Error('Не удалось продолжить загрузку всех блоков сессии.');
  for(const packet of data){
   if(!Number.isSafeInteger(packet.id)||packet.id<1||packet.id<=(cursor||0))throw Error('Порядок блоков изменился. Загрузите сессию заново.');
   if(packet.kind&&packet.kind!=='raw')throw Error('Сервер вернул разобранные сообщения вместо исходных байтов.');
   if(typeof packet.content!=='string')throw Error('В сессии отсутствует содержимое блока.');
   const encoded=packet.content.replace(/\s/g,'');bytes+=Math.floor(encoded.length*3/4)-(encoded.endsWith('==')?2:encoded.endsWith('=')?1:0);
   if(bytes>MAX_SESSION_BYTES||packets.length>=MAX_SESSION_PACKETS)throw Error('Полная сессия превышает лимит экспорта: 1 MiB или 4096 исходных блоков.');
   packets.push(packet);cursor=packet.id;
  }
  onProgress({packets:packets.length,bytes});
  if(more==='false'||more===false)break;
 }
 const after=await metadata();
 for(const field of ['packetsCount','sizeBytes','endTimestamp'])if(stream[field]!==after[field])throw Error('Сессия изменилась во время загрузки. Повторите загрузку, чтобы получить целый снимок.');
 if(!packets.length)throw Error('Исходные байты сессии удалены или недоступны.');
 return {stream,packets};
}
