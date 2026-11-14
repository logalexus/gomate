<template>
 <section class="gm-traffic-filters" aria-label="Фильтры сессий">
  <div class="service-picker">
   <label class="sr-only" for="traffic-service">Сервис</label><select id="traffic-service" class="custom-select custom-select-sm" :value="service" @change="setService($event.target.value)"><option value="">Все сервисы</option><option v-for="item in services" :key="item.port" :value="String(item.port)">{{ item.name }} · {{ item.port }}</option><option v-if="service&&!currentService" :value="service">Порт {{ service }}</option></select>
   <button type="button" class="filter-icon" aria-label="Добавить сервис" title="Добавить сервис" @click="$root.$emit('gm:add-service')"><i class="fas fa-plus" aria-hidden="true"/></button><button v-if="currentService" type="button" class="filter-icon" aria-label="Настроить выбранный сервис" title="Настроить сервис" @click="$root.$emit('gm:edit-service',currentService)"><i class="fas fa-cog" aria-hidden="true"/></button>
   <button v-if="activeCount" type="button" class="filter-icon" aria-label="Сбросить все фильтры" title="Сбросить все фильтры" @click="clear"><i class="fas fa-times" aria-hidden="true"/></button>
  </div>
  <div class="filter-labels" role="group" aria-label="Метки: должны совпасть все выбранные">
   <button type="button" class="filter-chip" :class="{'is-active':favorites}" :aria-pressed="String(favorites)" @click="toggleFavorites"><i :class="favorites?'fas fa-star':'far fa-star'" aria-hidden="true"/> Избранное</button>
   <button v-for="item in availablePatterns" :key="item.id" type="button" class="filter-chip" :class="{'is-active':selected.includes(item.id)}" :aria-pressed="String(selected.includes(item.id))" :disabled="selected.length>=100&&!selected.includes(item.id)" :title="item.name+(item.missing?' · недоступен':'')" @click="togglePattern(item.id)"><i :class="selected.includes(item.id)?'fas fa-check':'fas fa-hashtag'" aria-hidden="true"/><span>{{ item.name }}</span></button>
  </div>
  <span v-if="selected.length>1" class="filter-match-mode">Совпадают все {{ selected.length }} метки</span>
 </section>
</template>
<script>
import {serviceScope,filterLocation,patternSelection} from '../trafficFilters.mjs';
export default {
 computed:{
  service(){return serviceScope(this.$route.params.servicePort)},selected(){return patternSelection(this.$route.query)},
  services(){return this.$store.state.services},patterns(){return this.$store.state.patterns.filter(p=>!p.deleted&&p.actionType==='FIND')},favorites(){return this.$store.state.displayFavoritesOnly},
  availablePatterns(){return [...this.patterns,...this.selected.filter(id=>!this.patterns.some(p=>p.id===id)).map(id=>({id,name:'Шаблон #'+id,missing:true}))]},
  currentService(){return this.services.find(s=>String(s.port)===this.service)},activeCount(){return Number(!!this.service)+this.selected.length+Number(this.favorites)},
 },
 methods:{
  go(changes){const target=filterLocation(this.$route,changes);this.$router.push(target).catch(e=>{if(e.name!=='NavigationDuplicated')throw e})},
  setService(service){this.go({service})},togglePattern(id){this.go({patterns:this.selected.includes(id)?this.selected.filter(p=>p!==id):[...this.selected,id]})},
  toggleFavorites(){this.$store.commit('toggleDisplayFavoritesOnly');if(this.$route.params.streamId)this.go({})},
  clear(){if(this.favorites)this.$store.commit('toggleDisplayFavoritesOnly');this.go({service:'',patterns:[]})},
 },
};
</script>
<style scoped>
.gm-traffic-filters{padding:8px 10px;border-bottom:1px solid var(--gm-border);background:var(--gm-panel);flex-shrink:0}.service-picker{display:flex;align-items:center;gap:4px}.service-picker select{min-width:0;flex:1;height:30px;font-size:.75rem}.filter-icon{border:1px solid var(--gm-border);border-radius:5px;background:transparent;color:var(--gm-muted);width:30px;height:30px;flex-shrink:0;cursor:pointer;font-size:.75rem}.filter-icon:hover{color:var(--gm-text);background:var(--gm-panel-alt)}.filter-labels{display:flex;flex-wrap:wrap;align-content:start;gap:4px;margin-top:6px;max-height:130px;overflow:auto;padding:2px}.filter-chip{display:inline-flex;align-items:center;gap:5px;max-width:100%;min-height:26px;padding:3px 7px;border:1px solid var(--gm-border);border-radius:5px;color:var(--gm-muted);background:var(--gm-panel-alt);font-size:.75rem;line-height:1.3;text-align:left;cursor:pointer}.filter-chip:disabled{opacity:.55;cursor:default}.filter-chip:hover{border-color:var(--gm-control-border);color:var(--gm-text)}.filter-chip.is-active{border-color:var(--gm-accent);color:var(--gm-accent);background:var(--gm-accent-soft)}.filter-chip span{overflow-wrap:anywhere}.filter-chip i{font-size:10px;flex-shrink:0}.filter-match-mode{display:block;font-size:.75rem;color:var(--gm-muted);margin:3px 2px 0}@media(pointer:coarse){.filter-chip{min-height:34px}.filter-icon{height:34px;width:34px}}
</style>
