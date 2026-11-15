<template>
 <section class="gm-traffic-filters" aria-label="Session filters">
  <form class="traffic-search" role="search" aria-label="Search traffic" @submit.prevent="applySearch">
   <label class="sr-only" for="traffic-search">Search saved traffic</label>
   <input id="traffic-search" ref="search" v-model="draft" type="search" maxlength="512" :placeholder="regex?'Search traffic with regex…':'Search traffic…'" :aria-invalid="String(!!searchError)" :aria-describedby="searchError?'traffic-search-error':null" autocomplete="off" spellcheck="false" @keydown.esc.prevent="clearSearch">
   <button type="button" class="filter-icon regex-toggle" :class="{'is-active':regex}" :aria-pressed="String(regex)" aria-label="Use regular expression" title="Regular expression (RE2). Search ignores case." @click="regex=!regex">.*</button>
   <button v-if="draft||search.query" type="button" class="filter-icon" aria-label="Clear search" title="Clear search" @click="clearSearch"><i class="fas fa-times" aria-hidden="true"/></button>
   <button type="submit" class="filter-icon search-submit" aria-label="Search traffic" title="Search (Enter)"><i class="fas fa-search" aria-hidden="true"/></button>
  </form>
  <p v-if="searchError" id="traffic-search-error" class="search-error" role="alert">{{ searchError }}</p>
  <div class="service-picker">
   <label class="sr-only" for="traffic-service">Service</label><select id="traffic-service" class="custom-select custom-select-sm" :value="service" @change="setService($event.target.value)"><option value="">All services</option><option v-for="item in services" :key="item.port" :value="String(item.port)">{{ item.name }} · {{ item.port }}</option><option v-if="service&&!currentService" :value="service">Port {{ service }}</option></select>
   <button type="button" class="filter-icon" aria-label="Add service" title="Add service" @click="$root.$emit('gm:add-service')"><i class="fas fa-plus" aria-hidden="true"/></button><button v-if="currentService" type="button" class="filter-icon" aria-label="Configure selected service" title="Configure service" @click="$root.$emit('gm:edit-service',currentService)"><i class="fas fa-cog" aria-hidden="true"/></button>
   <button v-if="activeCount" type="button" class="filter-icon" aria-label="Reset all filters" title="Reset all filters" @click="clear"><i class="fas fa-times" aria-hidden="true"/></button>
  </div>
  <div class="filter-labels" role="group" aria-label="Labels: all selected must match">
   <button type="button" class="filter-chip" :class="{'is-active':favorites}" :aria-pressed="String(favorites)" @click="toggleFavorites"><i :class="favorites?'fas fa-star':'far fa-star'" aria-hidden="true"/> Favorites</button>
   <button v-for="item in availablePatterns" :key="item.id" type="button" class="filter-chip" :class="{'is-active':selected.includes(item.id)}" :aria-pressed="String(selected.includes(item.id))" :disabled="selected.length>=100&&!selected.includes(item.id)" :title="item.name+(item.missing?' · unavailable':'')" @click="togglePattern(item.id)"><i :class="selected.includes(item.id)?'fas fa-check':'fas fa-hashtag'" aria-hidden="true"/><span>{{ item.name }}</span></button>
  </div>
  <span v-if="selected.length>1" class="filter-match-mode">Match all {{ selected.length }} labels</span>
 </section>
</template>
<script>
import {serviceScope,filterLocation,patternSelection,trafficSearch} from '../trafficFilters.mjs';
export default {
 props:{searchError:{type:String,default:''}},
 data(){const search=trafficSearch(this.$route.query);return {draft:search.query,regex:search.mode==='regex'}},
 watch:{'$route.query.q':'syncSearch','$route.query.searchMode':'syncSearch'},
 computed:{
  search(){return trafficSearch(this.$route.query)},
  service(){return serviceScope(this.$route.params.servicePort)},selected(){return patternSelection(this.$route.query)},
  services(){return this.$store.state.services},patterns(){return this.$store.state.patterns.filter(p=>!p.deleted&&p.actionType==='FIND')},favorites(){return this.$store.state.displayFavoritesOnly},
  availablePatterns(){return [...this.patterns,...this.selected.filter(id=>!this.patterns.some(p=>p.id===id)).map(id=>({id,name:'Pattern #'+id,missing:true}))]},
  currentService(){return this.services.find(s=>String(s.port)===this.service)},activeCount(){return Number(!!this.service)+this.selected.length+Number(this.favorites)+Number(!!this.search.query)},
 },
 methods:{
  syncSearch(){this.draft=this.search.query;this.regex=this.search.mode==='regex'},
  applySearch(){const mode=this.regex?'regex':'text';if(this.draft===this.search.query&&mode===this.search.mode)this.$emit('search');else this.go({search:this.draft,searchMode:mode})},
  clearSearch(){this.draft='';this.regex=false;if(this.search.query||this.$route.query.searchMode)this.go({search:''});this.$refs.search.focus()},
  go(changes){const target=filterLocation(this.$route,changes);this.$router.push(target).catch(e=>{if(e.name!=='NavigationDuplicated')throw e})},
  setService(service){this.go({service})},togglePattern(id){this.go({patterns:this.selected.includes(id)?this.selected.filter(p=>p!==id):[...this.selected,id]})},
  toggleFavorites(){this.$store.commit('toggleDisplayFavoritesOnly');if(this.$route.params.streamId)this.go({})},
  clear(){this.draft='';this.regex=false;if(this.favorites)this.$store.commit('toggleDisplayFavoritesOnly');this.go({service:'',patterns:[],search:''})},
 },
};
</script>
<style scoped>
.traffic-search{display:flex;align-items:center;gap:4px;margin:0 0 6px}.traffic-search input{width:0;min-width:0;flex:1;height:30px;padding:4px 7px;border:1px solid var(--gm-border);border-radius:5px;background:var(--gm-panel-alt);color:var(--gm-text);font-size:.75rem}.traffic-search input:focus-visible,.traffic-search button:focus-visible{outline:2px solid var(--gm-accent);outline-offset:1px}.traffic-search input[aria-invalid=true]{border-color:var(--gm-danger)}.traffic-search .is-active,.search-submit{color:var(--gm-accent);background:var(--gm-accent-soft);border-color:var(--gm-accent)}.regex-toggle{font-family:monospace;font-weight:700}.search-error{font-size:.75rem;color:var(--gm-danger);margin:0 0 6px;overflow-wrap:anywhere}@media(pointer:coarse){.traffic-search input{height:34px}}
.gm-traffic-filters{padding:8px 10px;border-bottom:1px solid var(--gm-border);background:var(--gm-panel);flex-shrink:0}.service-picker{display:flex;align-items:center;gap:4px}.service-picker select{min-width:0;flex:1;height:30px;font-size:.75rem}.filter-icon{border:1px solid var(--gm-border);border-radius:5px;background:transparent;color:var(--gm-muted);width:30px;height:30px;flex-shrink:0;cursor:pointer;font-size:.75rem}.filter-icon:hover{color:var(--gm-text);background:var(--gm-panel-alt)}.filter-labels{display:flex;flex-wrap:wrap;align-content:start;gap:4px;margin-top:6px;max-height:130px;overflow:auto;padding:2px}.filter-chip{display:inline-flex;align-items:center;gap:5px;max-width:100%;min-height:26px;padding:3px 7px;border:1px solid var(--gm-border);border-radius:5px;color:var(--gm-muted);background:var(--gm-panel-alt);font-size:.75rem;line-height:1.3;text-align:left;cursor:pointer}.filter-chip:disabled{opacity:.55;cursor:default}.filter-chip:hover{border-color:var(--gm-control-border);color:var(--gm-text)}.filter-chip.is-active{border-color:var(--gm-accent);color:var(--gm-accent);background:var(--gm-accent-soft)}.filter-chip span{overflow-wrap:anywhere}.filter-chip i{font-size:10px;flex-shrink:0}.filter-match-mode{display:block;font-size:.75rem;color:var(--gm-muted);margin:3px 2px 0}@media(pointer:coarse){.filter-chip{min-height:34px}.filter-icon{height:34px;width:34px}}
</style>
