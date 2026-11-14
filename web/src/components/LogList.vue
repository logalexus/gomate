<template>
 <div ref="viewport" class="log-viewport" @scroll.passive="scroll" role="region" tabindex="0" aria-label="Строки логов">
  <div :style="{height:items.length*26+'px',position:'relative',minWidth:'100%'}">
   <div :style="{transform:`translateY(${start*26}px)`}">
    <button v-for="line in visible" :key="line.id" type="button" class="log-row" :class="{'log-stderr':line.stream==='stderr'}" @click="$emit('select',line)" :title="line.text"><span class="log-time">{{ time(line.time) }}</span><span class="log-channel">{{ line.stream }}</span><span>{{ line.text || ' ' }}</span></button>
   </div>
  </div>
 </div>
</template>
<script>
export default {
 props:{items:{type:Array,required:true}},data:()=>({top:0,height:500,observer:null}),
 computed:{start(){return Math.max(0,Math.floor(this.top/26)-5)},visible(){return this.items.slice(this.start,this.start+Math.ceil(this.height/26)+10)}},
 mounted(){this.observer=new ResizeObserver(()=>this.height=this.$el.clientHeight);this.observer.observe(this.$el)},beforeDestroy(){this.observer?.disconnect()},
 watch:{items(){this.$nextTick(()=>{this.top=this.$el.scrollTop})}},
 methods:{scroll(){this.top=this.$el.scrollTop},bottom(){this.$el.scrollTop=this.$el.scrollHeight;this.scroll()},home(){this.$el.scrollTop=0;this.scroll()},time(v){return v?new Date(v).toLocaleTimeString():'—'}},
}
</script>
<style scoped>
.log-viewport{height:clamp(240px,calc(100vh - 440px),680px);min-height:220px;overflow:auto;background:var(--gm-panel);color:var(--gm-text);padding:4px 0;scrollbar-color:var(--gm-border) transparent}.log-row{display:block;height:26px;line-height:26px;padding:0 14px;border:0;border-left:2px solid transparent;background:none;color:inherit;text-align:left;white-space:pre;width:100%;overflow:hidden;text-overflow:ellipsis;cursor:pointer;font-family:var(--gm-mono);font-size:.75rem}.log-row:hover,.log-row:focus{background:var(--gm-accent-soft);border-left-color:var(--gm-accent)}.log-viewport:focus-visible{outline:2px solid var(--gm-accent);outline-offset:-2px}.log-row:focus-visible{outline:2px solid var(--gm-accent);outline-offset:-2px}.log-row span{font-family:var(--gm-mono)}.log-time{display:inline-block;width:90px;color:var(--gm-muted)}.log-channel{display:inline-block;width:65px;color:var(--gm-accent)}.log-stderr{color:var(--gm-danger)}.log-stderr .log-channel{color:var(--gm-danger)}
@media(max-width:600px){.log-row{padding:0 8px;font-size:.7rem}.log-time{width:68px}.log-channel{width:52px}.log-viewport{height:350px}}
</style>
