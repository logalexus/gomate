<template>
 <section class="gm-traffic-filters" aria-label="Фильтры сессий">
  <div class="filter-heading"><button type="button" class="filter-title" aria-label="Настроить фильтры" :aria-expanded="String(expanded)" aria-controls="traffic-filter-fields" @click="expanded=!expanded"><i class="fas fa-sliders-h" aria-hidden="true"/> Фильтры <i :class="expanded?'fas fa-chevron-up':'fas fa-chevron-down'" aria-hidden="true"/></button><button v-if="activeCount" class="filter-reset" type="button" @click="clear">Сбросить все</button><span v-else class="filter-note">Все сессии</span></div>
  <div id="traffic-filter-fields" v-show="expanded">
  <div class="filter-field">
   <label for="traffic-service">Сервис</label>
   <div class="service-picker"><select id="traffic-service" class="custom-select custom-select-sm" :value="service" @change="setService($event.target.value)"><option value="">Все сервисы</option><option v-for="item in services" :key="item.port" :value="String(item.port)">{{ item.name }} · {{ item.port }}</option><option v-if="service&&!currentService" :value="service">Порт {{ service }}</option></select><button type="button" class="btn btn-sm btn-outline-secondary" aria-label="Добавить сервис" title="Добавить сервис" @click="$root.$emit('gm:add-service')"><i class="fas fa-plus" aria-hidden="true"/></button><button v-if="currentService" type="button" class="btn btn-sm btn-outline-secondary" aria-label="Настроить выбранный сервис" title="Настроить сервис" @click="$root.$emit('gm:edit-service',currentService)"><i class="fas fa-cog" aria-hidden="true"/></button></div>
  </div>
  <div class="filter-field"><label for="traffic-pattern">Совпадение с шаблоном</label><select id="traffic-pattern" class="custom-select custom-select-sm" :value="pattern" @change="setPattern($event.target.value)"><option value="">Любой шаблон или без совпадений</option><option v-for="item in patterns" :key="item.id" :value="String(item.id)">{{ item.name }}</option><option v-if="pattern&&!currentPattern" :value="pattern">Шаблон #{{ pattern }} · недоступен</option></select></div>
  <label class="favorite-filter"><input type="checkbox" :checked="favorites" @change="toggleFavorites"/><i class="far fa-star" aria-hidden="true"/> Только избранные</label>
  </div>
  <div v-if="activeCount" class="active-filters" aria-label="Активные фильтры">
   <button v-if="service" type="button" class="filter-chip" :aria-label="'Убрать фильтр сервиса '+(currentService?.name||service)" @click="setService('')"><span>{{ currentService?.name||'Порт '+service }}</span><i class="fas fa-times" aria-hidden="true"/></button>
   <button v-if="pattern" type="button" class="filter-chip" :aria-label="'Убрать фильтр шаблона '+(currentPattern?.name||pattern)" @click="setPattern('')"><span>{{ currentPattern?.name||'Шаблон #'+pattern }}</span><i class="fas fa-times" aria-hidden="true"/></button>
   <button v-if="favorites" type="button" class="filter-chip" aria-label="Убрать фильтр избранного" @click="toggleFavorites"><span>Избранное</span><i class="fas fa-times" aria-hidden="true"/></button>
  </div>
  <p v-if="activeCount>1" class="filter-note mb-0">Все выбранные условия должны совпасть.</p>
 </section>
</template>
<script>
import {serviceScope,filterLocation} from '../trafficFilters.mjs';
export default {
 data:()=>({expanded:true}),mounted(){if(window.innerHeight<700)this.expanded=false},
 computed:{
  service(){return serviceScope(this.$route.params.servicePort)},pattern(){return this.$route.query.pattern?String(this.$route.query.pattern):''},
  services(){return this.$store.state.services},patterns(){return this.$store.state.patterns.filter(p=>!p.deleted&&p.actionType==='FIND')},favorites(){return this.$store.state.displayFavoritesOnly},
  currentService(){return this.services.find(s=>String(s.port)===this.service)},currentPattern(){return this.patterns.find(p=>String(p.id)===this.pattern)},activeCount(){return Number(!!this.service)+Number(!!this.pattern)+Number(this.favorites)},
 },
 methods:{
  go(changes){const target=filterLocation(this.$route,changes);this.$router.push(target).catch(e=>{if(e.name!=='NavigationDuplicated')throw e})},
  setService(service){this.go({service})},setPattern(pattern){this.go({pattern})},
  toggleFavorites(){this.$store.commit('toggleDisplayFavoritesOnly');if(this.$route.params.streamId)this.go({})},
  clear(){if(this.favorites)this.$store.commit('toggleDisplayFavoritesOnly');this.go({service:'',pattern:''})},
 },
};
</script>
<style scoped>
.gm-traffic-filters{padding:16px;border-bottom:1px solid var(--gm-border);background:var(--gm-panel)}
.filter-heading,.service-picker{display:flex;align-items:center;gap:8px}.filter-heading{justify-content:space-between;margin-bottom:12px}.filter-title{border:0;background:none;color:var(--gm-text);padding:0;cursor:pointer;font-size:.8rem;font-weight:700;display:flex;gap:8px;align-items:center}.filter-reset{border:0;background:none;color:var(--gm-accent);font-size:.75rem;padding:4px;cursor:pointer}.filter-note{font-size:.75rem;color:var(--gm-muted)}.filter-field{margin-bottom:10px}.filter-field label{display:block;font-size:.75rem;font-weight:600;color:var(--gm-muted);margin-bottom:5px}.service-picker select{min-width:0;flex:1}.service-picker button{flex-shrink:0;min-width:32px}.favorite-filter{display:flex;gap:8px;align-items:center;font-size:.8rem;margin:12px 0 0;cursor:pointer}.favorite-filter input{accent-color:var(--gm-accent);width:15px;height:15px}.active-filters{display:flex;flex-wrap:wrap;gap:6px;margin-top:12px;margin-bottom:8px}.filter-chip{display:flex;align-items:center;gap:8px;max-width:100%;padding:5px 8px;border:1px solid var(--gm-accent);border-radius:6px;color:var(--gm-accent);background:var(--gm-accent-soft);font-size:.75rem;text-align:left;cursor:pointer}.filter-chip span{overflow-wrap:anywhere}.filter-chip i{font-size:.75rem;flex-shrink:0}
</style>
