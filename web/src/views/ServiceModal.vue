<template>
 <b-modal id="serviceModal" size="lg" :title="creating ? 'Добавить сервис' : 'Настройки сервиса'" @show="open" hide-footer scrollable>
  <b-alert :show="!!error" variant="danger" role="alert">{{ error }}</b-alert>
  <div class="service-intro"><span class="service-eyebrow">{{ creating?'НОВЫЙ ИСТОЧНИК':'КОНФИГУРАЦИЯ СЕРВИСА' }}</span><p>{{ creating?'Выберите источник, затем проверьте параметры и сохраните сервис.':'Имя и разбор протоколов для выбранного порта.' }}</p></div>
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
   </b-tab>
   <b-tab :title="dockerCandidate ? 'Параметры' : 'Вручную'"/>
  </b-tabs>
  <b-form v-if="!creating||tab===1" @submit.prevent="save" class="service-form">
   <div v-if="dockerCandidate" class="service-source"><div><span class="service-source-label"><i class="fas fa-cube" aria-hidden="true"/> Из Docker</span><strong>{{ dockerCandidate.name }}</strong><span>{{ dockerCandidate.protocol }} · {{ dockerCandidate.port }} → {{ dockerCandidate.privatePort }}</span></div><b-button type="button" size="sm" variant="link" :disabled="busy" @click="manualPort">Редактировать порт вручную</b-button></div>
   <section class="service-card"><h6>Основные параметры</h6><div class="row"><b-form-group class="col-md-8" label="Название" label-for="service-name"><b-form-input ref="serviceName" id="service-name" v-model="service.name" required maxlength="256" placeholder="Например, магазин или API"/></b-form-group><b-form-group class="col-md-4" label="Порт в точке захвата" label-for="service-port"><b-form-input id="service-port" v-model.number="service.port" type="number" min="1" max="65535" required :disabled="!creating||!!dockerCandidate" placeholder="8080"/></b-form-group></div><p class="small text-muted mb-0">{{ dockerCandidate ? 'Опубликованный порт контейнера. Имя и настройки ниже можно изменить.' : 'Укажите порт, видимый на выбранном интерфейсе захвата.' }}</p></section>
   <section class="service-card"><h6>Разбор протоколов</h6><div class="service-toggle"><b-form-checkbox v-model="service.http" switch>Разбирать HTTP, HTTP/2 и gRPC</b-form-checkbox><small>Показывать запросы и ответы в удобном виде.</small></div><div class="service-toggle"><b-form-checkbox v-model="service.urldecodeHttpRequests" switch>Декодировать URL и form-urlencoded</b-form-checkbox><small>Раскрывать кодированные значения HTTP-параметров.</small></div><div class="service-toggle"><b-form-checkbox v-model="service.parseWebSockets" switch>Разбирать WebSocket</b-form-checkbox><small>Выделять сообщения после переключения протокола.</small></div><p class="small text-muted mt-3 mb-0">Исходные байты сохраняются отдельно. Для произвольного TCP отключите HTTP-разбор. TLS-содержимое остаётся зашифрованным.</p></section>
   <div class="service-form-footer"><b-button v-if="!creating" variant="outline-danger" :disabled="busy" @click="remove">Удалить сервис</b-button><span v-else/><div><b-button type="button" variant="outline-secondary" @click="$bvModal.hide('serviceModal')">Отмена</b-button><b-button type="submit" variant="primary" :disabled="busy">{{ busy?'Сохранение…':'Сохранить' }}</b-button></div></div>
  </b-form>
  <div v-if="creating&&tab===0" class="service-discovery-footer"><p>«Добавить» открывает параметры. Сервис начнёт отслеживаться после сохранения.</p><b-button variant="outline-secondary" @click="$bvModal.hide('serviceModal')">Готово</b-button></div>
 </b-modal>
</template>
<script>
import {filterLocation} from '../trafficFilters.mjs';
const message=e=>e.response?.data?.error||e.message;
export default {
 props:{creating:Boolean,initialService:Object},
 data:()=>({service:{},tab:0,candidates:[],query:'',discovering:false,discoveryError:'',busy:false,error:'',dockerCandidate:null}),
 computed:{filtered(){const q=this.query.toLowerCase();return this.candidates.filter(c=>`${c.name} ${c.image} ${c.port} ${c.privatePort}`.toLowerCase().includes(q))}},
 methods:{
  open(){this.error='';this.dockerCandidate=null;this.query='';this.service={name:'',port:null,http:false,urldecodeHttpRequests:false,parseWebSockets:false,...this.initialService};this.tab=0;if(this.creating)this.discover()},
  existing(port){return this.$store.state.services.find(s=>s.port===port)},
  async discover(){this.discovering=true;this.discoveryError='';try{this.candidates=(await this.$http.get('/v2/docker-services')).data}catch(e){this.discoveryError=message(e)}finally{this.discovering=false}},
  async refresh(){this.$store.commit('setServices',(await this.$http.get('/service/')).data);this.$emit('service-update-needed')},
  async addDocker(c){
   if(this.busy||!c.available||this.existing(c.port))return;
   this.error='';this.dockerCandidate={...c};
   this.service={name:c.name,port:c.port,http:false,urldecodeHttpRequests:false,parseWebSockets:false};
   this.tab=1;await this.$nextTick();this.$refs.serviceName?.focus();
  },
  manualPort(){this.dockerCandidate=null;this.error=''},
  async save(){
   if(this.busy)return;
   this.busy=true;this.error='';
   try{
    if(this.creating&&this.dockerCandidate){await this.$http.post('/v2/docker-services',{id:this.dockerCandidate.id,service:this.service})}
    else{await this.$http.post(this.creating?'/service/':`/service/${this.service.port}`,this.service)}
    await this.refresh();this.$bvModal.hide('serviceModal');
   }catch(e){this.error=message(e)}finally{this.busy=false}
  },
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
.service-eyebrow{font-size:11px;font-weight:700;letter-spacing:.08em;color:var(--gm-accent)}
.service-intro p{font-size:12px;color:var(--gm-muted);margin:3px 0 10px}
.service-discovery-toolbar{display:flex;gap:8px;align-items:end;margin:10px 0}.service-discovery-toolbar label{font-size:12px;font-weight:600;margin-bottom:4px}
.docker-list{max-height:400px;overflow:auto;display:grid;gap:6px;padding:2px}
.candidate{display:flex;align-items:center;gap:8px;border:1px solid var(--gm-border);border-radius:7px;padding:8px 10px;background:var(--gm-panel)}
.candidate-info{flex:1;min-width:0}.candidate-icon{width:28px;height:28px;display:grid;place-items:center;background:var(--gm-panel-alt);border:1px solid var(--gm-border);border-radius:6px;color:var(--gm-accent);flex-shrink:0}
.candidate-name{display:flex;flex-wrap:wrap;gap:6px;align-items:center;overflow-wrap:anywhere;font-size:13px}.candidate-state{font-size:11px;color:var(--gm-success);background:var(--gm-success-soft);padding:1px 5px;border-radius:4px}
.candidate-image{font-size:12px;color:var(--gm-muted);overflow-wrap:anywhere;margin-top:1px}.candidate-ports{display:flex;flex-wrap:wrap;gap:6px;align-items:center;margin-top:3px;font-size:11px}.candidate-ports>span{border:1px solid var(--gm-border);border-radius:4px;padding:0 4px;text-transform:uppercase}.candidate-ports code{color:var(--gm-text);font-family:var(--gm-mono);font-size:12px}.candidate-ports small{color:var(--gm-muted)}.candidate>.btn{flex-shrink:0;font-size:12px}
.service-empty{text-align:center;padding:20px 12px;color:var(--gm-muted);border:1px dashed var(--gm-border);border-radius:7px;margin-bottom:10px}.service-empty h6{color:var(--gm-text)}.service-empty p{font-size:12px}
.service-form{margin-top:10px}.service-card{border:1px solid var(--gm-border);border-radius:7px;padding:12px;margin-bottom:10px}.service-card h6{font-size:13px;font-weight:700;margin-bottom:10px}.service-card .row{margin-left:-5px;margin-right:-5px}.service-card .form-group{padding-left:5px;padding-right:5px;margin-bottom:8px}
.service-toggle+.service-toggle{border-top:1px solid var(--gm-border);padding-top:7px;margin-top:7px}.service-toggle small{display:block;color:var(--gm-muted);padding-left:36px;margin-top:2px}.service-card>p.mt-3{margin-top:10px!important}
.service-source{display:flex;align-items:center;justify-content:space-between;gap:8px;padding:8px 10px;border:1px solid var(--gm-border);border-radius:7px;background:var(--gm-panel-alt);margin-bottom:10px;font-size:12px}.service-source>div{display:flex;flex-wrap:wrap;gap:4px 8px;min-width:0;overflow-wrap:anywhere}.service-source-label{color:var(--gm-accent)}.service-source .btn{flex-shrink:0;white-space:normal;font-size:12px}
.service-form-footer,.service-form-footer>div{display:flex;flex-wrap:wrap;align-items:center;gap:6px}.service-form-footer{justify-content:space-between;padding-top:10px;border-top:1px solid var(--gm-border)}
.service-discovery-footer{display:flex;align-items:center;gap:10px;justify-content:space-between;border-top:1px solid var(--gm-border);padding-top:10px;margin-top:10px}.service-discovery-footer p{font-size:12px;color:var(--gm-muted);margin:0}
@media(max-width:575px){.candidate{flex-wrap:wrap;padding:8px}.candidate>.btn{margin-left:36px}.service-card{padding:10px}.service-form-footer{gap:10px}.service-form-footer>div{width:100%}.service-form-footer>div .btn{flex:1}.service-source{align-items:start;flex-direction:column}.service-discovery-toolbar{gap:6px}.service-discovery-toolbar>div{min-width:0}.service-source .btn{padding-left:0}}
</style>
