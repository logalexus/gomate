<template>
 <aside class="gm-session-sidebar" aria-label="Traffic sessions">
  <header class="session-heading"><h2>Sessions <span class="session-total">{{ streams.length }}</span></h2><span class="session-mode" :class="{'is-live':live}"><span/>{{ search.query?(loading?'Searching':'Search'):live?'LIVE':cursor!==null?'History':'Pause' }}</span><button type="button" class="btn btn-sm btn-outline-secondary" :aria-pressed="String(search.query?loading:$store.state.pause)" :aria-label="search.query?(loading?'Stop search':'Restart search'):$store.state.pause?'Resume updates':'Pause'" :title="search.query?(loading?'Stop search':'Restart search'):$store.state.pause?'Resume updates':'Pause'" @click="togglePause"><i :class="search.query?(loading?'fas fa-stop':'fas fa-redo'):$store.state.pause?'fas fa-play':'fas fa-pause'" aria-hidden="true"/></button></header>
  <TrafficFilters :search-error="searchError" @search="reset"/>
  <p v-if="search.query" class="search-progress" role="status">{{ loading?'Searching saved traffic…':error?'Search failed':stopped?'Search stopped':hasMore?'More history available':'Search complete' }} · {{ scanned }} sessions checked <button v-if="stopped&&!loading" @click="load(true)">Continue</button></p>
  <p v-if="cursor!==null&&!search.query" class="session-notice">You are browsing history. <button @click="latest">Jump to new sessions</button></p>
  <p v-if="error&&!searchError" class="session-error" role="alert">{{ error }} <button class="btn btn-sm btn-outline-secondary" @click="load">Retry</button></p>
  <div class="session-list-area" :aria-busy="String(loading)">
   <VirtualList v-if="streams.length" :items="streams" ref="list" class="session-list"><template #default="{item}"><SidebarStream :stream="item" @favorite-changed="favoriteChanged"/></template></VirtualList>
   <div v-else class="session-empty"><i :class="loading?'fas fa-circle-notch fa-spin':'fas fa-inbox'" aria-hidden="true"/><strong>{{ loading?'Loading sessions…':'No sessions found' }}</strong><p>{{ search.query?(loading?'Scanning saved requests and responses.':'Try another search or clear some filters.'):(loading?'Loading data for the selected services.':'Check your filters and capture status. New sessions will appear here automatically.') }}</p></div>
  </div>
  <footer class="session-pagination" title="Ctrl + ↑ / ↓ to switch sessions"><div><button type="button" class="btn btn-sm btn-outline-secondary" :disabled="!history.length||loading" @click="newer" aria-label="Newer sessions"><i class="fas fa-arrow-left" aria-hidden="true"/> Newer</button><button type="button" class="btn btn-sm btn-outline-secondary" :disabled="!hasMore||loading" @click="older">Older <i class="fas fa-arrow-right" aria-hidden="true"/></button><button type="button" class="btn btn-sm btn-outline-primary" :disabled="loading" @click="latest">Latest</button></div></footer>
 </aside>
</template>
<script>
import SidebarStream from './SidebarStream';
import VirtualList from './VirtualList';
import TrafficFilters from './TrafficFilters';
import {streamLocation,patternSelection,trafficSearch} from '../trafficFilters.mjs';
export default {
 name:'Sidebar',components:{SidebarStream,VirtualList,TrafficFilters},props:['servicePort','streamId'],
 data:()=>({streams:[],cursor:null,history:[],hasMore:false,loading:false,generation:0,error:'',searchError:'',nextCursor:null,scanned:0,stopped:false,abort:null}),
 computed:{search(){return trafficSearch(this.$route.query)},filterKey(){return JSON.stringify([this.$route.params.servicePort,patternSelection(this.$route.query),this.search,this.$store.state.displayFavoritesOnly])},live(){return !this.search.query&&!this.$store.state.pause&&this.cursor===null}},
 mounted(){this.reset();document.addEventListener('keydown',this.navigate)},
 beforeDestroy(){document.removeEventListener('keydown',this.navigate);this.generation++;this.abort?.abort()},
 watch:{filterKey:'reset'},
 methods:{
  async load(resume=false){
   this.abort?.abort();const abort=new AbortController();this.abort=abort;const generation=++this.generation;
   this.loading=true;this.stopped=false;this.error='';this.searchError='';
   const pageSize=Math.max(1,Math.min(100,Number(this.$store.state.pageSize)||25));
   const filters={pageSize,patternIds:patternSelection(this.$route.query),favorites:this.$store.state.displayFavoritesOnly};
   try{
    if(this.search.query){
     if(resume!==true){this.streams=[];this.scanned=0;this.nextCursor=this.cursor;this.hasMore=false}
     do{
      const {data}=await this.$http.post('/v2/traffic/search',{...filters,pageSize:pageSize-this.streams.length,...this.search,port:Number(this.$route.params.servicePort)||0,cursor:this.nextCursor},{signal:abort.signal});
      if(generation!==this.generation)return;
      this.streams=[...this.streams,...data.streams];this.scanned+=data.scanned;this.nextCursor=data.next;this.hasMore=!!data.next;
      if(!data.next||this.streams.length>=pageSize)break;
      await new Promise(resolve=>setTimeout(resolve,60));
     }while(!abort.signal.aborted);
    }else{
     const {data}=await this.$http.post(`/stream/${this.$route.params.servicePort||'all'}`,{...filters,startingFrom:this.cursor},{signal:abort.signal});
     if(generation!==this.generation)return;this.streams=data;this.hasMore=data.length===pageSize;
    }
   }catch(e){if(generation===this.generation&&!abort.signal.aborted){this.error=e.response?.data?.error||'Could not load sessions.';if(e.response?.status===400&&this.search.query)this.searchError=this.error;else if(this.search.query)this.stopped=true}}
   finally{if(generation===this.generation)this.loading=false}
  },
  refreshLive(){if(!this.loading&&this.live)return this.load()},
  reset(){this.$refs.list?.scrollToIndex(0);this.cursor=null;this.history=[];return this.load()},
  older(){if(!this.hasMore)return;this.history.push(this.cursor);if(this.history.length>1000)this.history.shift();this.cursor=this.search.query?this.nextCursor:this.streams[this.streams.length-1].id;this.$refs.list?.scrollToIndex(0);this.load()},
  newer(){this.cursor=this.history.pop()??null;this.$refs.list?.scrollToIndex(0);this.load()},
  latest(){if(this.$store.state.pause)this.$store.commit('togglePause');this.reset()},
  togglePause(){if(this.search.query){if(this.loading){this.abort?.abort();this.generation++;this.loading=false;this.stopped=true}else this.reset();return}this.$store.commit('togglePause');if(!this.$store.state.pause)this.reset()},
  favoriteChanged(){if(this.$store.state.displayFavoritesOnly)this.load()},
  navigate(e){if(!e.ctrlKey||!['ArrowUp','ArrowDown'].includes(e.key)||e.target.closest('input,textarea,select,[contenteditable=true],.modal'))return;const index=this.streams.findIndex(s=>s.id===Number(this.$route.params.streamId)),next=index+(e.key==='ArrowUp'?-1:1),stream=this.streams[next];if(!stream)return;e.preventDefault();this.$router.push(streamLocation(this.$route,stream)).catch(()=>{});this.$refs.list?.scrollToIndex(next)},
 },
};
</script>
<style scoped>
.search-progress{display:flex;align-items:center;gap:5px;flex-wrap:wrap;padding:5px 10px;margin:0;font-size:.75rem;color:var(--gm-muted);border-bottom:1px solid var(--gm-border)}.search-progress button{border:0;background:none;color:var(--gm-accent);padding:0;cursor:pointer;text-decoration:underline}
.gm-session-sidebar{display:flex;flex-direction:column;min-width:0;background:var(--gm-panel);border:1px solid var(--gm-border);border-radius:var(--gm-radius);overflow:hidden;height:100%;min-height:0}.session-heading{padding:7px 10px;display:flex;align-items:center;justify-content:space-between;gap:8px}.session-heading h2{font-size:.9rem;letter-spacing:-.04em;margin:0;font-weight:700}.session-mode{font-size:.75rem;letter-spacing:.06em;display:flex;align-items:center;gap:6px;color:var(--gm-muted);padding:5px 7px;border:1px solid var(--gm-border);border-radius:5px}.session-mode span{width:6px;height:6px;background:currentColor;border-radius:50%}.session-mode.is-live{color:var(--gm-success)}.session-list-area{flex:1;min-height:0;position:relative}.session-list{height:100%!important}.session-empty{padding:32px 20px;text-align:center;display:flex;flex-direction:column;align-items:center;gap:12px;color:var(--gm-muted)}.session-empty>i{font-size:1.5rem}.session-empty strong{font-size:.85rem;color:var(--gm-text)}.session-empty p{font-size:.75rem;line-height:1.6;max-width:230px}.session-pagination{padding:5px 10px;border-top:1px solid var(--gm-border)}.session-pagination>div{display:flex;gap:6px;flex-wrap:wrap}.session-notice,.session-error{padding:10px 14px;margin:0;font-size:.75rem;color:var(--gm-warning);background:var(--gm-panel-alt)}.session-notice button{border:0;background:none;color:var(--gm-accent);padding:0;text-decoration:underline}.session-error{color:var(--gm-danger)}@media(max-width:800px){.gm-session-sidebar{height:min(640px,75dvh);min-height:420px}.session-heading{padding-top:7px}}
.session-heading h2{margin-right:auto}.session-total{font-size:.75rem;color:var(--gm-muted);font-weight:400;margin-left:4px}.session-heading>.btn{min-width:28px;padding:3px 6px}.session-pagination .btn{font-size:.75rem;padding:3px 8px;min-height:26px}
</style>
