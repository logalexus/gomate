<template>
 <b-modal @ok="lookBack" @show="error=''" id="lookBackModal" title="Поиск в сохранённом трафике" cancel-title="Отмена" :ok-title="busy?'Запуск…':'Начать поиск'" :ok-disabled="busy" centered scrollable>
  <div class="lookback-summary"><span class="lookback-eyebrow">ПРОВЕРИТЬ ИСТОРИЮ</span><h6>{{ patternName }}</h6><p>Найдите совпадения в уже сохранённых запросах и ответах.</p></div>
  <b-alert :show="!!error" variant="danger" role="alert">{{ error }}</b-alert>
  <form ref="lookBackForm" @submit.prevent="lookBack">
   <b-form-group label="За сколько минут проверить трафик" label-for="lookback-minutes" description="От текущего времени назад. Максимум — 7 дней."><b-form-input id="lookback-minutes" required type="number" min="1" max="10080" v-model.number="minutes"/></b-form-group>
   <div class="lookback-presets"><b-button v-for="preset in presets" :key="preset.minutes" type="button" variant="outline-secondary" size="sm" :aria-pressed="String(minutes===preset.minutes)" @click="minutes=preset.minutes">{{ preset.label }}</b-button></div>
  </form>
  <div class="lookback-note"><i class="fas fa-history" aria-hidden="true"/><p>Поиск выполняется в фоне по исходным и разобранным блокам. LIVE-захват продолжает работать; прогресс задания появится в статусе gomate.</p></div>
 </b-modal>
</template>
<script>
export default {
 name:'LookBack',props:{patternId:Number},
 data:()=>({minutes:5,busy:false,error:'',presets:[{minutes:5,label:'5 минут'},{minutes:30,label:'30 минут'},{minutes:60,label:'1 час'},{minutes:360,label:'6 часов'},{minutes:1440,label:'24 часа'}]}),
 computed:{patternName(){return this.$store.state.patterns.find(p=>p.id===this.patternId)?.name||'Выбранный шаблон'}},
 methods:{
  async lookBack(ev){
   ev?.preventDefault();if(this.busy||!this.$refs.lookBackForm.reportValidity())return;
   this.busy=true;this.error='';
   try{const {data}=await this.$http.post(`pattern/${this.patternId}/lookback`,this.minutes);this.$bvToast.toast(`Задание #${data.id} добавлено в очередь. Следите за прогрессом в статусе gomate.`,{title:'Поиск в прошлом',variant:'info'});this.$bvModal.hide('lookBackModal')}
   catch(e){this.error=e.response?.data?.error||e.message||'Не удалось запустить поиск.'}
   finally{this.busy=false}
  },
 },
};
</script>
<style scoped>
.lookback-eyebrow{font-size:.68rem;font-weight:700;letter-spacing:.14em;color:var(--gm-accent)}.lookback-summary h6{font-size:1.1rem;margin:8px 0;font-weight:600;overflow-wrap:anywhere}.lookback-summary p{color:var(--gm-muted);font-size:.9rem;margin-bottom:24px}.lookback-presets{display:flex;flex-wrap:wrap;gap:8px}.lookback-note{display:flex;gap:12px;align-items:start;background:var(--gm-panel-alt);padding:16px;border-radius:var(--gm-radius);margin-top:24px;border:1px solid var(--gm-border)}.lookback-note i{color:var(--gm-accent);margin-top:4px}.lookback-note p{font-size:.8rem;color:var(--gm-muted);margin:0}
</style>
