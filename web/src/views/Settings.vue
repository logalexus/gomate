<template>
 <b-modal id="settingsModal" title="gomate settings" size="xl" scrollable @show="load" @shown="startPolling" @hidden="stopPolling" hide-footer>
  <div class="settings-intro"><span class="settings-eyebrow">WORKSPACE</span><p>Configure the traffic source and retention policy. Changes persist across restarts.</p></div>
  <b-alert :show="!!error" variant="danger" role="alert">{{ error }}</b-alert>
  <div v-if="loading" class="settings-empty" role="status">Loading host configuration…</div>
  <template v-else-if="form">
   <b-tabs class="settings-tabs" v-model="activeTab">
    <b-tab title="Traffic capture">
     <div class="settings-section">
      <div class="settings-status" :class="{'is-live':capture.running&&!capture.error}"><span class="settings-status-dot"/><div><strong>{{ capture.error || (capture.running ? 'Capture running: ' + capture.interface : 'Capture stopped') }}</strong><div class="small text-muted">{{ capture.running ? 'Local addresses: '+(capture.localIPs || []).join(', ') : 'Select an interface and enable LIVE capture below.' }}</div></div></div>
      <section class="settings-card"><div class="settings-card-title"><div><h6>Traffic source</h6><p>One host interface for LIVE analysis.</p></div><b-form-checkbox v-model="form.captureEnabled" switch>Analyze LIVE traffic</b-form-checkbox></div>
       <b-form-group label="Host network interface" label-for="capture-interface" description="Published Docker ports usually require the external host interface.">
        <div class="settings-control-row"><b-form-select id="capture-interface" v-model="form.interface" @change="interfaceChanged"><option value="">Select an interface</option><option v-for="item in interfaces" :key="item.name" :value="item.name" :disabled="!item.supported">{{ item.name }} · {{ item.addresses.join(', ') || 'no IP' }}{{ item.supported ? '' : ' — ' + item.reason }}</option></b-form-select><b-button variant="outline-secondary" @click="refreshInterfaces" aria-label="Refresh interfaces">Refresh</b-button></div>
       </b-form-group>
       <b-form-checkbox v-model="form.autoIPs">Use IPs from selected interface</b-form-checkbox>
       <div v-if="form.autoIPs" class="settings-addresses mt-2">{{ selectedAddresses || 'Select an interface with a local IP.' }}</div>
       <b-form-group v-else label="Local IPs for direction detection" label-for="capture-ips" class="mt-3" description="Comma-separated. For capture on a Docker bridge, enter the container addresses."><b-form-textarea id="capture-ips" v-model="ipText" rows="2"/></b-form-group>
      </section>
      <div class="settings-note">Changing the interface ends current streams and starts a new capture. A brief interruption is possible.</div>
     </div>
    </b-tab>
    <b-tab title="Retention and cleanup">
     <div class="settings-section settings-storage">
      <section class="settings-card"><div class="settings-card-title"><div><h6>Automatic retention</h6><p>Packet content can expire before session metadata.</p></div></div>
       <div class="row"><b-form-group class="col-md-6" label="Packet content retention, minutes" label-for="packet-retention" description="0: no separate expiry; content is deleted with its session."><b-form-input id="packet-retention" type="number" min="0" max="129600" v-model.number="packetMinutes"/><div class="settings-presets"><b-button v-for="n in [15,60,360,1440]" :key="n" size="sm" variant="outline-secondary" :aria-pressed="String(packetMinutes===n)" @click="packetMinutes=n">{{ duration(n) }}</b-button></div></b-form-group><b-form-group class="col-md-6" label="Session retention, minutes" label-for="session-retention" description="Since the last traffic. 0 disables automatic deletion."><b-form-input id="session-retention" type="number" min="0" max="129600" v-model.number="sessionMinutes"/><div class="settings-presets"><b-button v-for="n in [60,360,1440,10080]" :key="n" size="sm" variant="outline-secondary" :aria-pressed="String(sessionMinutes===n)" @click="sessionMinutes=n">{{ duration(n) }}</b-button></div></b-form-group></div>
       <b-form-checkbox v-model="form.preserveFavorites" switch>Keep favorite sessions and their content during automatic cleanup</b-form-checkbox>
       <b-form-group label="Automatic cleanup interval, seconds" label-for="cleanup-interval" class="mt-3 mb-0"><b-form-input id="cleanup-interval" type="number" min="5" max="3600" v-model.number="form.cleanupIntervalSeconds" class="settings-short-input"/></b-form-group>
      </section>
      <section class="settings-card settings-danger"><div class="settings-card-title"><div><h6>Clean up now</h6><p>Estimate the size first. New traffic after the job starts is retained.</p></div><span class="settings-danger-label">Data deletion</span></div>
       <div class="row"><b-form-group class="col-md-6" label="Delete" label-for="cleanup-mode"><b-form-select id="cleanup-mode" v-model="cleanupMode" :options="[{value:'packets',text:'Packet content — keep sessions'},{value:'sessions',text:'Sessions and their content'}]"/></b-form-group><b-form-group class="col-md-6" label="Service" label-for="cleanup-service"><b-form-select id="cleanup-service" v-model="cleanupService" :options="serviceOptions"/></b-form-group></div>
       <p class="settings-note">{{ cleanupMode==='packets' ? 'Only packet content will be deleted. Sessions, anomalies and fingerprints will remain.' : 'Sessions, their content and associated alerts will be deleted. Behavior groups will be recalculated from remaining sessions.' }}</p>
       <b-form-checkbox v-model="cleanupKeep">Keep favorites during this cleanup</b-form-checkbox>
       <div class="settings-control-row mt-3"><b-button variant="outline-secondary" :disabled="busyPreview" @click="previewCleanup">{{ busyPreview ? 'Estimating…' : 'Estimate size' }}</b-button><b-button variant="outline-danger" :disabled="!preview || activeCleanup" @click="confirmDelete=false; $bvModal.show('confirmCleanup')">Delete…</b-button></div>
       <div v-if="preview" class="settings-preview mt-3" role="status"><span><strong>{{ preview.sessions }}</strong> {{ cleanupMode==='packets' ? 'sessions in cleanup scope · retained' : 'sessions to delete' }}</span><span><strong>{{ preview.packets }}</strong> blocks</span><span><strong>{{ bytes(preview.payloadBytes) }}</strong> of content</span></div>
       <div v-for="job in jobs.slice(0,3)" :key="job.id" class="settings-job"><span>Cleanup #{{ job.id }} · {{ job.mode==='packets' ? 'packet content' : 'sessions and content' }} · {{ jobStatus(job.status) }}<small class="d-block text-muted">{{ job.mode==='packets' ? 'Blocks deleted' : 'Records deleted (blocks + sessions)' }}: {{ job.deletedRecords }}</small><small v-if="job.mode==='packets'" class="d-block text-muted">Sessions, anomalies and fingerprints are preserved.</small></span><b-button v-if="['queued','running'].includes(job.status)" size="sm" variant="outline-secondary" @click="cancelCleanup(job.id)">Stop</b-button></div>
      </section>
     </div>
    </b-tab>
    <b-tab title="Application"><div class="settings-section"><section class="settings-card"><div class="settings-card-title"><div><h6>Server response timeout</h6><p>Applies to all users. Takes effect after saving without restarting capture.</p></div></div>
     <b-form-group label="Response timeout, seconds" label-for="request-timeout" description="From 5 to 300 seconds. Default: 10; try 30–60 on a slow machine. The browser allows an extra 5 seconds for response transfer."><b-form-input id="request-timeout" type="number" min="5" max="300" step="1" v-model.number="form.requestTimeoutSeconds" class="settings-short-input"/></b-form-group>
     <p class="settings-note">A longer timeout does not speed up processing. Reduce load by pausing auto-refresh or choosing one service. Applies to traffic, anomalies, chat, settings and Docker. Script execution timeout is set separately in the runner window.</p>
    </section></div></b-tab>
    <b-tab title="Notifications"><NotificationSettings/></b-tab>
    <b-tab title="Display"><div class="settings-section"><section class="settings-card"><div class="settings-card-title"><div><h6>Density and data format</h6><p>These settings apply immediately and are saved only in your browser.</p></div><span class="settings-local-label">Local</span></div><div class="row"><b-form-group class="col-md-4" label="Sessions per page" label-for="settings-pageSize"><b-form-input id="settings-pageSize" type="number" min="1" max="100" v-model.number="pageSize"/></b-form-group><b-form-group class="col-md-4" label="Bytes per HEX row" label-for="settings-hexdumpBlockSize"><b-form-input id="settings-hexdumpBlockSize" type="number" min="1" max="64" v-model.number="hexdumpBlockSize"/></b-form-group><b-form-group class="col-md-4" label="HEX row numbering" label-for="settings-hex-base"><b-form-select id="settings-hex-base" v-model="hexdumpLineNumberBase" :options="[{value:10,text:'Decimal'},{value:16,text:'Hexadecimal'}]"/></b-form-group></div></section></div></b-tab>
    <b-tab title="Farm" lazy><FarmSettings/></b-tab>
   </b-tabs>
   <div v-if="activeTab!==5" class="settings-footer"><span class="small" :class="saved?'text-success':'text-muted'" role="status">{{ saved ? 'Settings saved' : 'Server settings change after saving' }}</span><div class="settings-control-row"><b-button variant="outline-secondary" @click="$bvModal.hide('settingsModal')">Close</b-button><b-button variant="primary" :disabled="saving" @click="save">{{ saving ? 'Applying…' : 'Save and apply' }}</b-button></div></div>
  </template>
  <b-modal id="confirmCleanup" title="Confirm cleanup" hide-footer>
   <p>Scope: <strong>{{ serviceOptions.find(s=>s.value===Number(cleanupService))?.text || 'All services' }}</strong>.</p><p v-if="preview">Estimate: {{ preview.sessions }} sessions, {{ preview.packets }} blocks, {{ bytes(preview.payloadBytes) }} of content.</p><p>{{ cleanupMode==='packets' ? 'Packet content will be deleted. Sessions, anomalies and fingerprints will remain.' : 'Sessions, their content and associated alerts within the selected scope will be deleted.' }}</p><b-alert show variant="warning">{{ cleanupKeep ? 'Favorites will be kept.' : 'Favorites will also be deleted.' }} This action cannot be undone.</b-alert><b-form-checkbox v-model="confirmDelete">I confirm deletion of the selected data</b-form-checkbox><div class="settings-control-row justify-content-end mt-3"><b-button variant="outline-secondary" @click="$bvModal.hide('confirmCleanup')">Cancel</b-button><b-button variant="danger" :disabled="!confirmDelete || deleting" @click="startCleanup">{{ deleting ? 'Starting…' : 'Delete data' }}</b-button></div>
  </b-modal>
 </b-modal>
</template>
<script>
import FarmSettings from '../components/FarmSettings.vue';
import NotificationSettings from '../components/NotificationSettings.vue';
const message=e=>e.response?.data?.error || e.message || 'Request failed';
export default {
 components: {NotificationSettings,FarmSettings},
 data:()=>({activeTab:0,openFarmPending:false,form:null,interfaces:[],capture:{},loading:false,saving:false,saved:false,error:'',ipText:'',packetMinutes:0,sessionMinutes:60,jobs:[],timer:null,polling:false,cleanupMode:'packets',cleanupService:0,cleanupKeep:true,preview:null,busyPreview:false,confirmDelete:false,deleting:false}),
 computed:{
  selectedAddresses(){return this.interfaces.find(x=>x.name===this.form?.interface)?.addresses.join(', ');},
  serviceOptions(){return [{value:0,text:'All services'},...this.$store.state.services.map(s=>({value:s.port,text:`${s.name} · ${s.port}`}))];},
  activeCleanup(){return this.jobs.some(j=>['queued','running'].includes(j.status));},
  pageSize:{get(){return this.$store.state.pageSize},set(v){this.$store.commit('setPageSize',Math.max(1,Math.min(100,Number(v)||25)))}},
  hexdumpBlockSize:{get(){return this.$store.state.hexdumpBlockSize},set(v){this.$store.commit('setHexdumpBlockSize',Math.max(1,Math.min(64,Number(v)||16)))}},
  hexdumpLineNumberBase:{get(){return this.$store.state.hexdumpLineNumberBase},set(v){this.$store.commit('setHexdumpLineNumberBase',v)}},
 },
 watch:{packetMinutes(){this.saved=false},sessionMinutes(){this.saved=false},ipText(){this.saved=false},cleanupMode(){this.preview=null},cleanupService(){this.preview=null},cleanupKeep(){this.preview=null},form:{deep:true,handler(){this.saved=false}}},
 mounted(){this.$root.$on('gm:farm-settings',this.openFarm)},
 beforeDestroy(){this.$root.$off('gm:farm-settings',this.openFarm);this.stopPolling()},
 methods:{
 openFarm(){this.openFarmPending=true;this.$bvModal.show('settingsModal')},
  async load(){this.loading=true;this.error='';this.saved=false;try{const [s,i,j]=await Promise.all([this.$http.get('/v2/settings'),this.$http.get('/v2/interfaces'),this.$http.get('/v2/cleanup')]);this.form={...s.data.settings,requestTimeoutSeconds:s.data.settings.requestTimeoutSeconds || s.data.settings.anomalyTimeoutSeconds || 10};this.capture=s.data.capture;this.interfaces=i.data;this.jobs=j.data;this.ipText=this.form.localIPs.join(', ');this.packetMinutes=this.form.packetRetentionSeconds/60;this.sessionMinutes=this.form.sessionRetentionSeconds/60;}catch(e){this.error=message(e)}finally{this.loading=false;if(this.openFarmPending){this.$nextTick(()=>{this.activeTab=5;this.openFarmPending=false})}}},
  startPolling(){this.stopPolling();this.timer=setInterval(this.poll,2000)},stopPolling(){clearInterval(this.timer);this.timer=null},
  async poll(){if(this.polling)return;this.polling=true;try{const [s,j]=await Promise.all([this.$http.get('/v2/settings'),this.$http.get('/v2/cleanup')]);this.capture=s.data.capture;this.jobs=j.data;}catch(e){this.error=message(e)}finally{this.polling=false}},
  async refreshInterfaces(){try{this.interfaces=(await this.$http.get('/v2/interfaces')).data}catch(e){this.error=message(e)}},
  interfaceChanged(){this.form.autoIPs=true;this.ipText=''},
  async save(){if(!Number.isInteger(this.form.requestTimeoutSeconds)||this.form.requestTimeoutSeconds<5||this.form.requestTimeoutSeconds>300){this.error='Response timeout: an integer from 5 to 300 seconds.';return}this.error='';this.saving=true;try{const body={...this.form,localIPs:this.ipText.split(/[\s,]+/).filter(Boolean),packetRetentionSeconds:Number(this.packetMinutes)*60,sessionRetentionSeconds:Number(this.sessionMinutes)*60};const {data}=await this.$http.put('/v2/settings',body);this.form=data.settings;this.capture=data.capture;this.$nextTick(()=>this.saved=true);}catch(e){this.error=message(e)}finally{this.saving=false}},
  async previewCleanup(){this.busyPreview=true;this.error='';try{this.preview=(await this.$http.post('/v2/cleanup/preview',{preserveFavorites:this.cleanupKeep,service:this.cleanupService})).data;}catch(e){this.error=message(e)}finally{this.busyPreview=false}},
  async startCleanup(){this.deleting=true;try{await this.$http.post('/v2/cleanup',{mode:this.cleanupMode,preserveFavorites:this.cleanupKeep,service:this.cleanupService,confirm:'DELETE'});this.$bvModal.hide('confirmCleanup');this.preview=null;await this.poll();}catch(e){this.error=message(e)}finally{this.deleting=false}},
  async cancelCleanup(id){try{await this.$http.delete(`/v2/cleanup/${id}`);await this.poll()}catch(e){this.error=message(e)}},
  duration(n){return n>=1440?`${n/1440} d`:n>=60?`${n/60} h`:`${n} min`},bytes(n){return `${(n/1048576).toFixed(2)} MiB`},jobStatus(s){return {queued:'queued',running:'running',completed:'completed',cancelled:'stopped'}[s]||s},
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
