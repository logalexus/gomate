<template>
 <aside class="gm-session-sidebar" aria-label="Сессии трафика">
  <header class="session-heading"><h2>Сессии <span class="session-total">{{ streams.length }}</span></h2><span class="session-mode" :class="{'is-live':live}"><span/>{{ live?'LIVE':cursor!==null?'История':'Пауза' }}</span><button type="button" class="btn btn-sm btn-outline-secondary" :aria-pressed="String($store.state.pause)" :aria-label="$store.state.pause?'Продолжить обновление':'Пауза'" :title="$store.state.pause?'Продолжить обновление':'Пауза'" @click="togglePause"><i :class="$store.state.pause?'fas fa-play':'fas fa-pause'" aria-hidden="true"/></button></header>
  <TrafficFilters/>
  <p v-if="cursor!==null" class="session-notice">Вы просматриваете историю. <button @click="latest">К новым сессиям</button></p>
  <p v-if="error" class="session-error" role="alert">{{ error }} <button class="btn btn-sm btn-outline-secondary" @click="load">Повторить</button></p>
  <div class="session-list-area" :aria-busy="String(loading)">
   <VirtualList v-if="streams.length" :items="streams" ref="list" class="session-list"><template #default="{item}"><SidebarStream :stream="item" @favorite-changed="favoriteChanged"/></template></VirtualList>
   <div v-else class="session-empty"><i :class="loading?'fas fa-circle-notch fa-spin':'fas fa-inbox'" aria-hidden="true"/><strong>{{ loading?'Загрузка сессий…':'Сессий не найдено' }}</strong><p>{{ loading?'Получаем данные выбранных сервисов.':'Проверьте фильтры и состояние захвата. Новые сессии появятся здесь автоматически.' }}</p></div>
  </div>
  <footer class="session-pagination" title="Ctrl + ↑ / ↓ — переключить сессию"><div><button type="button" class="btn btn-sm btn-outline-secondary" :disabled="!history.length||loading" @click="newer" aria-label="Более новые сессии"><i class="fas fa-arrow-left" aria-hidden="true"/> Новее</button><button type="button" class="btn btn-sm btn-outline-secondary" :disabled="!hasMore||loading" @click="older">Старее <i class="fas fa-arrow-right" aria-hidden="true"/></button><button type="button" class="btn btn-sm btn-outline-primary" :disabled="loading" @click="latest">К новым</button></div></footer>
 </aside>
</template>
<script>
import SidebarStream from './SidebarStream';
import VirtualList from './VirtualList';
import TrafficFilters from './TrafficFilters';
import {streamLocation,patternSelection} from '../trafficFilters.mjs';
export default {
 name:'Sidebar',components:{SidebarStream,VirtualList,TrafficFilters},props:['servicePort','streamId'],
 data:()=>({streams:[],cursor:null,history:[],hasMore:false,loading:false,generation:0,error:''}),
 computed:{live(){return !this.$store.state.pause&&this.cursor===null}},
 mounted(){this.reset();document.addEventListener('keydown',this.navigate)},
 beforeDestroy(){document.removeEventListener('keydown',this.navigate);this.generation++},
 watch:{'$route.params.servicePort':'reset','$route.query.pattern':'reset','$route.query.patterns':'reset','$store.state.displayFavoritesOnly':'reset'},
 methods:{
  async load(){const generation=++this.generation;this.loading=true;const pageSize=Math.max(1,Math.min(100,Number(this.$store.state.pageSize)||25));try{const {data}=await this.$http.post(`/stream/${this.$route.params.servicePort||'all'}`,{startingFrom:this.cursor,pageSize,patternIds:patternSelection(this.$route.query),favorites:this.$store.state.displayFavoritesOnly});if(generation!==this.generation)return;this.streams=data;this.hasMore=data.length===pageSize;this.error=''}catch(e){if(generation===this.generation)this.error='Не удалось загрузить сессии.'}finally{if(generation===this.generation)this.loading=false}},
  refreshLive(){if(!this.loading&&this.live)return this.load()},
  reset(){this.$refs.list?.scrollToIndex(0);this.cursor=null;this.history=[];return this.load()},
  older(){if(!this.streams.length)return;this.history.push(this.cursor);if(this.history.length>1000)this.history.shift();this.cursor=this.streams[this.streams.length-1].id;this.$refs.list?.scrollToIndex(0);this.load()},
  newer(){this.cursor=this.history.pop()??null;this.$refs.list?.scrollToIndex(0);this.load()},
  latest(){if(this.$store.state.pause)this.$store.commit('togglePause');this.reset()},
  togglePause(){this.$store.commit('togglePause');if(!this.$store.state.pause)this.reset()},
  favoriteChanged(){if(this.$store.state.displayFavoritesOnly)this.load()},
  navigate(e){if(!e.ctrlKey||!['ArrowUp','ArrowDown'].includes(e.key)||e.target.closest('input,textarea,select,[contenteditable=true],.modal'))return;const index=this.streams.findIndex(s=>s.id===Number(this.$route.params.streamId)),next=index+(e.key==='ArrowUp'?-1:1),stream=this.streams[next];if(!stream)return;e.preventDefault();this.$router.push(streamLocation(this.$route,stream)).catch(()=>{});this.$refs.list?.scrollToIndex(next)},
 },
};
</script>
<style scoped>
.gm-session-sidebar{display:flex;flex-direction:column;min-width:0;background:var(--gm-panel);border:1px solid var(--gm-border);border-radius:var(--gm-radius);overflow:hidden;height:100%;min-height:0}.session-heading{padding:7px 10px;display:flex;align-items:center;justify-content:space-between;gap:8px}.session-heading h2{font-size:.9rem;letter-spacing:-.04em;margin:0;font-weight:700}.session-mode{font-size:.75rem;letter-spacing:.06em;display:flex;align-items:center;gap:6px;color:var(--gm-muted);padding:5px 7px;border:1px solid var(--gm-border);border-radius:5px}.session-mode span{width:6px;height:6px;background:currentColor;border-radius:50%}.session-mode.is-live{color:var(--gm-success)}.session-list-area{flex:1;min-height:0;position:relative}.session-list{height:100%!important}.session-empty{padding:32px 20px;text-align:center;display:flex;flex-direction:column;align-items:center;gap:12px;color:var(--gm-muted)}.session-empty>i{font-size:1.5rem}.session-empty strong{font-size:.85rem;color:var(--gm-text)}.session-empty p{font-size:.75rem;line-height:1.6;max-width:230px}.session-pagination{padding:5px 10px;border-top:1px solid var(--gm-border)}.session-pagination>div{display:flex;gap:6px;flex-wrap:wrap}.session-notice,.session-error{padding:10px 14px;margin:0;font-size:.75rem;color:var(--gm-warning);background:var(--gm-panel-alt)}.session-notice button{border:0;background:none;color:var(--gm-accent);padding:0;text-decoration:underline}.session-error{color:var(--gm-danger)}@media(max-width:800px){.gm-session-sidebar{height:min(640px,75dvh);min-height:420px}.session-heading{padding-top:7px}}
.session-heading h2{margin-right:auto}.session-total{font-size:.75rem;color:var(--gm-muted);font-weight:400;margin-left:4px}.session-heading>.btn{min-width:28px;padding:3px 6px}.session-pagination .btn{font-size:.75rem;padding:3px 8px;min-height:26px}
</style>
