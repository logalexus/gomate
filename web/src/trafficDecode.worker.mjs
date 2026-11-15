import {decode,detect} from './trafficDecode.mjs';
self.onmessage=({data})=>{
 try{self.postMessage({result:data.operation==='auto'?detect(data.input):decode(data.input,data.operation)});}
 catch(error){self.postMessage({error:error.message||'Could not decode the data.'});}
};
