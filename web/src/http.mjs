import Axios from 'axios';

// One policy for every screen, including initial loading and control requests.
export function createHTTPClient(options = {}) {
 const defaults = {baseURL:'/api', withCredentials:true, maxContentLength:20000,
  headers:{Accept:'application/json','Content-Type':'application/json'}, ...options};
 const client = Axios.create({...defaults, timeout:15000});
 const configClient = Axios.create({...defaults, timeout:15000});
 let seconds=10, checkedAt=0, pending=null, revision=0;
 const accept = value => {
  if (!Number.isInteger(value) || value<5 || value>300) return false;
  seconds=value;checkedAt=Date.now();return true;
 };
 async function refresh() {
  if (Date.now()-checkedAt<5000) return;
  if (!pending) {
   const version=revision;
   pending=configClient.get('/v2/config').then(({data})=>{
    if (version===revision) accept(data.timeoutSeconds);
   }).catch(()=>{}).finally(()=>{
    // If discovery is unavailable, still let settings and other pages load
    // using the last known budget. Do not retry writes automatically.
    checkedAt=Date.now();pending=null;
   });
  }
  await pending;
 }
 client.interceptors.request.use(async config=>{
  await refresh();
  config.timeout=(seconds+5)*1000;
  return config;
 });
 client.interceptors.response.use(response=>{
  const value=response.data?.settings?.requestTimeoutSeconds;
  if (accept(value)) revision++;
  return response;
 });
 return client;
}
