<template>
 <article class="resource-chart gm-panel">
  <header><h3>{{ title }}</h3><span>{{ subtitle }}</span></header>
  <div class="resource-legend">
   <span v-for="(s,i) in series" :key="s.key"><i :class="{'is-secondary':i===1}" aria-hidden="true"/>{{ s.label }} <strong>{{ format(last[s.key],unit) }}</strong></span>
  </div>
  <div v-if="!hasData" class="resource-chart-empty">{{ points.length < 2 ? 'Накапливаем первые замеры…' : 'Нет данных за этот период' }}</div>
  <svg v-else viewBox="0 0 560 134" role="img" tabindex="0" :aria-label="title+'. Стрелки влево и вправо — значения по времени.'" @mousemove="inspect" @mouseleave="index=null" @focus="index=points.length-1" @blur="index=null" @keydown.left.prevent="step(-1)" @keydown.right.prevent="step(1)">
   <g v-for="fraction in [0,0.5,1]" :key="fraction"><line x1="48" x2="542" :y1="106-fraction*96" :y2="106-fraction*96" class="resource-grid"/><text x="43" :y="110-fraction*96" text-anchor="end">{{ axis(maximum*fraction) }}</text></g>
   <path v-for="(s,i) in series" :key="s.key" :d="path(s.key)" class="resource-line" :class="{'is-secondary':i===1}"/>
   <circle v-for="(s,i) in series" :key="s.key+'last'" v-show="last[s.key]!=null" :cx="x(last.time)" :cy="106-96*Math.min(1,last[s.key]/maximum)" r="2.5" class="resource-dot" :class="{'is-secondary':i===1}"/>
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
import {formatMetric,chartPath} from '../monitor.mjs';
export default {
 name:'ResourceChart',
 props:{title:String,subtitle:String,points:{type:Array,default:()=>[]},series:{type:Array,required:true},unit:{type:String,default:'bytes'},ceiling:{type:Number,default:0},start:Number,end:Number},
 data:()=>({index:null}),
 computed:{
  last(){return this.points[this.points.length-1]||{}},
  values(){return this.points.flatMap(p=>this.series.map(s=>p[s.key])).filter(v=>v!=null&&Number.isFinite(v))},
  hasData(){return this.values.length>0},
  peak(){return this.hasData?Math.max(...this.values):null},
  average(){const values=this.points.map(p=>p[this.series[0].key]).filter(v=>v!=null&&Number.isFinite(v));return values.length?values.reduce((a,b)=>a+b,0)/values.length:null},
  maximum(){return Math.max(1,this.ceiling,this.peak||0)},
  hover(){return this.index===null?null:this.points[Math.min(this.index,this.points.length-1)]},
 },
 methods:{
  format:formatMetric,
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
.resource-chart{padding:12px;min-width:0}.resource-chart header{display:flex;align-items:baseline;justify-content:space-between;gap:8px}.resource-chart h3{font-size:14px;font-weight:650;margin:0}.resource-chart header>span{font-size:12px;color:var(--gm-muted)}.resource-legend{display:flex;gap:12px;flex-wrap:wrap;min-height:32px;align-items:center;font-size:12px;color:var(--gm-muted)}.resource-legend strong{color:var(--gm-text);font-variant-numeric:tabular-nums;margin-left:4px}.resource-legend i{display:inline-block;width:13px;border-top:2px solid var(--gm-accent);margin-right:5px}.resource-legend i.is-secondary{border-top:2px dashed var(--gm-info)}svg{display:block;width:100%;height:140px;overflow:visible}svg text{fill:var(--gm-muted);font:11px var(--gm-font)}.resource-grid{stroke:var(--gm-border);stroke-width:1}.resource-line{fill:none;stroke:var(--gm-accent);stroke-width:2;vector-effect:non-scaling-stroke}.resource-line.is-secondary{stroke:var(--gm-info);stroke-dasharray:5 3}.resource-dot{fill:var(--gm-accent)}.resource-dot.is-secondary{fill:var(--gm-info)}.resource-cursor{stroke:var(--gm-muted);stroke-dasharray:3 3}.resource-chart-values{display:flex;flex-wrap:wrap;gap:12px;min-height:20px;font-size:12px;color:var(--gm-muted);font-variant-numeric:tabular-nums}.resource-chart-empty{height:140px;display:grid;place-items:center;color:var(--gm-muted);font-size:13px}
</style>
