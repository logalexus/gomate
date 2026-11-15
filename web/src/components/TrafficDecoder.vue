<template>
 <b-modal ref="modal" title="Traffic decoder" size="xl" scrollable hide-footer :return-focus="returnFocus" @hidden="close" @shown="focusInput">
  <div class="traffic-decoder">
   <div class="decoder-intro"><h4>{{ source }}</h4><p>Detect the encoding or choose one manually. Use the result as input for the next layer; captured traffic stays unchanged.</p></div>
   <div class="decoder-history mb-2">
    <b-button size="sm" variant="outline-secondary" :disabled="!history.length" @click="back">Back</b-button>
    <b-button size="sm" variant="outline-secondary" @click="reset">Source fragment</b-button>
    <span class="small">Step {{ history.length+1 }} / 9</span>
   </div>
   <div class="decoder-workbench"><section class="decoder-input-pane">
   <div class="decoder-pane-heading"><span>01</span><label for="traffic-decode-input">Input</label></div>
   <b-form-textarea id="traffic-decode-input" ref="input" v-model="input" rows="5" spellcheck="false" @input="edited"/>
   <div class="small text-muted mt-1">{{ input.length.toLocaleString('en-US') }} / 65,536 characters. Paste a value or select a smaller fragment.</div>
   <div class="decoder-actions my-2">
    <b-button variant="primary" :disabled="busy||!validInput" @click="run('auto')">{{ busy?'Processing…':'Auto-detect' }}</b-button>
    <div class="decoder-method"><label for="traffic-decode-method" class="small mb-1">Manual decoder</label><b-form-select id="traffic-decode-method" v-model="operation" :options="operations"/></div>
    <b-button variant="outline-primary" :disabled="busy||!validInput" @click="run(operation)">Decode</b-button>
    <b-button v-if="busy" variant="outline-secondary" @click="cancel">Cancel</b-button>
   </div>
   <b-alert :show="!!error" variant="danger" role="alert">{{ error }}</b-alert>
   <p v-if="searched&&!busy" class="small" role="status">{{ candidates.length?'Candidates found: '+candidates.length:'No suitable text decoding found. Select the value without surrounding text or choose a manual decoder.' }}</p>
   <p v-if="limited" class="small text-warning">Auto-detection limit reached. Select a smaller fragment and try again.</p>
   <div v-if="candidates.length" class="decoder-candidates mb-2" aria-label="Decoding candidates">
    <button v-for="(candidate,i) in candidates" :key="i" type="button" class="decoder-candidate btn btn-outline-secondary text-left" :class="{active:selected===i}" :aria-pressed="String(selected===i)" @click="choose(i)">
     <strong>{{ candidate.label }}</strong><span class="small ml-2">position {{ candidate.offset }}</span><code>{{ candidate.text.slice(0,180) }}{{ candidate.text.length>180?'…':'' }}</code>
    </button>
   </div>
   </section>
   <section v-if="output" class="decoder-output-pane" aria-label="Decoded result">
    <div class="decoder-pane-heading"><span>02</span><h5>Result · {{ output.label }}</h5></div><p class="decoder-output-size">{{ output.byteLength.toLocaleString('en-US') }} decoded bytes</p>
    <b-alert :show="!!output.warning" variant="warning">{{ output.warning }}</b-alert>
    <template v-if="output.jwt">
     <label for="traffic-jwt-header">JWT header</label><b-form-textarea id="traffic-jwt-header" :value="output.jwt.header" readonly rows="3" spellcheck="false"/>
     <label for="traffic-jwt-payload" class="mt-2">JWT payload</label><b-form-textarea id="traffic-jwt-payload" ref="payload" @focus="lastResultField='payload'" :value="output.jwt.payload" readonly rows="6" spellcheck="false"/>
     <ul v-if="output.jwt.dates.length" class="small mt-2"><li v-for="date in output.jwt.dates" :key="date.field">{{ date.field }}: {{ date.value }} (UTC)</li></ul>
     <b-button size="sm" class="my-2" variant="outline-primary" :disabled="history.length>=8" @click="useText(output.jwt.payload,'JWT payload')">Payload → input</b-button>
    </template>
    <b-form-checkbox v-if="!output.binary&&!output.jwt" v-model="showHex" class="mb-2">Show HEX</b-form-checkbox>
    <p v-if="output.binary" class="small">Binary result: this is not UTF-8 text. Showing HEX instead.</p>
    <p v-if="(showHex||output.binary)&&output.hexTruncated" class="small text-warning">HEX is limited to the first 4096 bytes. Copying below also copies only the displayed fragment.</p>
    <label for="traffic-decode-output">{{ showHex||output.binary?'HEX':'Result text' }}</label>
    <b-form-textarea id="traffic-decode-output" ref="output" @focus="lastResultField='output'" :value="resultText" readonly rows="8" spellcheck="false"/>
    <div class="decoder-actions mt-2">
     <b-button variant="outline-secondary" @click="copy">Copy result</b-button>
     <b-button variant="outline-primary" :disabled="output.binary||showHex||history.length>=8||output.text.length>maxInput" @click="useText(output.text,output.label)">Result → input</b-button>
     <b-button variant="outline-primary" :disabled="history.length>=8" @mousedown.prevent @click="useSelection">Selected result → input</b-button>
    </div>
    <span v-if="copyState" class="small" role="status">{{ copyState }}</span>
    <p class="small text-muted mt-2 mb-0">For nested Base64 in JSON, select only the value without quotes. Decoded text is not included in server pattern searches.</p>
   </section>
   <div v-else class="decoder-awaiting"><i :class="busy?'fas fa-circle-notch fa-spin':'fas fa-code'" aria-hidden="true"/><h5>{{ busy?'Decoding…':'Your result will appear here' }}</h5><p>Base64, JWT, URL, HEX and JSON.<br>Up to eight consecutive transformations.</p></div>
   </div>
  </div>
 </b-modal>
</template>
<script>
import {MAX_INPUT,OPERATIONS} from '../trafficDecode.mjs';
export default {
 data:()=>({input:'',original:'',originalSource:'',source:'',operation:'base64',operations:OPERATIONS,maxInput:MAX_INPUT,history:[],candidates:[],output:null,selected:-1,searched:false,limited:false,busy:false,error:'',showHex:false,copyState:'',lastResultField:'output',returnFocus:null}),
 computed:{validInput(){return !!this.input.trim()&&this.input.length<=MAX_INPUT},resultText(){return this.output?(this.showHex||this.output.binary?this.output.hex:this.output.text):''}},
 beforeDestroy(){this.stop()},
 methods:{
  open({text,source,operation,returnFocus}){this.stop();this.returnFocus=returnFocus;this.original=text;this.originalSource=source;this.source=source;this.input=text;this.history=[];this.operation=operation||'base64';this.clear();this.$refs.modal.show();this.run(operation||'auto')},
  focusInput(){this.$refs.input?.$el.focus()},
  stop(){clearTimeout(this.deadline);this.worker?.terminate();this.worker=null;this.busy=false},
  cancel(){this.stop();this.error='Processing canceled.';this.$nextTick(this.focusInput)},
  close(){this.stop();this.clear();this.input='';this.original='';this.history=[]},
  clear(){this.candidates=[];this.output=null;this.selected=-1;this.searched=false;this.limited=false;this.error='';this.showHex=false;this.copyState=''},
  edited(){this.stop();this.clear()},
  run(operation){
   this.stop();this.clear();
   if(!this.validInput){this.error=this.input.length>MAX_INPUT?'Limit: 65,536 characters. Select a smaller fragment.':'Enter data to decode.';return}
   this.busy=true;
   // Disabling the action button otherwise drops focus to <body>, outside
   // Bootstrap's modal Escape handler. Keep keyboard focus in the dialog.
   this.$nextTick(this.focusInput);
   try{
    const worker=new Worker(new URL('../trafficDecode.worker.mjs',import.meta.url),{type:'module'});this.worker=worker;
    worker.onmessage=({data})=>{if(this.worker!==worker)return;this.stop();if(data.error){this.error=data.error;return}if(operation==='auto'){this.searched=true;this.candidates=data.result.results;this.limited=data.result.limited;if(this.candidates.length)this.choose(0)}else this.output=data.result};
    worker.onerror=()=>{if(this.worker!==worker)return;this.stop();this.error='Could not start the decoder. Refresh the page and try again.'};
    this.deadline=setTimeout(()=>{if(this.worker!==worker)return;this.stop();this.error='Processing timed out (2 seconds). Select a smaller fragment.'},2000);
    worker.postMessage({operation,input:this.input});
   }catch{this.stop();this.error='Web Workers are unavailable in this browser.'}
  },
  choose(index){this.selected=index;this.output=this.candidates[index];this.showHex=false;this.copyState=''},
  useText(text,label){if(this.history.length>=8||text.length>MAX_INPUT)return;this.history.push({input:this.input,source:this.source});this.input=text;this.source=label;this.run('auto')},
  useSelection(){
   const el=this.$refs[this.lastResultField]?.$el;if(el&&el.selectionEnd>el.selectionStart){this.useText(el.value.slice(el.selectionStart,el.selectionEnd),'Selected result');return}
   this.copyState='Select a fragment in the result field or JWT payload first.';
  },
  back(){const previous=this.history.pop();if(!previous)return;this.input=previous.input;this.source=previous.source;this.run('auto')},
  reset(){this.input=this.original;this.source=this.originalSource;this.history=[];this.run('auto')},
  async copy(){try{if(navigator.clipboard&&window.isSecureContext)await navigator.clipboard.writeText(this.resultText);else{const el=this.$refs.output.$el;el.focus();el.select();if(!document.execCommand('copy'))throw Error('copy')}this.copyState='Result copied'}catch{this.copyState='Select the result text and copy it manually.'}},
 },
};
</script>
<style scoped>
.traffic-decoder{color:var(--gm-text)}.decoder-intro{margin-bottom:10px}.decoder-intro h4{font-size:.9rem;font-weight:600;letter-spacing:-.02em;margin:0 0 4px}.decoder-intro p{color:var(--gm-muted);font-size:.76rem;line-height:1.65;max-width:740px;margin:0}.traffic-decoder textarea{font-family:var(--gm-mono);font-size:.75rem;line-height:1.7;tab-size:4}.decoder-history{padding:6px 0;border-top:1px solid var(--gm-border);border-bottom:1px solid var(--gm-border);margin-bottom:10px!important}.decoder-history>span{margin-left:auto;color:var(--gm-muted);font-family:var(--gm-mono);font-size:.75rem}.decoder-workbench{display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1fr);gap:10px;align-items:start}.decoder-input-pane,.decoder-output-pane{min-width:0;padding:10px;border:1px solid var(--gm-border);border-radius:var(--gm-radius);background:var(--gm-panel-alt)}.decoder-pane-heading{display:flex;align-items:center;gap:8px;margin-bottom:8px}.decoder-pane-heading>span{font-family:var(--gm-mono);font-size:.75rem;color:var(--gm-accent);border:1px solid var(--gm-border);border-radius:4px;padding:3px 6px}.decoder-pane-heading label,.decoder-pane-heading h5{font-size:.8rem;font-weight:600;margin:0}.decoder-output-size{color:var(--gm-muted);font-size:.75rem;margin-top:-4px}.traffic-decoder .decoder-actions,.traffic-decoder .decoder-history{display:flex;flex-wrap:wrap;gap:8px;align-items:center}.traffic-decoder .decoder-actions{align-items:end}.traffic-decoder .decoder-actions>.btn{font-size:.75rem}.traffic-decoder .decoder-method{flex:1;min-width:170px}.traffic-decoder .decoder-candidates{display:grid;gap:8px;max-height:260px;overflow:auto}.traffic-decoder .decoder-candidate{white-space:normal;overflow-wrap:anywhere;padding:8px;font-size:.75rem;border-color:var(--gm-border);background:var(--gm-panel);color:var(--gm-text)}.traffic-decoder .decoder-candidate.active{border-color:var(--gm-accent);background:var(--gm-accent-soft);color:var(--gm-accent)}.traffic-decoder .decoder-candidate code{display:block;color:var(--gm-text);white-space:pre-wrap;max-height:4.5em;overflow:hidden;font-family:var(--gm-mono);font-size:.75rem;margin-top:7px}.traffic-decoder label{font-weight:600;font-size:.75rem}.traffic-decoder .small{font-size:.75rem;line-height:1.6}.decoder-awaiting{min-height:240px;border:1px dashed var(--gm-border);border-radius:var(--gm-radius);display:flex;flex-direction:column;align-items:center;justify-content:center;text-align:center;padding:16px;color:var(--gm-muted)}.decoder-awaiting>i{font-size:1.25rem;margin-bottom:12px;color:var(--gm-accent)}.decoder-awaiting h5{color:var(--gm-text);font-size:.9rem}.decoder-awaiting p{font-size:.75rem;line-height:1.7;margin:0}@media(max-width:850px){.decoder-workbench{grid-template-columns:minmax(0,1fr)}.decoder-awaiting{min-height:180px}.decoder-input-pane,.decoder-output-pane{padding:10px}}
</style>
