<template>
 <b-modal @ok="lookBack" @show="error=''" id="lookBackModal" title="Search saved traffic" cancel-title="Cancel" :ok-title="busy?'Starting…':'Start search'" :ok-disabled="busy" centered scrollable>
  <div class="lookback-summary"><span class="lookback-eyebrow">CHECK HISTORY</span><h6>{{ patternName }}</h6><p>Find matches in saved requests and responses.</p></div>
  <b-alert :show="!!error" variant="danger" role="alert">{{ error }}</b-alert>
  <form ref="lookBackForm" @submit.prevent="lookBack">
   <b-form-group label="Look back, minutes" label-for="lookback-minutes" description="Counted back from now. Maximum: 7 days."><b-form-input id="lookback-minutes" required type="number" min="1" max="10080" v-model.number="minutes"/></b-form-group>
   <div class="lookback-presets"><b-button v-for="preset in presets" :key="preset.minutes" type="button" variant="outline-secondary" size="sm" :aria-pressed="String(minutes===preset.minutes)" @click="minutes=preset.minutes">{{ preset.label }}</b-button></div>
  </form>
  <div class="lookback-note"><i class="fas fa-history" aria-hidden="true"/><p>Search runs in the background across raw and parsed blocks. LIVE capture continues; job progress appears in the gomate status bar.</p></div>
 </b-modal>
</template>
<script>
export default {
 name:'LookBack',props:{patternId:Number},
 data:()=>({minutes:5,busy:false,error:'',presets:[{minutes:5,label:'5 minutes'},{minutes:30,label:'30 minutes'},{minutes:60,label:'1 hour'},{minutes:360,label:'6 hours'},{minutes:1440,label:'24 hours'}]}),
 computed:{patternName(){return this.$store.state.patterns.find(p=>p.id===this.patternId)?.name||'Selected pattern'}},
 methods:{
  async lookBack(ev){
   ev?.preventDefault();if(this.busy||!this.$refs.lookBackForm.reportValidity())return;
   this.busy=true;this.error='';
   try{const {data}=await this.$http.post(`pattern/${this.patternId}/lookback`,this.minutes);this.$bvToast.toast(`Job #${data.id} queued. Track progress in the gomate status bar.`,{title:'Search history',variant:'info'});this.$bvModal.hide('lookBackModal')}
   catch(e){this.error=e.response?.data?.error||e.message||'Could not start the search.'}
   finally{this.busy=false}
  },
 },
};
</script>
<style scoped>
.lookback-eyebrow{font-size:11px;font-weight:700;letter-spacing:.08em;color:var(--gm-accent)}.lookback-summary h6{font-size:14px;margin:4px 0;font-weight:600;overflow-wrap:anywhere}.lookback-summary p{color:var(--gm-muted);font-size:12px;margin-bottom:12px}.lookback-presets{display:flex;flex-wrap:wrap;gap:6px}.lookback-note{display:flex;gap:8px;align-items:start;background:var(--gm-panel-alt);padding:10px;border-radius:8px;margin-top:12px;border:1px solid var(--gm-border)}.lookback-note i{color:var(--gm-accent);margin-top:3px}.lookback-note p{font-size:12px;color:var(--gm-muted);margin:0}
</style>
