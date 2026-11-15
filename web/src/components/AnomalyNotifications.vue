<template>
 <aside v-if="count || gap" class="anomaly-notification" aria-label="Уведомления об аномалиях" data-testid="anomaly-notification" @mouseenter="hold" @mouseleave="release" @focusin="hold" @focusout="release">
  <div class="notice-heading"><strong role="status" aria-live="polite">Аномалии с высоким скором · {{ count }}</strong><button type="button" class="btn btn-sm btn-outline-secondary" aria-label="Закрыть уведомление" @click="dismiss">×</button></div>
  <p v-if="count" class="notice-summary">Максимальный скор <strong>{{ maxScore.toFixed(1) }}</strong> · порог ≥ {{ threshold }}</p>
  <ul v-if="items.length"><li v-for="item in items" :key="item.streamId"><router-link :to="{name:'stream',params:{servicePort:item.service,streamId:item.streamId}}" @click.native="dismiss">Сессия #{{ item.streamId }}</router-link><span>порт {{ item.service }} <b>{{ item.score.toFixed(1) }}</b></span></li></ul>
  <p v-if="gap" class="notice-summary">Часть событий пропущена после перерыва. Проверьте список аномалий.</p>
  <router-link :to="{name:'anomalies'}" class="notice-all" @click.native="dismiss">Открыть аномалии<span v-if="count > items.length"> · ещё {{ count-items.length }}</span></router-link>
 </aside>
</template>
<script>
import {eligibleNotices, notificationScore} from '../utils/anomaly-notifications.mjs';
const storageKey='gomate.anomaly-notifications.v1';
export default {
 data:()=>({cursor:'',seen:new Set(),items:[],count:0,maxScore:0,gap:false,timer:null,busy:false,generation:0,stopped:false,controller:null,hideTimer:null,held:false}),
 computed:{enabled(){return this.$store.state.anomalyNotificationsEnabled !== false},threshold(){return notificationScore(this.$store.state.anomalyNotificationScore)}},
 watch:{enabled(){this.restart()},threshold(){this.restart()}},
 mounted(){
  try {const saved=JSON.parse(sessionStorage.getItem(storageKey));if(saved){this.seen=new Set((saved.seen||[]).filter(Number.isSafeInteger).slice(-10000));}} catch (_) { /* Storage may be disabled. */ }
  // Every page load starts from now, including after a long closed-tab period.
  this.poll();this.timer=setInterval(this.poll,2000);
 },
 beforeDestroy(){this.stopped=true;this.generation++;this.controller?.abort();clearInterval(this.timer);clearTimeout(this.hideTimer)},
 methods:{
  hold(){this.held=true;clearTimeout(this.hideTimer)},
  release(event){if(event?.currentTarget?.contains(event.relatedTarget))return;this.held=false;this.scheduleHide()},
  scheduleHide(){clearTimeout(this.hideTimer);if(!this.held)this.hideTimer=setTimeout(this.dismiss,12000)},
  dismiss(){clearTimeout(this.hideTimer);this.held=false;this.items=[];this.count=0;this.maxScore=0;this.gap=false},
  restart(){this.generation++;this.cursor='';this.controller?.abort();this.dismiss();this.$nextTick(this.poll)},
  async poll(){
   if(this.stopped || !this.enabled || this.busy)return;
   this.busy=true;const generation=this.generation;this.controller=new AbortController();
   try {
    const {data}=await this.$http.get('/v2/anomalies/notifications',{params:{cursor:this.cursor},signal:this.controller.signal});
    if(this.stopped || generation!==this.generation || !this.enabled || typeof data.cursor!=='string' || !Array.isArray(data.items))return;
    const baseline=!this.cursor;this.cursor=data.cursor;
    if(baseline || data.reset)return;
    this.gap=this.gap || data.gap;
    if(data.gap)this.scheduleHide();
    const fresh=eligibleNotices(data.items,this.threshold,this.seen);
    if(!fresh.length)return;
    for(const item of fresh)this.seen.add(item.streamId);
    while(this.seen.size>10000)this.seen.delete(this.seen.values().next().value);
    try {sessionStorage.setItem(storageKey,JSON.stringify({seen:[...this.seen]}))} catch (_) { /* In-memory deduplication still works. */ }
    this.count+=fresh.length;this.maxScore=Math.max(this.maxScore,...fresh.map(x=>x.score));
    this.items=[...this.items,...fresh].sort((a,b)=>b.score-a.score).slice(0,3);
    this.scheduleHide();
   } catch (_) { /* Retry with the same cursor; transient failures must not lose events. */ }
   finally {this.busy=false;this.controller=null}
  },
 },
};
</script>
<style scoped>
.anomaly-notification{position:fixed;z-index:1040;right:12px;top:58px;width:340px;max-width:calc(100vw - 24px);padding:10px 12px;background:var(--gm-panel);color:var(--gm-text);border:1px solid var(--gm-border);border-left:3px solid var(--gm-warning,#d98d21);border-radius:8px;box-shadow:0 6px 24px #0003;font-size:12px}
.notice-heading{display:flex;align-items:center;justify-content:space-between;gap:8px}.notice-heading strong{font-size:13px}.notice-heading button{font-size:18px;padding:0 7px;line-height:24px}.notice-summary{margin:6px 0;color:var(--gm-muted)}ul{list-style:none;padding:0;margin:8px 0}li{display:flex;justify-content:space-between;gap:8px;margin:5px 0;flex-wrap:wrap}li span{color:var(--gm-muted)}li b{margin-left:6px;color:var(--gm-text)}.notice-all{font-weight:600}
</style>
