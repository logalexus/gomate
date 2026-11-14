<template>
 <section class="gm-traffic-detail">
  <header class="traffic-detail-heading"><h1>{{ currentId?'Сессия #'+currentId:'Трафик' }}</h1><span v-if="currentId" class="traffic-block-count">{{ packets.length }} блоков</span></header>
  <template v-if="currentId">
   <section class="packet-view-toolbar" aria-label="Представление пакетов">
    <div class="packet-view-field"><select id="packet-representation" aria-label="Содержимое" class="custom-select custom-select-sm" v-model="representation" @change="reset"><option value="">Все блоки</option><option value="raw">Исходные байты</option><option value="decoded">Разобранные сообщения</option></select></div>
    <div class="packet-view-field"><select id="packet-direction" aria-label="Направление на странице" class="custom-select custom-select-sm" v-model="direction"><option value="">Запросы и ответы</option><option value="incoming">Только запросы</option><option value="outgoing">Только ответы</option></select></div>
    <div class="packet-view-field"><div class="packet-display" role="group" aria-label="Отображение содержимого"><button type="button" :class="{active:!$store.state.hexdumpMode}" :aria-pressed="String(!$store.state.hexdumpMode)" @click="setHex(false)">Текст</button><button type="button" :class="{active:$store.state.hexdumpMode}" :aria-pressed="String($store.state.hexdumpMode)" @click="setHex(true)">HEX</button></div></div>
   </section>
   <div v-if="representation||direction" class="packet-active-filters" aria-label="Активные фильтры блоков"><span>Показаны:</span><button v-if="representation" @click="representation='';reset()" :aria-label="'Убрать фильтр содержимого'">{{ representation==='raw'?'Исходные байты':'Разобранные сообщения' }} <i class="fas fa-times" aria-hidden="true"/></button><button v-if="direction" @click="direction=''" aria-label="Убрать фильтр направления">{{ direction==='incoming'?'Запросы':'Ответы' }} <i class="fas fa-times" aria-hidden="true"/></button><button class="packet-reset-view" @click="clearView">Сбросить</button></div>

   <div v-if="error" class="packet-error" role="alert">{{ error }} <button class="btn btn-sm btn-outline-secondary" @click="load">Повторить</button></div>
   <div v-if="loading&&!packets.length" class="packet-empty" role="status"><i class="fas fa-circle-notch fa-spin" aria-hidden="true"/><h2>Загружаем содержимое</h2><p>Получаем выбранную страницу сессии.</p></div>
   <div v-else-if="!packetsWithOffsets.length" class="packet-empty"><i class="fas fa-filter" aria-hidden="true"/><h2>{{ packets.length?'Нет блоков этого направления':'Нет доступных блоков' }}</h2><p>{{ packets.length?'Измените направление или перейдите на следующую страницу.':'Попробуйте другое представление. Содержимое также могло быть удалено по сроку хранения.' }}</p><button v-if="representation||direction" class="btn btn-sm btn-outline-primary" @click="clearView">Показать все блоки</button></div>
   <div class="packet-list"><Packet v-for="packetWithOffset in packetsWithOffsets" :key="packetWithOffset.packet.id" :packet="packetWithOffset.packet" :offset="packetWithOffset.offset" @decode="openDecoder" @export="openWorkbench"/></div>
   <footer class="packet-pagination"><button class="btn btn-sm btn-outline-secondary" :disabled="!history.length||loading" aria-label="Предыдущие блоки" @click="previousPage"><i class="fas fa-arrow-left" aria-hidden="true"/> Назад</button><span>Страница {{ history.length+1 }}</span><button class="btn btn-sm btn-outline-secondary" :disabled="!hasMore||loading" aria-label="Следующие блоки" @click="nextPage">Далее <i class="fas fa-arrow-right" aria-hidden="true"/></button></footer>
  </template>
  <div v-else class="traffic-welcome"><i class="fas fa-stream" aria-hidden="true"/><h2>Выберите сессию</h2><p>Откройте сессию в списке, чтобы увидеть её содержимое.</p></div>
  <TrafficDecoder ref="decoder"/><RequestWorkbench ref="workbench"/>
 </section>
</template>
<script>
import RequestWorkbench from '../components/RequestWorkbench.vue';
import Packet from '../components/Packet';
import TrafficDecoder from '@/components/TrafficDecoder.vue';
export default {
 name:'ContentStream',components:{Packet,TrafficDecoder,RequestWorkbench},props:['servicePort','streamId'],
 data:()=>({packets:[],cursor:null,history:[],loading:false,hasMore:false,generation:0,error:'',representation:'',direction:''}),
 computed:{currentId(){return this.$route.params.streamId},packetsWithOffsets(){return this.packets.map((packet,i)=>({packet,offset:i?packet.timestamp-this.packets[i-1].timestamp:null})).filter(({packet})=>!this.direction||packet.incoming===(this.direction==='incoming'))}},
 mounted(){this.reset()},beforeDestroy(){this.generation++},watch:{'$route.params.streamId':'reset'},
 methods:{
  openWorkbench(value){this.$refs.workbench.open({...value,streamId:this.currentId})},
  openDecoder(value){this.$refs.decoder.open(value)},setHex(value){if(this.$store.state.hexdumpMode!==value)this.$store.commit('toggleHexdumpMode')},
  clearView(){this.representation='';this.direction='';this.reset()},
  reset(){this.packets=[];this.cursor=null;this.history=[];this.error='';this.load()},
  async load(){const generation=++this.generation;if(!this.currentId){this.loading=false;return}this.loading=true;const pageSize=Math.max(1,Math.min(50,Number(this.$store.state.pageSize)||25));try{const {data,headers}=await this.$http.post(`/packet/${this.currentId}`,{startingFrom:this.cursor,pageSize,representation:this.representation});if(generation!==this.generation)return;this.packets=data;this.hasMore=headers['x-has-more']==='true';this.error=''}catch(e){if(generation===this.generation)this.error='Не удалось загрузить содержимое сессии.'}finally{if(generation===this.generation)this.loading=false}},
  refreshLive(){if(!this.loading&&!this.hasMore&&!this.$store.state.pause)return this.load()},
  nextPage(){if(!this.packets.length)return;this.history.push(this.cursor);if(this.history.length>1000)this.history.shift();this.cursor=this.packets[this.packets.length-1].id;this.load()},
  previousPage(){this.cursor=this.history.pop()??null;this.load()},
 },
};
</script>
<style scoped>
.gm-traffic-detail{min-width:0}.traffic-detail-heading{display:flex;justify-content:space-between;gap:12px;align-items:center;padding:0 0 10px}.traffic-detail-heading h1{font-size:1.05rem;font-weight:650;letter-spacing:-.02em;margin:0}.traffic-block-count{font-size:.75rem;color:var(--gm-muted);white-space:nowrap}.packet-view-toolbar{display:flex;gap:8px;align-items:center;flex-wrap:wrap;background:var(--gm-panel);border:1px solid var(--gm-border);border-radius:var(--gm-radius);padding:7px 8px;margin-bottom:10px}.packet-view-field{flex:1;min-width:150px}.packet-view-field:last-child{flex:0 0 auto;min-width:100px}.packet-view-field .custom-select{font-size:.75rem;min-height:30px;height:30px;padding-top:3px;padding-bottom:3px}.packet-display{display:flex;border:1px solid var(--gm-border);border-radius:6px;overflow:hidden;padding:2px;background:var(--gm-bg)}.packet-display button{border:0;border-radius:4px;min-height:24px;padding:2px 10px;background:transparent;color:var(--gm-muted);font-size:.75rem;cursor:pointer}.packet-display button.active{background:var(--gm-accent-soft);color:var(--gm-accent);font-weight:600}.packet-active-filters{display:flex;align-items:center;flex-wrap:wrap;gap:6px;margin:0 0 10px;font-size:.75rem;color:var(--gm-muted)}.packet-active-filters button{border:1px solid var(--gm-accent);background:var(--gm-accent-soft);color:var(--gm-accent);padding:2px 7px;min-height:26px;border-radius:5px;font-size:.75rem;cursor:pointer}.packet-active-filters button i{margin-left:4px;font-size:.7rem}.packet-active-filters .packet-reset-view{border:0;background:none}.packet-list{display:grid;gap:8px}.packet-pagination{display:flex;align-items:center;justify-content:flex-end;gap:10px;padding:10px 0 0;flex-wrap:wrap}.packet-pagination>span{font-size:.75rem;color:var(--gm-muted)}.packet-pagination .btn{min-height:28px;padding:3px 9px}.packet-error{color:var(--gm-danger);padding:10px}.packet-empty{padding:22px 14px;text-align:center;border:1px dashed var(--gm-border);border-radius:var(--gm-radius);color:var(--gm-muted)}.packet-empty>i{font-size:1.2rem;margin-bottom:10px}.packet-empty h2{font-size:1rem;color:var(--gm-text)}.packet-empty p{font-size:.8rem}.traffic-welcome{border:1px solid var(--gm-border);border-radius:var(--gm-radius);background:var(--gm-panel);display:flex;flex-direction:column;align-items:center;justify-content:center;padding:35px 20px;text-align:center}.traffic-welcome>i{color:var(--gm-accent);font-size:1.3rem;margin-bottom:14px}.traffic-welcome h2{font-size:1rem;font-weight:600}.traffic-welcome>p{font-size:.8rem;line-height:1.5;color:var(--gm-muted);margin:0}@media(max-width:575px){.packet-view-toolbar{gap:6px}.packet-view-field{min-width:130px}.packet-view-field:last-child{min-width:100px}.traffic-detail-heading{padding-bottom:8px}.packet-pagination{justify-content:space-between}}
</style>
