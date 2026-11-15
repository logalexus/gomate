<template>
 <div class="anomaly-flow-host">
  <b-modal v-if="compact" id="anomaly-flow-modal" visible size="xl" scrollable hide-footer :title="title" @hidden="$emit('close')">
   <template #modal-header><div><h2 id="anomaly-flow-modal___BV_modal_title_" class="flow-title">{{ title }}</h2><p class="flow-subtitle">{{ serviceName }}</p></div><b-button size="sm" variant="outline-secondary" aria-label="Close stream" @click="$emit('close')">Close</b-button></template>
   <p class="flow-caption">Requests and responses · browse the full stream page by page.</p>
   <ContentStream :key="session.streamId" :stream-id="session.streamId" :service-port="session.service" embedded/>
  </b-modal>
  <aside v-else id="anomaly-flow-panel" ref="panel" class="anomaly-flow-panel" :style="panelHeight ? {maxHeight:panelHeight+'px'} : null" aria-labelledby="anomaly-flow-title">
   <header class="flow-heading"><div><h2 id="anomaly-flow-title" class="flow-title">{{ title }}</h2><p class="flow-subtitle">{{ serviceName }}</p></div><b-button size="sm" variant="outline-secondary" aria-label="Close stream" @click="$emit('close')">Close</b-button></header>
   <div class="flow-body"><p class="flow-caption">Requests and responses · browse the full stream page by page.</p><ContentStream :key="session.streamId" :stream-id="session.streamId" :service-port="session.service" embedded/></div>
  </aside>
 </div>
</template>
<script>
import ContentStream from '../views/ContentStream.vue';
export default {
 name:'AnomalyFlowPanel',components:{ContentStream},props:{session:{type:Object,required:true},serviceName:String},
 data:()=>({compact:window.innerWidth<1200,panelHeight:null}),
 computed:{title(){return `Session stream #${this.session.streamId}`}},
 mounted(){window.addEventListener('resize',this.onResize);document.addEventListener('scroll',this.queueMeasure,true);this.queueMeasure()},
 updated(){this.queueMeasure()},
 beforeDestroy(){window.removeEventListener('resize',this.onResize);document.removeEventListener('scroll',this.queueMeasure,true);cancelAnimationFrame(this.measureFrame)},
 methods:{
  onResize(){this.compact=window.innerWidth<1200;this.$nextTick(this.queueMeasure)},
  queueMeasure(){if(this.measureFrame)return;this.measureFrame=requestAnimationFrame(()=>{this.measureFrame=null;const panel=this.$refs.panel;if(panel)this.panelHeight=Math.max(180,window.innerHeight-panel.getBoundingClientRect().top-36)})},
 },
};
</script>
<style scoped>
.anomaly-flow-host{min-width:0;align-self:start;position:sticky;top:0}
.anomaly-flow-panel{display:flex;flex-direction:column;min-width:0;max-height:calc(100vh - 120px);max-height:calc(100dvh - 120px);border:1px solid var(--gm-border);border-radius:8px;background:var(--gm-panel);overflow:hidden}
.flow-heading{display:flex;justify-content:space-between;align-items:flex-start;gap:10px;padding:10px 12px;border-bottom:1px solid var(--gm-border);background:var(--gm-panel-alt)}
.flow-title{font-size:14px;font-weight:600;line-height:1.5;margin:0;color:var(--gm-text)}
.flow-subtitle{font-size:12px;color:var(--gm-muted);margin:2px 0 0}
.flow-caption{font-size:12px;color:var(--gm-muted);margin:0 0 10px}
.flow-body{padding:12px;overflow:auto;min-height:0;overscroll-behavior:contain}
</style>
