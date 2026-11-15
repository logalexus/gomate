<template>
 <b-modal id="settingsModal" title="Настройки gomate" size="xl" scrollable @show="load" @shown="startPolling" @hidden="stopPolling" hide-footer>
  <div class="settings-intro"><span class="settings-eyebrow">РАБОЧЕЕ ОКРУЖЕНИЕ</span><p>Настройте источник трафика и срок хранения данных. Изменения сохраняются между перезапусками.</p></div>
  <b-alert :show="!!error" variant="danger" role="alert">{{ error }}</b-alert>
  <div v-if="loading" class="settings-empty" role="status">Загружаем конфигурацию хоста…</div>
  <template v-else-if="form">
   <b-tabs class="settings-tabs">
    <b-tab title="Захват трафика" active>
     <div class="settings-section">
      <div class="settings-status" :class="{'is-live':capture.running&&!capture.error}"><span class="settings-status-dot"/><div><strong>{{ capture.error || (capture.running ? 'Захват работает: ' + capture.interface : 'Захват остановлен') }}</strong><div class="small text-muted">{{ capture.running ? 'Локальные адреса: '+(capture.localIPs || []).join(', ') : 'Выберите интерфейс и включите LIVE-захват ниже.' }}</div></div></div>
      <section class="settings-card"><div class="settings-card-title"><div><h6>Источник трафика</h6><p>Один интерфейс хоста для LIVE-анализа.</p></div><b-form-checkbox v-model="form.captureEnabled" switch>Анализировать LIVE-трафик</b-form-checkbox></div>
       <b-form-group label="Сетевой интерфейс хоста" label-for="capture-interface" description="Для опубликованных Docker-портов обычно нужен внешний интерфейс хоста.">
        <div class="settings-control-row"><b-form-select id="capture-interface" v-model="form.interface" @change="interfaceChanged"><option value="">Выберите интерфейс</option><option v-for="item in interfaces" :key="item.name" :value="item.name" :disabled="!item.supported">{{ item.name }} · {{ item.addresses.join(', ') || 'без IP' }}{{ item.supported ? '' : ' — ' + item.reason }}</option></b-form-select><b-button variant="outline-secondary" @click="refreshInterfaces" aria-label="Обновить интерфейсы">Обновить</b-button></div>
       </b-form-group>
       <b-form-checkbox v-model="form.autoIPs">Использовать IP выбранного интерфейса</b-form-checkbox>
       <div v-if="form.autoIPs" class="settings-addresses mt-2">{{ selectedAddresses || 'Выберите интерфейс с локальным IP.' }}</div>
       <b-form-group v-else label="Локальные IP для определения направления" label-for="capture-ips" class="mt-3" description="Через запятую. При захвате на Docker bridge укажите адреса контейнеров."><b-form-textarea id="capture-ips" v-model="ipText" rows="2"/></b-form-group>
      </section>
      <div class="settings-note">При смене интерфейса текущие потоки завершаются и запускается новый захват. Возможен короткий перерыв.</div>
     </div>
    </b-tab>
    <b-tab title="Хранение и очистка">
     <div class="settings-section settings-storage">
      <section class="settings-card"><div class="settings-card-title"><div><h6>Автоматическое хранение</h6><p>Содержимое пакетов можно удалять раньше метаданных сессии.</p></div></div>
       <div class="row"><b-form-group class="col-md-6" label="Содержимое пакетов: хранить, минут" label-for="packet-retention" description="0 — без отдельного срока; содержимое удалится вместе с сессией."><b-form-input id="packet-retention" type="number" min="0" max="129600" v-model.number="packetMinutes"/><div class="settings-presets"><b-button v-for="n in [15,60,360,1440]" :key="n" size="sm" variant="outline-secondary" :aria-pressed="String(packetMinutes===n)" @click="packetMinutes=n">{{ duration(n) }}</b-button></div></b-form-group><b-form-group class="col-md-6" label="Сессии: хранить, минут" label-for="session-retention" description="От последнего трафика. 0 — автоматическое удаление отключено."><b-form-input id="session-retention" type="number" min="0" max="129600" v-model.number="sessionMinutes"/><div class="settings-presets"><b-button v-for="n in [60,360,1440,10080]" :key="n" size="sm" variant="outline-secondary" :aria-pressed="String(sessionMinutes===n)" @click="sessionMinutes=n">{{ duration(n) }}</b-button></div></b-form-group></div>
       <b-form-checkbox v-model="form.preserveFavorites" switch>Не удалять избранные сессии и их содержимое автоматически</b-form-checkbox>
       <b-form-group label="Интервал автоматической очистки, секунд" label-for="cleanup-interval" class="mt-3 mb-0"><b-form-input id="cleanup-interval" type="number" min="5" max="3600" v-model.number="form.cleanupIntervalSeconds" class="settings-short-input"/></b-form-group>
      </section>
      <section class="settings-card settings-danger"><div class="settings-card-title"><div><h6>Очистить сейчас</h6><p>Сначала оцените объём. Новый трафик после запуска задания сохраняется.</p></div><span class="settings-danger-label">Удаление данных</span></div>
       <div class="row"><b-form-group class="col-md-6" label="Что удалить" label-for="cleanup-mode"><b-form-select id="cleanup-mode" v-model="cleanupMode" :options="[{value:'packets',text:'Содержимое пакетов — оставить сессии'},{value:'sessions',text:'Сессии вместе с содержимым'}]"/></b-form-group><b-form-group class="col-md-6" label="Сервис" label-for="cleanup-service"><b-form-select id="cleanup-service" v-model="cleanupService" :options="serviceOptions"/></b-form-group></div>
       <p class="settings-note">{{ cleanupMode==='packets' ? 'Удалится только содержимое пакетов. Сессии, аномалии и фингерпринты останутся.' : 'Удалятся сессии, их содержимое и связанные срабатывания. Группы поведения будут пересчитаны по оставшимся сессиям.' }}</p>
       <b-form-checkbox v-model="cleanupKeep">Сохранить избранное при этой очистке</b-form-checkbox>
       <div class="settings-control-row mt-3"><b-button variant="outline-secondary" :disabled="busyPreview" @click="previewCleanup">{{ busyPreview ? 'Подсчёт…' : 'Оценить объём' }}</b-button><b-button variant="outline-danger" :disabled="!preview || activeCleanup" @click="confirmDelete=false; $bvModal.show('confirmCleanup')">Удалить…</b-button></div>
       <div v-if="preview" class="settings-preview mt-3" role="status"><span><strong>{{ preview.sessions }}</strong> {{ cleanupMode==='packets' ? 'сессий в области очистки · останутся' : 'сессий к удалению' }}</span><span><strong>{{ preview.packets }}</strong> блоков</span><span><strong>{{ bytes(preview.payloadBytes) }}</strong> содержимого</span></div>
       <div v-for="job in jobs.slice(0,3)" :key="job.id" class="settings-job"><span>Очистка #{{ job.id }} · {{ job.mode==='packets' ? 'содержимое пакетов' : 'сессии и содержимое' }} · {{ jobStatus(job.status) }}<small class="d-block text-muted">{{ job.mode==='packets' ? 'Удалено блоков' : 'Удалено записей (блоки + сессии)' }}: {{ job.deletedRecords }}</small><small v-if="job.mode==='packets'" class="d-block text-muted">Сессии, аномалии и фингерпринты сохранены.</small></span><b-button v-if="['queued','running'].includes(job.status)" size="sm" variant="outline-secondary" @click="cancelCleanup(job.id)">Остановить</b-button></div>
      </section>
     </div>
    </b-tab>
    <b-tab title="Приложение"><div class="settings-section"><section class="settings-card"><div class="settings-card-title"><div><h6>Ожидание ответов сервера</h6><p>Настройка для всех пользователей. Применяется после сохранения без перезапуска захвата.</p></div></div>
     <b-form-group label="Время ожидания ответа, секунд" label-for="request-timeout" description="От 5 до 300 секунд. По умолчанию 10; на медленной машине можно поставить 30–60. Браузер подождёт ещё 5 секунд на передачу ответа."><b-form-input id="request-timeout" type="number" min="5" max="300" step="1" v-model.number="form.requestTimeoutSeconds" class="settings-short-input"/></b-form-group>
     <p class="settings-note">Увеличение ожидания не ускоряет обработку. Чтобы снизить нагрузку, поставьте автообновление на паузу или выберите один сервис. Действует во всех разделах: трафик, аномалии, чат, настройки и Docker. Время выполнения скрипта задаётся отдельно в окне запуска.</p>
    </section></div></b-tab>
    <b-tab title="Уведомления"><NotificationSettings/></b-tab>
    <b-tab title="Отображение"><div class="settings-section"><section class="settings-card"><div class="settings-card-title"><div><h6>Плотность и формат данных</h6><p>Эти настройки применяются сразу и сохраняются только в вашем браузере.</p></div><span class="settings-local-label">Локально</span></div><div class="row"><b-form-group class="col-md-4" label="Сессий на странице" label-for="settings-pageSize"><b-form-input id="settings-pageSize" type="number" min="1" max="100" v-model.number="pageSize"/></b-form-group><b-form-group class="col-md-4" label="Байтов в строке HEX" label-for="settings-hexdumpBlockSize"><b-form-input id="settings-hexdumpBlockSize" type="number" min="1" max="64" v-model.number="hexdumpBlockSize"/></b-form-group><b-form-group class="col-md-4" label="Нумерация HEX-строк" label-for="settings-hex-base"><b-form-select id="settings-hex-base" v-model="hexdumpLineNumberBase" :options="[{value:10,text:'Десятичная'},{value:16,text:'Шестнадцатеричная'}]"/></b-form-group></div></section></div></b-tab>
   </b-tabs>
   <div class="settings-footer"><span class="small" :class="saved?'text-success':'text-muted'" role="status">{{ saved ? 'Настройки сохранены' : 'Серверные настройки изменятся после сохранения' }}</span><div class="settings-control-row"><b-button variant="outline-secondary" @click="$bvModal.hide('settingsModal')">Закрыть</b-button><b-button variant="primary" :disabled="saving" @click="save">{{ saving ? 'Применение…' : 'Сохранить и применить' }}</b-button></div></div>
  </template>
  <b-modal id="confirmCleanup" title="Подтверждение очистки" hide-footer>
   <p>Область: <strong>{{ serviceOptions.find(s=>s.value===Number(cleanupService))?.text || 'Все сервисы' }}</strong>.</p><p v-if="preview">Оценка: {{ preview.sessions }} сессий, {{ preview.packets }} блоков, {{ bytes(preview.payloadBytes) }} содержимого.</p><p>{{ cleanupMode==='packets' ? 'Будет удалено содержимое пакетов. Сессии, аномалии и фингерпринты останутся.' : 'Будут удалены сессии, их содержимое и связанные срабатывания в выбранной области.' }}</p><b-alert show variant="warning">{{ cleanupKeep ? 'Избранное будет сохранено.' : 'Избранное тоже будет удалено.' }} Действие нельзя отменить.</b-alert><b-form-checkbox v-model="confirmDelete">Подтверждаю удаление выбранных данных</b-form-checkbox><div class="settings-control-row justify-content-end mt-3"><b-button variant="outline-secondary" @click="$bvModal.hide('confirmCleanup')">Отмена</b-button><b-button variant="danger" :disabled="!confirmDelete || deleting" @click="startCleanup">{{ deleting ? 'Запуск…' : 'Удалить данные' }}</b-button></div>
  </b-modal>
 </b-modal>
</template>
<script>
import NotificationSettings from '../components/NotificationSettings.vue';
const message=e=>e.response?.data?.error || e.message || 'Не удалось выполнить запрос';
export default {
 components: {NotificationSettings},
 data:()=>({form:null,interfaces:[],capture:{},loading:false,saving:false,saved:false,error:'',ipText:'',packetMinutes:0,sessionMinutes:60,jobs:[],timer:null,polling:false,cleanupMode:'packets',cleanupService:0,cleanupKeep:true,preview:null,busyPreview:false,confirmDelete:false,deleting:false}),
 computed:{
  selectedAddresses(){return this.interfaces.find(x=>x.name===this.form?.interface)?.addresses.join(', ');},
  serviceOptions(){return [{value:0,text:'Все сервисы'},...this.$store.state.services.map(s=>({value:s.port,text:`${s.name} · ${s.port}`}))];},
  activeCleanup(){return this.jobs.some(j=>['queued','running'].includes(j.status));},
  pageSize:{get(){return this.$store.state.pageSize},set(v){this.$store.commit('setPageSize',Math.max(1,Math.min(100,Number(v)||25)))}},
  hexdumpBlockSize:{get(){return this.$store.state.hexdumpBlockSize},set(v){this.$store.commit('setHexdumpBlockSize',Math.max(1,Math.min(64,Number(v)||16)))}},
  hexdumpLineNumberBase:{get(){return this.$store.state.hexdumpLineNumberBase},set(v){this.$store.commit('setHexdumpLineNumberBase',v)}},
 },
 watch:{packetMinutes(){this.saved=false},sessionMinutes(){this.saved=false},ipText(){this.saved=false},cleanupMode(){this.preview=null},cleanupService(){this.preview=null},cleanupKeep(){this.preview=null},form:{deep:true,handler(){this.saved=false}}},
 beforeDestroy(){this.stopPolling()},
 methods:{
  async load(){this.loading=true;this.error='';this.saved=false;try{const [s,i,j]=await Promise.all([this.$http.get('/v2/settings'),this.$http.get('/v2/interfaces'),this.$http.get('/v2/cleanup')]);this.form={...s.data.settings,requestTimeoutSeconds:s.data.settings.requestTimeoutSeconds || s.data.settings.anomalyTimeoutSeconds || 10};this.capture=s.data.capture;this.interfaces=i.data;this.jobs=j.data;this.ipText=this.form.localIPs.join(', ');this.packetMinutes=this.form.packetRetentionSeconds/60;this.sessionMinutes=this.form.sessionRetentionSeconds/60;}catch(e){this.error=message(e)}finally{this.loading=false}},
  startPolling(){this.stopPolling();this.timer=setInterval(this.poll,2000)},stopPolling(){clearInterval(this.timer);this.timer=null},
  async poll(){if(this.polling)return;this.polling=true;try{const [s,j]=await Promise.all([this.$http.get('/v2/settings'),this.$http.get('/v2/cleanup')]);this.capture=s.data.capture;this.jobs=j.data;}catch(e){this.error=message(e)}finally{this.polling=false}},
  async refreshInterfaces(){try{this.interfaces=(await this.$http.get('/v2/interfaces')).data}catch(e){this.error=message(e)}},
  interfaceChanged(){this.form.autoIPs=true;this.ipText=''},
  async save(){if(!Number.isInteger(this.form.requestTimeoutSeconds)||this.form.requestTimeoutSeconds<5||this.form.requestTimeoutSeconds>300){this.error='Время ожидания ответа: целое число от 5 до 300 секунд.';return}this.error='';this.saving=true;try{const body={...this.form,localIPs:this.ipText.split(/[\s,]+/).filter(Boolean),packetRetentionSeconds:Number(this.packetMinutes)*60,sessionRetentionSeconds:Number(this.sessionMinutes)*60};const {data}=await this.$http.put('/v2/settings',body);this.form=data.settings;this.capture=data.capture;this.$nextTick(()=>this.saved=true);}catch(e){this.error=message(e)}finally{this.saving=false}},
  async previewCleanup(){this.busyPreview=true;this.error='';try{this.preview=(await this.$http.post('/v2/cleanup/preview',{preserveFavorites:this.cleanupKeep,service:this.cleanupService})).data;}catch(e){this.error=message(e)}finally{this.busyPreview=false}},
  async startCleanup(){this.deleting=true;try{await this.$http.post('/v2/cleanup',{mode:this.cleanupMode,preserveFavorites:this.cleanupKeep,service:this.cleanupService,confirm:'DELETE'});this.$bvModal.hide('confirmCleanup');this.preview=null;await this.poll();}catch(e){this.error=message(e)}finally{this.deleting=false}},
  async cancelCleanup(id){try{await this.$http.delete(`/v2/cleanup/${id}`);await this.poll()}catch(e){this.error=message(e)}},
  duration(n){return n>=1440?`${n/1440} д`:n>=60?`${n/60} ч`:`${n} мин`},bytes(n){return `${(n/1048576).toFixed(2)} MiB`},jobStatus(s){return {queued:'в очереди',running:'выполняется',completed:'завершена',cancelled:'остановлена'}[s]||s},
 }
}
</script>
<style scoped>
.settings-intro p,.settings-card-title p{color:var(--gm-muted);margin:4px 0 0;font-size:12px;line-height:1.5}.settings-eyebrow{font-size:11px;letter-spacing:.08em;color:var(--gm-accent);font-weight:700}
.settings-tabs{margin-top:10px}.settings-section{padding:8px 0 12px;display:grid;gap:10px}.settings-card{border:1px solid var(--gm-border);background:var(--gm-panel);border-radius:8px;padding:12px;min-width:0}.settings-card-title{display:flex;flex-wrap:wrap;gap:8px;justify-content:space-between;align-items:start;margin-bottom:10px}.settings-card-title h6{font-size:13px;font-weight:700;margin:0}.settings-card-title p{max-width:60ch}
.settings-status{display:flex;gap:8px;align-items:center;background:var(--gm-panel-alt);padding:8px 10px;border:1px solid var(--gm-border);border-radius:8px;font-size:13px}.settings-status>div{min-width:0;overflow-wrap:anywhere}.settings-status-dot{width:7px;height:7px;background:var(--gm-muted);border-radius:50%;flex-shrink:0}.settings-status.is-live .settings-status-dot{background:var(--gm-success)}
.settings-control-row,.settings-presets{display:flex;align-items:center;flex-wrap:wrap;gap:6px}.settings-control-row>.custom-select{flex:1;min-width:0}.settings-presets{margin-top:6px}.settings-addresses{font-family:var(--gm-mono);font-size:12px;color:var(--gm-accent);overflow-wrap:anywhere}.settings-note{font-size:12px;color:var(--gm-muted);border-left:2px solid var(--gm-border);padding-left:10px;margin-bottom:8px}.settings-short-input{max-width:160px}.settings-danger{border-left:3px solid var(--gm-danger)}.settings-danger-label{color:var(--gm-danger);font-size:12px;font-weight:600}.settings-local-label{color:var(--gm-accent);font-size:12px;background:var(--gm-accent-soft);padding:2px 6px;border-radius:4px}
.settings-preview{display:flex;flex-wrap:wrap;gap:8px 16px;padding:8px 10px;background:var(--gm-panel-alt);border-radius:6px;font-size:12px}.settings-preview strong{font-size:13px;margin-right:3px}.settings-job{display:flex;justify-content:space-between;gap:8px;align-items:center;border-top:1px solid var(--gm-border);padding-top:8px;margin-top:8px;font-size:12px}.settings-job>span{min-width:0;overflow-wrap:anywhere}.settings-footer{display:flex;flex-wrap:wrap;gap:8px;justify-content:space-between;align-items:center;padding-top:10px;border-top:1px solid var(--gm-border)}.settings-empty{padding:24px;text-align:center;color:var(--gm-muted)}
@media(min-width:1100px){.settings-storage{grid-template-columns:1fr 1fr;align-items:start}.settings-storage .row{margin-left:-6px;margin-right:-6px}.settings-storage .row>.form-group{padding-left:6px;padding-right:6px}}
@media(max-width:575px){.settings-card{padding:10px}.settings-footer>.settings-control-row{width:100%}.settings-footer .btn{flex:1}.settings-control-row>.custom-select{flex-basis:100%}.settings-section{gap:8px}}
</style>
