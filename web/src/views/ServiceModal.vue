<template>
 <b-modal id="serviceModal" size="lg" :title="creating ? 'Добавить сервис' : 'Настройки сервиса'" @show="open" hide-footer scrollable>
  <b-alert :show="!!error" variant="danger" role="alert">{{ error }}</b-alert>
  <div class="service-intro"><span class="service-eyebrow">{{ creating?'НОВЫЙ ИСТОЧНИК':'КОНФИГУРАЦИЯ СЕРВИСА' }}</span><p>{{ creating?'Выберите опубликованный порт контейнера или добавьте сервис вручную.':'Управляйте именем и разбором протоколов для этого порта.' }}</p></div>
  <b-tabs v-if="creating" v-model="tab" class="service-tabs">
   <b-tab title="Из Docker">
    <div class="service-discovery-toolbar"><div class="flex-grow-1"><label for="docker-service-search">Найти приложение</label><b-form-input id="docker-service-search" v-model="query" placeholder="Контейнер, образ или порт" aria-label="Поиск Docker-сервиса"/></div><b-button variant="outline-secondary" @click="discover" :disabled="discovering">{{ discovering?'Поиск…':'Обновить' }}</b-button></div>
    <b-alert :show="!!discoveryError" variant="warning">{{ discoveryError }} <b-button variant="link" @click="tab=1">Добавить вручную</b-button></b-alert>
    <div v-if="discovering" class="service-empty" role="status">Ищем приложения на Docker-хосте…</div>
    <div v-else-if="!filtered.length&&!discoveryError" class="service-empty"><h6>{{ candidates.length?'Ничего не найдено':'Нет доступных приложений' }}</h6><p>{{ candidates.length?'Измените поисковый запрос.':'Запустите контейнер с опубликованным портом или добавьте сервис вручную.' }}</p><b-button v-if="!candidates.length" variant="outline-primary" @click="tab=1">Добавить вручную</b-button></div>
    <div class="docker-list">
     <article v-for="c in filtered" :key="c.id" class="candidate" :class="{'candidate-tracked':!!existing(c.port)}">
      <div class="candidate-icon" aria-hidden="true"><i class="fas fa-cube"/></div><div class="candidate-info"><div class="candidate-name"><strong>{{ c.name }}</strong><span v-if="existing(c.port)" class="candidate-state">Отслеживается</span></div><div class="candidate-image">{{ c.image }}</div><div class="candidate-ports"><span>{{ c.protocol }}</span><code>{{ c.port ? c.port + ' → ' + c.privatePort : c.privatePort || 'порт не объявлен' }}</code><small>{{ c.hostIP }}</small></div><small v-if="!c.available" class="text-muted d-block mt-1">{{ c.reason }}</small><small v-if="existing(c.port)" class="text-muted d-block mt-1">{{ existing(c.port).name }}</small></div>
      <b-button variant="outline-primary" :disabled="!c.available||!!existing(c.port)||busy" @click="addDocker(c)" :aria-label="'Добавить '+c.name+' '+c.port">{{ existing(c.port)?'Добавлен':'+ Добавить' }}</b-button>
     </article>
    </div>
    <b-alert :show="!!added" variant="success" class="mt-3" role="status">{{ added }} добавлен. Можно добавить следующий сервис.</b-alert>
   </b-tab>
   <b-tab title="Вручную"/>
  </b-tabs>
  <b-form v-if="!creating||tab===1" @submit.prevent="save" class="service-form">
   <section class="service-card"><h6>Основные параметры</h6><div class="row"><b-form-group class="col-md-8" label="Название" label-for="service-name"><b-form-input id="service-name" v-model="service.name" required maxlength="256" placeholder="Например, магазин или API"/></b-form-group><b-form-group class="col-md-4" label="Порт в точке захвата" label-for="service-port"><b-form-input id="service-port" v-model.number="service.port" type="number" min="1" max="65535" required :disabled="!creating" placeholder="8080"/></b-form-group></div><p class="small text-muted mb-0">Укажите порт, видимый на выбранном интерфейсе захвата.</p></section>
   <section class="service-card"><h6>Разбор протоколов</h6><div class="service-toggle"><b-form-checkbox v-model="service.http" switch>Разбирать HTTP, HTTP/2 и gRPC</b-form-checkbox><small>Показывать запросы и ответы в удобном виде.</small></div><div class="service-toggle"><b-form-checkbox v-model="service.urldecodeHttpRequests" switch>Декодировать URL и form-urlencoded</b-form-checkbox><small>Раскрывать кодированные значения HTTP-параметров.</small></div><div class="service-toggle"><b-form-checkbox v-model="service.parseWebSockets" switch>Разбирать WebSocket</b-form-checkbox><small>Выделять сообщения после переключения протокола.</small></div><p class="small text-muted mt-3 mb-0">Исходные байты сохраняются отдельно. Для произвольного TCP отключите HTTP-разбор. TLS-содержимое остаётся зашифрованным.</p></section>
   <div class="service-form-footer"><b-button v-if="!creating" variant="outline-danger" :disabled="busy" @click="remove">Удалить сервис</b-button><span v-else/><div><b-button type="button" variant="outline-secondary" @click="$bvModal.hide('serviceModal')">Отмена</b-button><b-button type="submit" variant="primary" :disabled="busy">{{ busy?'Сохранение…':'Сохранить' }}</b-button></div></div>
  </b-form>
  <div v-if="creating&&tab===0" class="service-discovery-footer"><p>После добавления откройте настройки сервиса, чтобы включить HTTP/gRPC/WebSocket-разбор.</p><b-button variant="outline-secondary" @click="$bvModal.hide('serviceModal')">Готово</b-button></div>
 </b-modal>
</template>
<script>
import {filterLocation} from '../trafficFilters.mjs';
const message=e=>e.response?.data?.error||e.message;
export default {
 props:{creating:Boolean,initialService:Object},
 data:()=>({service:{},tab:0,candidates:[],query:'',discovering:false,discoveryError:'',busy:false,error:'',added:''}),
 computed:{filtered(){const q=this.query.toLowerCase();return this.candidates.filter(c=>`${c.name} ${c.image} ${c.port} ${c.privatePort}`.toLowerCase().includes(q))}},
 methods:{
  open(){this.error='';this.added='';this.service={name:'',port:null,http:false,urldecodeHttpRequests:false,parseWebSockets:false,...this.initialService};this.tab=0;if(this.creating)this.discover()},
  existing(port){return this.$store.state.services.find(s=>s.port===port)},
  async discover(){this.discovering=true;this.discoveryError='';try{this.candidates=(await this.$http.get('/v2/docker-services')).data}catch(e){this.discoveryError=message(e)}finally{this.discovering=false}},
  async refresh(){this.$store.commit('setServices',(await this.$http.get('/service/')).data);this.$emit('service-update-needed')},
  async addDocker(c){this.busy=true;this.error='';try{await this.$http.post('/v2/docker-services',{id:c.id});await this.refresh();this.added=`${c.name} · ${c.port}`;}catch(e){this.error=message(e)}finally{this.busy=false}},
  async save(){this.busy=true;this.error='';try{await this.$http.post(this.creating?'/service/':`/service/${this.service.port}`,this.service);await this.refresh();this.$bvModal.hide('serviceModal')}catch(e){this.error=message(e)}finally{this.busy=false}},
  async remove(){
   if(this.busy)return;
   const {port,name}=this.service;
   this.busy=true;this.error='';
   try{
    if(!await this.$bvModal.msgBoxConfirm(`«${name}» · ${port} перестанет отслеживаться. Сохранённая история останется.`,{title:'Удалить сервис?',okTitle:'Удалить',cancelTitle:'Отмена',okVariant:'danger'}))return;
    await this.$http.delete(`/service/${port}`);
    await this.refresh();
    this.$bvModal.hide('serviceModal');
    if(this.$route.name==='stream'&&String(this.$route.params.servicePort)===String(port))await this.$router.push(filterLocation(this.$route,{service:''}));
   }catch(e){this.error=message(e)}finally{this.busy=false}
  },
 }
}
</script>
<style scoped>
.service-eyebrow{font-size:.68rem;font-weight:700;letter-spacing:.14em;color:var(--gm-accent)}.service-intro p{font-size:.9rem;color:var(--gm-muted);margin:6px 0 20px}.service-discovery-toolbar{display:flex;gap:10px;align-items:end;margin:20px 0}.service-discovery-toolbar label{font-size:.8rem;font-weight:600}.docker-list{max-height:420px;overflow:auto;display:grid;gap:10px}.candidate{display:flex;align-items:center;gap:14px;border:1px solid var(--gm-border);border-radius:var(--gm-radius);padding:16px;background:var(--gm-panel)}.candidate-info{flex:1;min-width:0}.candidate-icon{width:38px;height:38px;display:grid;place-items:center;background:var(--gm-panel-alt);border:1px solid var(--gm-border);border-radius:9px;color:var(--gm-accent);flex-shrink:0}.candidate-name{display:flex;flex-wrap:wrap;gap:8px;align-items:center;overflow-wrap:anywhere}.candidate-state{font-size:.65rem;color:var(--gm-success);background:var(--gm-accent-soft);padding:2px 6px;border-radius:4px}.candidate-image{font-size:.75rem;color:var(--gm-muted);overflow-wrap:anywhere;margin-top:3px}.candidate-ports{display:flex;flex-wrap:wrap;gap:8px;align-items:center;margin-top:9px;font-size:.7rem}.candidate-ports>span{border:1px solid var(--gm-border);border-radius:4px;padding:1px 5px;text-transform:uppercase}.candidate-ports code{color:var(--gm-text);font-family:var(--gm-mono);font-size:.8rem}.candidate-ports small{color:var(--gm-muted)}.candidate>.btn{flex-shrink:0;font-size:.78rem}.service-empty{text-align:center;padding:40px 20px;color:var(--gm-muted);border:1px dashed var(--gm-border);border-radius:var(--gm-radius);margin-bottom:16px}.service-empty h6{color:var(--gm-text)}.service-empty p{font-size:.85rem}.service-form{margin-top:20px}.service-card{border:1px solid var(--gm-border);border-radius:var(--gm-radius);padding:20px;margin-bottom:16px}.service-card h6{font-weight:700;margin-bottom:20px}.service-toggle+.service-toggle{border-top:1px solid var(--gm-border);padding-top:12px;margin-top:12px}.service-toggle small{display:block;color:var(--gm-muted);padding-left:36px;margin-top:4px}.service-form-footer,.service-form-footer>div{display:flex;flex-wrap:wrap;align-items:center;gap:8px}.service-form-footer{justify-content:space-between;padding-top:16px;border-top:1px solid var(--gm-border)}.service-discovery-footer{display:flex;align-items:center;gap:18px;justify-content:space-between;border-top:1px solid var(--gm-border);padding-top:18px;margin-top:20px}.service-discovery-footer p{font-size:.8rem;color:var(--gm-muted);margin:0}@media(max-width:575px){.candidate{flex-wrap:wrap;padding:12px}.candidate>.btn{margin-left:52px}.service-card{padding:14px}.service-form-footer{gap:16px}.service-form-footer>div{width:100%}.service-form-footer>div .btn{flex:1}.service-discovery-toolbar{align-items:stretch;flex-direction:column}}
</style>
