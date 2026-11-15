<template>
 <article class="resource-chart gm-panel" :class="capacity>0?'is-load gm-load-'+load.level:'is-throughput'">
  <header><h3>{{ title }}</h3><span v-if="capacity>0" class="resource-load-badge">{{ load.label }}</span></header>
  <p class="resource-chart-subtitle">{{ subtitle }}</p>
  <div class="resource-legend">
   <span v-for="(s,i) in series" :key="s.key"><i :class="{'is-secondary':i===1}" aria-hidden="true"/>{{ s.label }} <strong>{{ format(last[s.key],unit) }}</strong></span>
  </div>
  <div v-if="!hasData" class="resource-chart-empty">{{ points.length < 2 ? 'Накапливаем первые замеры…' : 'Нет данных за этот период' }}</div>
  <svg v-else viewBox="0 0 560 134" role="img" tabindex="0" :aria-label="title+'. Стрелки влево и вправо — значения по времени.'" @mousemove="inspect" @mouseleave="index=null" @focus="index=points.length-1" @blur="index=null" @keydown.left.prevent="step(-1)" @keydown.right.prevent="step(1)">
   <defs v-if="capacity>0"><linearGradient :id="gradientId" gradientUnits="userSpaceOnUse" x1="0" y1="106" x2="0" y2="10"><stop offset="0" class="resource-stop-normal"/><stop :offset="thresholdOffset(0.7)" class="resource-stop-normal"/><stop :offset="thresholdOffset(0.7)" class="resource-stop-warning"/><stop :offset="thresholdOffset(0.9)" class="resource-stop-warning"/><stop :offset="thresholdOffset(0.9)" class="resource-stop-danger"/><stop offset="1" class="resource-stop-danger"/></linearGradient></defs>
   <g v-if="capacity>0" aria-hidden="true"><rect x="48" y="10" width="494" :height="thresholdY(0.9)-10" class="resource-band-danger"/><rect x="48" :y="thresholdY(0.9)" width="494" :height="thresholdY(0.7)-thresholdY(0.9)" class="resource-band-warning"/></g>
   <g v-for="fraction in [0,0.5,1]" :key="fraction"><line x1="48" x2="542" :y1="106-fraction*96" :y2="106-fraction*96" class="resource-grid"/><text x="43" :y="110-fraction*96" text-anchor="end">{{ axis(maximum*fraction) }}</text></g>
   <g v-if="capacity>0" aria-hidden="true"><line v-for="fraction in [0.7,0.9]" :key="fraction" x1="48" x2="542" :y1="thresholdY(fraction)" :y2="thresholdY(fraction)" class="resource-threshold"/></g>
   <path v-for="(s,i) in series" :key="s.key" :d="path(s.key)" :style="capacity>0?{stroke:'url(#'+gradientId+')'}:null" class="resource-line" :class="{'is-secondary':i===1}"/>
   <circle v-for="(s,i) in series" :key="s.key+'last'" v-show="last[s.key]!=null" :cx="x(last.time)" :cy="106-96*Math.max(0,Math.min(1,last[s.key]/maximum))" r="2.5" class="resource-dot" :class="{'is-secondary':i===1}"/>
   <line v-if="hover" :x1="x(hover.time)" :x2="x(hover.time)" y1="8" y2="106" class="resource-cursor"/>
   <text x="48" y="129">{{ time(start) }}</text><text x="542" y="129" text-anchor="end">{{ time(end) }}</text>
  </svg>
  <div class="resource-chart-values" :aria-live="index!==null?'polite':'off'">
   <template v-if="hover"><span>{{ time(hover.time,true) }}</span><span v-for="s in series" :key="s.key">{{ s.label }}: {{ format(hover[s.key],unit) }}</span></template>
   <template v-else><span>Макс. {{ format(peak,unit) }}</span><span>{{ series[0].label }} · среднее {{ format(average,unit) }}</span></template>
  </div>
 </article>
</template>
<script>
import {formatMetric,chartPath,resourceLoad} from '../monitor.mjs';
let nextChartId=0;
export default {
 name:'ResourceChart',
 props:{title:String,subtitle:String,points:{type:Array,default:()=>[]},series:{type:Array,required:true},unit:{type:String,default:'bytes'},ceiling:{type:Number,default:0},capacity:Number,start:Number,end:Number},
 data:()=>({index:null,gradientId:'resource-load-gradient-'+nextChartId++}),
 computed:{
  last(){return this.points[this.points.length-1]||{}},
  load(){return resourceLoad(this.last[this.series[0].key],this.capacity)},
  values(){return this.points.flatMap(p=>this.series.map(s=>p[s.key])).filter(v=>v!=null&&Number.isFinite(v))},
  hasData(){return this.values.length>0},
  peak(){return this.hasData?Math.max(...this.values):null},
  average(){const values=this.points.map(p=>p[this.series[0].key]).filter(v=>v!=null&&Number.isFinite(v));return values.length?values.reduce((a,b)=>a+b,0)/values.length:null},
  maximum(){return Math.max(1,this.ceiling,this.capacity||0,this.peak||0)},
  hover(){return this.index===null?null:this.points[Math.min(this.index,this.points.length-1)]},
 },
 methods:{
  format:formatMetric,
  thresholdOffset(fraction){return Math.min(1,fraction*this.capacity/this.maximum)},
  thresholdY(fraction){return 106-96*this.thresholdOffset(fraction)},
  axis(v){return this.unit==='percent'?Math.round(v)+'%':formatMetric(v).replace(/\s/g,'')},
  time(v,seconds=false){return new Date(v).toLocaleTimeString('ru-RU',{hour:'2-digit',minute:'2-digit',...(seconds?{second:'2-digit'}:{})})},
  path(key){return chartPath(this.points,key,this.start,this.end,this.maximum)},
  x(t){return 48+494*(t-this.start)/Math.max(1,this.end-this.start)},
  step(delta){this.index=Math.max(0,Math.min(this.points.length-1,(this.index===null?this.points.length-1:this.index)+delta))},
  inspect(e){const svg=e.currentTarget,point=svg.createSVGPoint();point.x=e.clientX;point.y=e.clientY;const matrix=svg.getScreenCTM();if(!matrix)return;const t=this.start+Math.max(0,Math.min(1,(point.matrixTransform(matrix.inverse()).x-48)/494))*(this.end-this.start);let best=0;this.points.forEach((p,i)=>{if(Math.abs(p.time-t)<Math.abs(this.points[best].time-t))best=i});this.index=best},
 },
};
</script>
<style scoped>
.resource-chart{padding:12px;min-width:0}.resource-chart header{display:flex;align-items:baseline;justify-content:space-between;gap:8px}.resource-chart h3{font-size:14px;font-weight:650;margin:0}.resource-legend{display:flex;gap:12px;flex-wrap:wrap;min-height:32px;align-items:center;font-size:12px;color:var(--gm-muted)}.resource-legend strong{color:var(--gm-text);font-variant-numeric:tabular-nums;margin-left:4px}.resource-legend i{display:inline-block;width:13px;border-top:2px solid var(--gm-info);margin-right:5px}.resource-legend i.is-secondary{border-top:2px dashed var(--gm-accent)}svg{display:block;width:100%;height:140px;overflow:visible}svg text{fill:var(--gm-muted);font:11px var(--gm-font)}.resource-grid{stroke:var(--gm-border);stroke-width:1}.resource-line{fill:none;stroke:var(--gm-info);stroke-width:2;vector-effect:non-scaling-stroke}.resource-line.is-secondary{stroke:var(--gm-accent);stroke-dasharray:5 3}.resource-dot{fill:var(--gm-info)}.resource-dot.is-secondary{fill:var(--gm-accent)}.resource-cursor{stroke:var(--gm-muted);stroke-dasharray:3 3}.resource-chart-values{display:flex;flex-wrap:wrap;gap:12px;min-height:20px;font-size:12px;color:var(--gm-muted);font-variant-numeric:tabular-nums}.resource-chart-empty{height:140px;display:grid;place-items:center;color:var(--gm-muted);font-size:13px}

.resource-chart header{align-items:center;flex-wrap:wrap}.resource-chart-subtitle{font-size:12px;color:var(--gm-muted);margin:4px 0 0;min-height:18px}.resource-load-badge{color:var(--gm-load-color);background:var(--gm-load-soft);font-size:11px;font-weight:600;padding:2px 6px;border-radius:4px}.resource-chart.is-load{border-top:2px solid var(--gm-load-color)}.is-load .resource-legend strong{color:var(--gm-load-color)}.is-load .resource-legend i{border-color:var(--gm-load-color)}.is-load .resource-dot{fill:var(--gm-load-color)}.resource-stop-normal{stop-color:var(--gm-success)}.resource-stop-warning{stop-color:var(--gm-warning)}.resource-stop-danger{stop-color:var(--gm-danger)}.resource-band-warning{fill:var(--gm-warning-soft);opacity:.55}.resource-band-danger{fill:var(--gm-danger-soft);opacity:.55}.resource-threshold{stroke:var(--gm-muted);stroke-opacity:.4;stroke-dasharray:2 4;vector-effect:non-scaling-stroke}
</style>
