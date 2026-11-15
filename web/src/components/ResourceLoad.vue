<template>
 <div class="resource-load" :class="'gm-load-'+load.level" :title="description">
  <div class="resource-load-caption"><span><i aria-hidden="true"/>{{ load.label }}</span><strong v-if="load.percent!==null">{{ format(load.percent,'percent') }}<span v-if="basis"> {{ basis }}</span></strong></div>
  <div v-if="load.percent!==null" class="resource-load-track" role="meter" :aria-label="label" aria-valuemin="0" aria-valuemax="100" :aria-valuenow="load.width" :aria-valuetext="description"><span :style="{width:load.width+'%'}"/></div>
  <div v-else class="resource-load-track is-unknown" aria-hidden="true"/>
 </div>
</template>
<script>
import {formatMetric,resourceLoad} from '../monitor.mjs';
export default {
 name:'ResourceLoad',
 props:{value:Number,capacity:Number,label:{type:String,required:true},basis:{type:String,default:''}},
 computed:{
  load(){return resourceLoad(this.value,this.capacity)},
  description(){return `${this.label}: ${this.load.label}${this.load.percent===null?'':' · '+formatMetric(this.load.percent,'percent')+(this.basis?' '+this.basis:'')}. Elevated ≥ 70%, high ≥ 90%.`},
 },
 methods:{format:formatMetric},
};
</script>
<style scoped>
.resource-load{min-width:110px;color:var(--gm-load-color);font-size:11px;font-variant-numeric:tabular-nums}.resource-load-caption{display:flex;justify-content:space-between;gap:8px;align-items:baseline;margin-bottom:5px}.resource-load-caption>span{display:inline-flex;align-items:center;gap:5px}.resource-load-caption i{width:6px;height:6px;border-radius:50%;background:currentColor;flex-shrink:0}.resource-load-caption strong{font-size:11px;font-weight:600;white-space:nowrap}.resource-load-caption strong span{font-weight:400}.resource-load-track{height:4px;border-radius:3px;overflow:hidden;background:var(--gm-border)}.resource-load-track>span{display:block;height:100%;border-radius:inherit;background:var(--gm-load-color)}.resource-load-track.is-unknown{background:repeating-linear-gradient(110deg,var(--gm-border) 0 4px,transparent 4px 8px)}
</style>
