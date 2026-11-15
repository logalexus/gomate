<template>
 <b-modal ref="modal" :title="sessionMode?'Full session script':'Request editor'" size="xl" modal-class="workbench-modal" scrollable hide-footer :return-focus="returnFocus" @hide="close">
  <div class="request-workbench">
   <div class="workbench-controls">
    <div><label for="export-format">Pattern</label><select id="export-format" class="custom-select custom-select-sm" v-model="format" @change="settingsChanged"><option value="requests">{{ sessionMode?'HTTP · requests':'Python · requests' }}</option><option v-if="sessionMode" value="tcp">TCP · socket</option><template v-else><option value="curl">Shell · curl</option><option value="pwntools">Python · pwntools</option></template></select></div>
    <div class="workbench-host"><label for="export-host">Service address</label><input id="export-host" class="form-control form-control-sm" v-model="host" :disabled="loading" @input="settingsChanged" placeholder="192.168.56.103" spellcheck="false"/></div>
    <div class="workbench-port"><label for="export-port">Port</label><input id="export-port" class="form-control form-control-sm" v-model="port" :disabled="loading" inputmode="numeric" @input="settingsChanged"/></div>
    <div><label for="export-scheme">Connection</label><select id="export-scheme" class="custom-select custom-select-sm" v-model="scheme" @change="settingsChanged"><option value="http">{{ (format==='pwntools'||format==='tcp')?'No TLS':'HTTP' }}</option><option value="https">{{ (format==='pwntools'||format==='tcp')?'TLS':'HTTPS' }}</option></select></div>
   </div>
   <div v-if="!sessionMode&&prepared.requests.length>1" class="workbench-request"><label for="export-request">Request in block</label><select id="export-request" class="custom-select custom-select-sm" v-model.number="requestIndex" @change="settingsChanged"><option v-for="(request,i) in prepared.requests" :key="i" :value="i">{{ i+1 }} · {{ request.method }} {{ request.target }}</option></select></div>
   <p v-if="loading" class="small text-muted" role="status">{{ sessionMode?`Loading full session… ${sessionProgress.packets} blocks · ${Math.ceil(sessionProgress.bytes/1024)} KiB`:'Loading connection address…' }}</p>
   <p v-if="contextError" class="small text-warning" role="alert">{{ contextError }} <button v-if="sessionMode&&!loading" type="button" class="btn btn-sm btn-link" @click="retrySession">Retry loading</button></p>
   <p v-if="sessionMode&&!loading&&!contextError" class="small text-muted">Session #{{ stream.id }} · {{ sessionPackets.length }} raw blocks · {{ Math.ceil(prepared.bytes/1024) }} KiB<span v-if="prepared.requests.length"> · {{ prepared.requests.length }} HTTP requests</span></p>
   <div class="workbench-panes">
    <section class="workbench-editor-pane" aria-labelledby="workbench-script-title">
   <div class="workbench-editor-toolbar"><span id="workbench-script-title">{{ sessionMode?`Script · session #${sessionId}`:`Script · block #${packet?.id}` }}</span><button type="button" class="btn btn-sm btn-outline-secondary" :disabled="loading||active||(sessionMode&&!!contextError)" @click="regenerate">Regenerate</button></div>
   <p v-if="pending" class="small text-warning">The script has manual edits. Click Regenerate to apply the settings above.</p>
   <p v-if="exportError" class="small text-warning" role="status">{{ exportError }}</p>
   <details v-if="warnings.length" class="workbench-notes"><summary>Replay notes · {{ warnings.length }}</summary><ul><li v-for="warning in warnings" :key="warning">{{ warning }}</li></ul></details>
   <div class="workbench-code"><ScriptEditor ref="editor" v-model="code" :language="language" @input="copyState=''"/></div>
    </section>
   <section class="workbench-result" aria-label="Execution result">
    <div class="workbench-result-title">Output <span v-if="!job" class="text-muted">stdout / stderr</span></div>
    <div v-if="!job" class="workbench-output-empty">{{ active?'Starting…':'Run the script to see the response' }}</div>
    <div v-else class="workbench-output">
    <div class="workbench-result-heading" role="status"><strong>{{ statusLabel }}</strong><span>{{ ((job.elapsedMs||0)/1000).toFixed(2) }} s<span v-if="job.exitCode!=null"> · exit {{ job.exitCode }}</span></span></div>
    <p v-if="job.error" class="small text-danger">{{ job.error }}</p>
    <p v-if="job.truncated" class="small text-warning">Output reached the limit and was truncated.</p>
    <label for="runner-stdout">stdout<span v-if="(job.stdout||'').length>65536"> · last 64 Ki characters</span></label><pre id="runner-stdout" tabindex="0">{{ output(job.stdout)||'No output' }}</pre>
    <template v-if="job.stderr"><label for="runner-stderr">stderr<span v-if="job.stderr.length>65536"> · last 64 Ki characters</span></label><pre id="runner-stderr" tabindex="0" class="workbench-stderr">{{ output(job.stderr) }}</pre></template>
    </div>
   </section>
   </div>
   <div class="workbench-actions">
    <div><label class="sr-only" for="runner-language">Execution language</label><select id="runner-language" class="custom-select custom-select-sm" v-model="language" :disabled="active"><option value="python">Python 3</option><option value="bash">Bash</option></select></div>
    <div class="workbench-timeout"><label for="runner-timeout">Timeout, s</label><input id="runner-timeout" class="form-control form-control-sm" v-model.number="timeout" type="number" min="1" :max="capabilities.maxTimeoutSeconds||120" :disabled="active"/></div>
    <button type="button" class="btn btn-sm btn-primary" :disabled="!canRun" @click="run"><i class="fas fa-play" aria-hidden="true"/> Run</button>
    <button v-if="active" type="button" class="btn btn-sm btn-outline-danger" :disabled="stopping" @click="stop">{{ stopping?'Stopping…':'Stop' }}</button>
    <button type="button" class="btn btn-sm btn-outline-secondary" :disabled="!code" @click="copy">Copy script</button>
    <span v-if="copyState" class="small" role="status">{{ copyState }}</span>
   </div>
   <p class="workbench-run-context">Runs on the gomate server · closing this window stops the script.</p>
   <p v-if="codeBytes>maxCodeBytes" class="small text-warning">You can copy the script. To run it, shorten it to {{ maxCodeBytes/1024 }} KiB.</p>
   <p v-if="runnerError" class="small text-warning" role="alert">{{ runnerError }} <button v-if="!active" class="btn btn-sm btn-link" @click="loadCapabilities">Check availability</button></p>

  </div>
 </b-modal>
</template>
<script>
import {prepareExport,generateExport,prepareSessionExport,generateSessionExport} from '../requestExport.mjs';
import {loadSessionExport} from '../sessionExport.mjs';
const emptyPrepared=()=>({requests:[],raw:'',httpError:'',warnings:[],endpoint:{host:'',port:80,scheme:'http'}});
export default {
 components:{ScriptEditor:()=>import('./ScriptEditor.vue')},
 data:()=>({sessionMode:false,sessionId:null,sessionPackets:[],sessionProgress:{packets:0,bytes:0},loadController:null,packet:null,stream:{},format:'requests',requestIndex:0,host:'',port:80,scheme:'http',prepared:emptyPrepared(),code:'',generatedCode:'',language:'python',warnings:[],exportError:'',contextError:'',pending:false,loading:false,returnFocus:null,copyState:'',timeout:30,capabilities:{},runnerError:'',job:null,starting:false,stopping:false,epoch:0,timer:null}),
 computed:{
  service(){return this.$store.state.services.find(s=>s.port===this.stream.service)||{}},
  active(){return this.starting||['starting','running','stopping'].includes(this.job?.status)},
  codeBytes(){return new TextEncoder().encode(this.code).length},maxCodeBytes(){return this.capabilities.maxCodeBytes||262144},
  canRun(){return !this.active&&!this.loading&&this.capabilities.available&&this.code.trim()&&this.codeBytes<=this.maxCodeBytes&&Number.isInteger(this.timeout)&&this.timeout>=1&&this.timeout<=(this.capabilities.maxTimeoutSeconds||120)},
  statusLabel(){return {starting:'Starting…',running:'Running…',stopping:'Stopping…',completed:'Completed',finished:'Completed',failed:'Error',cancelled:'Stopped',canceled:'Stopped',timeout:'Timed out',timed_out:'Timed out',cleanup_failed:'Container cleanup failed'}[this.job?.status]||this.job?.status},
 },
 beforeDestroy(){this.close()},
 methods:{
  async open({packet,format,streamId,returnFocus}){
   this.close();const epoch=this.epoch;this.sessionMode=false;this.packet=packet;this.format=format;this.requestIndex=0;this.stream={};this.returnFocus=returnFocus;this.code='';this.generatedCode='';this.job=null;this.runnerError='';this.contextError='';this.exportError='';this.warnings=[];this.copyState='';this.pending=false;this.capabilities={};this.loading=true;this.$refs.modal.show();this.loadCapabilities();
   try{const {data}=await this.$http.get(`/stream/${streamId}`);if(epoch!==this.epoch)return;this.stream=data&&data.id?data:{}}catch{if(epoch!==this.epoch)return;this.contextError='Could not load the connection address. Check the address and port manually.'}
   if(epoch!==this.epoch)return;this.prepared=prepareExport(packet,this.stream,this.service);Object.assign(this,this.prepared.endpoint);this.loading=false;this.regenerate();
  },
  async openSession({streamId,servicePort,returnFocus}){
   this.close();const epoch=this.epoch;this.sessionMode=true;this.sessionId=streamId;this.stream={service:servicePort};this.format='requests';this.requestIndex=0;this.returnFocus=returnFocus;this.runnerError='';this.contextError='';this.exportError='';this.warnings=[];this.copyState='';this.pending=false;this.capabilities={};this.loading=true;this.sessionProgress={packets:0,bytes:0};this.loadController=new AbortController();this.$refs.modal.show();this.loadCapabilities();
   try{
    const result=await loadSessionExport(this.$http,streamId,{signal:this.loadController.signal,onProgress:progress=>{if(epoch===this.epoch)this.sessionProgress=progress}});
    if(epoch!==this.epoch)return;this.stream=result.stream;this.sessionPackets=result.packets;this.prepared=prepareSessionExport(this.sessionPackets,this.stream,this.service);Object.assign(this,this.prepared.endpoint);this.format=this.prepared.httpError?'tcp':'requests';this.regenerate();
   }catch(e){if(epoch===this.epoch)this.contextError=e.response?.data?.error||e.message||'Could not load the full session.'}
   finally{if(epoch===this.epoch){this.loading=false;this.loadController=null}}
  },
  retrySession(){this.$refs.editor?.focus({preventScroll:true});this.openSession({streamId:this.sessionId,servicePort:this.stream.service,returnFocus:this.returnFocus})},
  async loadCapabilities(){const epoch=this.epoch;try{const {data}=await this.$http.get('/v2/runner');if(epoch!==this.epoch)return;this.capabilities=data;this.runnerError=data.available?'':'Runner unavailable. You can still edit and copy the script.'}catch{if(epoch===this.epoch)this.runnerError='Could not connect to the runner. You can still edit and copy the script.'}},
  settingsChanged(){if(this.loading||this.active||this.code!==this.generatedCode){this.pending=true;return}this.regenerate()},
  regenerate(){if(this.sessionMode&&this.contextError)return;const generated=(this.sessionMode?generateSessionExport:generateExport)({packets:this.sessionPackets,packet:this.packet,stream:this.stream,service:this.service,format:this.format,host:this.host,port:this.port,scheme:this.scheme,requestIndex:this.requestIndex});this.code=generated.code;this.generatedCode=this.code;this.language=this.format==='curl'?'bash':'python';this.warnings=generated.warnings;this.exportError=generated.error;this.pending=false;this.copyState=''},
  async copy(){try{if(navigator.clipboard&&window.isSecureContext)await navigator.clipboard.writeText(this.code);else{if(!this.$refs.editor?.copy())throw Error('copy')}this.copyState='Script copied'}catch{this.copyState='Select the script and copy it manually.'}},
  async run(){if(!this.canRun)return;const epoch=++this.epoch;clearTimeout(this.timer);this.starting=true;this.$nextTick(()=>this.$refs.editor?.focus({preventScroll:true}));this.job=null;this.runnerError='';try{const {data}=await this.$http.post('/v2/runner/jobs',{language:this.language,code:this.code,timeoutSeconds:this.timeout});if(epoch!==this.epoch){this.cancelRemote(data.id);return}this.job=data;this.poll(epoch)}catch(e){if(epoch===this.epoch)this.runnerError=e.response?.data?.error||'Could not run the script.'}finally{if(epoch===this.epoch)this.starting=false}},
  async poll(epoch){clearTimeout(this.timer);if(epoch!==this.epoch||!this.job?.id)return;try{const {data}=await this.$http.get('/v2/runner/jobs/'+this.job.id);if(epoch!==this.epoch)return;this.job=data;this.runnerError='';if(['starting','running','stopping'].includes(data.status))this.timer=setTimeout(()=>this.poll(epoch),750)}catch(e){if(epoch!==this.epoch)return;this.runnerError='Could not retrieve output. Reconnecting…';this.timer=setTimeout(()=>this.poll(epoch),2000)}},
  cancelRemote(id){if(id)return this.$http.delete('/v2/runner/jobs/'+id).catch(()=>{})},
  async stop(){if(!this.job?.id||this.stopping)return;this.stopping=true;this.$nextTick(()=>this.$refs.editor?.focus({preventScroll:true}));const epoch=this.epoch;try{const {data}=await this.$http.delete('/v2/runner/jobs/'+this.job.id);if(epoch===this.epoch){this.job=data;this.poll(epoch)}}catch(e){if(epoch===this.epoch)this.runnerError=e.response?.data?.error||'Could not stop the process. The server timeout remains active.'}finally{if(epoch===this.epoch)this.stopping=false}},
  close(){const id=this.active?this.job?.id:null;this.epoch++;this.loadController?.abort();this.loadController=null;this.sessionPackets=[];this.loading=false;clearTimeout(this.timer);this.cancelRemote(id);this.job=null;this.starting=false;this.stopping=false;this.code='';this.generatedCode='';this.packet=null;this.prepared=emptyPrepared()},
  output(value){return (value||'').slice(-65536).replace(/\x1b\[[0-?]*[ -/]*[@-~]/g,'')},
 },
};
</script>
<style>
/* The modal is portaled by BootstrapVue; keep these rules scoped by its own class. */
body .workbench-modal .modal-dialog{width:calc(100% - 32px);max-width:1600px;margin:16px auto;height:calc(100% - 32px);max-height:calc(100% - 32px)}
body .workbench-modal .modal-content{height:100%;max-height:100%}
body .workbench-modal .modal-header{padding:10px 16px}
body .workbench-modal .modal-title{font-size:16px}
body .workbench-modal .modal-body{padding:12px 16px;display:flex;min-height:0}
@media(max-width:767px){
 body .workbench-modal .modal-dialog{width:calc(100% - 16px);margin:8px auto;height:calc(100% - 16px);max-height:calc(100% - 16px)}
 body .workbench-modal .modal-body{display:block;padding:10px}
}
</style>
<style scoped>
.request-workbench{font-size:.8rem;display:flex;flex-direction:column;flex:1;min-width:0;min-height:0}
.workbench-controls{display:flex;flex-wrap:wrap;gap:8px;margin-bottom:10px;flex-shrink:0}
.workbench-controls>div{flex:1;min-width:110px}
.workbench-controls .workbench-host{flex:2;min-width:160px}
.workbench-controls .workbench-port{flex:0 0 80px;min-width:80px}
.request-workbench label{display:block;font-size:.75rem;color:var(--gm-muted);margin-bottom:4px}
.workbench-panes{display:grid;grid-template-columns:minmax(0,3fr) minmax(0,2fr);gap:8px;flex:1;min-height:220px}
.workbench-editor-pane,.workbench-result{min-width:0;min-height:0;display:flex;flex-direction:column;border:1px solid var(--gm-border);border-radius:8px;overflow:hidden}
.workbench-editor-toolbar,.workbench-actions,.workbench-result-heading{display:flex;align-items:center;gap:8px;flex-wrap:wrap}
.workbench-editor-toolbar,.workbench-result-title{padding:6px 10px;background:var(--gm-panel-alt);border-bottom:1px solid var(--gm-border);min-height:38px;flex-shrink:0}
.workbench-editor-toolbar,.workbench-result-title{display:flex;align-items:center;justify-content:space-between;gap:8px}
.workbench-editor-pane>p,.workbench-notes{padding:6px 10px;margin:0;flex-shrink:0;max-height:100px;overflow:auto}
.workbench-code{flex:1;min-height:0}
.workbench-notes{font-size:.75rem;color:var(--gm-muted)}
.workbench-notes ul{padding-left:20px;margin:6px 0}
.workbench-actions{margin-top:10px;flex-shrink:0}
.workbench-timeout{display:flex;align-items:center;gap:6px}
.workbench-timeout label{margin:0}.workbench-timeout input{width:72px}
.workbench-run-context{font-size:.75rem;color:var(--gm-muted);margin:6px 0 0}
.workbench-output{padding:10px;overflow:auto;flex:1;min-height:0;background:var(--gm-bg)}
.workbench-output-empty{display:flex;align-items:center;justify-content:center;text-align:center;padding:12px;flex:1;color:var(--gm-muted);background:var(--gm-bg)}
.workbench-result-heading{justify-content:space-between;margin-bottom:8px}
.workbench-result pre{font-family:var(--gm-mono);font-size:.75rem;line-height:1.5;white-space:pre-wrap;overflow-wrap:anywhere;color:var(--gm-text);margin:0 0 10px}
.workbench-result .workbench-stderr{color:var(--gm-warning)}
@media(max-width:767px){
 .workbench-panes{display:flex;flex-direction:column;flex:none}
 .workbench-editor-pane{height:50vh;height:50dvh;min-height:280px}
 .workbench-result{height:240px;flex-shrink:0}
 .workbench-actions{position:sticky;bottom:-10px;background:var(--gm-panel);padding:8px 0;margin-top:0;border-top:1px solid var(--gm-border)}
}
</style>
