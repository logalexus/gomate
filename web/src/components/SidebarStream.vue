<template>
 <article :id="`stream-${stream.id}`" class="stream-item" :class="{'is-selected':selected,'is-favorite':favorite0}">
  <router-link class="stream-link" :to="location" :aria-current="selected?'true':null">
   <div class="stream-topline"><span class="stream-id">#{{ stream.id }}</span><span class="stream-protocol">{{ stream.protocol }}</span><time :datetime="new Date(stream.startTimestamp).toISOString()">{{ dateToText(stream.startTimestamp) }}</time></div>
   <div class="stream-service">{{ serviceName }} <span>:{{ stream.service }}</span></div>
   <div v-if="stream.firstRequestPath" class="stream-path" :title="stream.firstRequestPath+(stream.firstRequestTruncated?'… (путь сокращён)':'')">{{ stream.firstRequestPath }}{{ stream.firstRequestTruncated?'…':'' }}</div>
   <div class="stream-stats"><span>{{ sizeText }} · {{ stream.packetsCount }} блоков</span><span v-if="stream.incomplete" class="stream-warning">Неполная</span><span v-else-if="stream.payloadPurged" class="stream-muted">Очищено</span></div>
   <div v-if="notDeletedFoundPatterns.length" class="stream-matches"><span v-for="pattern in notDeletedFoundPatterns" :key="pattern.id"><i :style="patternColor(pattern)"/>{{ pattern.name }}</span></div>
   <div v-if="stream.userAgentHash" class="stream-fingerprint" :title="stream.userAgentHash">UA {{ stream.userAgentHash }}</div>
  </router-link>
  <button type="button" class="stream-favorite" :class="{'is-active':favorite0}" :disabled="saving" :aria-label="favorite0?'Убрать сессию '+stream.id+' из избранного':'Добавить сессию '+stream.id+' в избранное'" :aria-pressed="String(favorite0)" @click="toggleFavorite"><i :class="favorite0?'fas fa-star':'far fa-star'" aria-hidden="true"/></button>
 </article>
</template>
<script>
import {streamLocation} from '../trafficFilters.mjs';
export default {
 name:'SidebarStream',props:{stream:{type:Object,required:true}},
 data(){return {favorite0:this.stream.favorite,saving:false}},
 computed:{
  selected(){return Number(this.$route.params.streamId)===this.stream.id},location(){return streamLocation(this.$route,this.stream)},
  serviceName(){return this.$store.state.services.find(s=>s.port===this.stream.service)?.name||'Сервис'},
  sizeText(){const size=Number(this.stream.sizeBytes)||0;return size<1024?size+' Б':size<1048576?(size/1024).toFixed(1)+' KiB':(size/1048576).toFixed(1)+' MiB'},
  notDeletedFoundPatterns(){const ids=this.stream.foundPatternsIds||[];return this.$store.state.patterns.filter(p=>!p.deleted&&ids.includes(p.id))},
 },
 watch:{'stream.favorite'(value){this.favorite0=value}},
 methods:{
  dateToText(timestamp){const date=new Date(timestamp),today=new Date();return date.toLocaleString('ru-RU',{...(date.toDateString()===today.toDateString()?{}:{day:'2-digit',month:'2-digit'}),hour:'2-digit',minute:'2-digit',second:'2-digit'})},
  patternColor(pattern){return /^#[0-9a-f]{6}$/i.test(pattern.color)?{backgroundColor:pattern.color}:{}},
  async toggleFavorite(){if(this.saving)return;this.saving=true;try{await this.$http.post(`stream/${this.stream.id}/${this.favorite0?'unfavorite':'favorite'}`);this.favorite0=!this.favorite0;this.$emit('favorite-changed')}catch(e){this.$bvToast.toast('Не удалось изменить избранное. Повторите попытку.',{title:'Избранное',variant:'danger'})}finally{this.saving=false}},
 },
};
</script>
<style scoped>
.stream-item{position:relative;list-style:none;border-bottom:1px solid var(--gm-border);border-left:3px solid transparent;background:var(--gm-panel);transition:background-color .15s}.stream-item:hover{background:var(--gm-panel-alt)}.stream-item.is-selected{border-left-color:var(--gm-accent);background:var(--gm-accent-soft)}.stream-link{display:block;padding:8px 34px 8px 10px;color:var(--gm-text);text-decoration:none!important;min-height:76px}.stream-topline{display:flex;align-items:center;gap:7px;font-size:.75rem;margin-bottom:3px}.stream-id{font-family:var(--gm-mono);color:var(--gm-muted)}.stream-protocol{font-family:var(--gm-mono);font-size:.75rem;border:1px solid var(--gm-border);padding:1px 5px;border-radius:4px}.stream-topline time{margin-left:auto;color:var(--gm-muted);font-family:var(--gm-mono);white-space:nowrap;font-size:.75rem}.stream-service{font-size:.83rem;font-weight:600;overflow-wrap:anywhere}.stream-service>span{font-weight:400;color:var(--gm-muted);font-size:.75rem}.stream-stats{display:flex;flex-wrap:wrap;justify-content:space-between;gap:4px;margin-top:3px;font-size:.75rem;color:var(--gm-muted)}.stream-warning{color:var(--gm-warning)}.stream-matches{display:flex;flex-wrap:wrap;gap:5px;margin-top:4px}.stream-matches>span{font-size:.75rem;display:flex;align-items:center;gap:5px;border:1px solid var(--gm-border);padding:2px 5px;border-radius:4px;max-width:100%;overflow-wrap:anywhere}.stream-matches i{display:inline-block;width:5px;height:5px;border-radius:50%;background:var(--gm-accent);flex-shrink:0}.stream-fingerprint{color:var(--gm-muted);font-family:var(--gm-mono);font-size:.75rem;margin-top:3px;text-overflow:ellipsis;overflow:hidden;white-space:nowrap}.stream-favorite{position:absolute;right:7px;top:6px;border:0;background:transparent;color:var(--gm-muted);padding:6px;cursor:pointer;font-size:.75rem}.stream-favorite.is-active{color:var(--gm-warning)}
.stream-path{font-family:var(--gm-mono);font-size:.78rem;line-height:1.45;margin-top:3px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--gm-text)}
</style>
