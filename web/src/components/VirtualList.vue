<template>
  <div ref="viewport" class="virtual-viewport" @scroll.passive="onScroll">
    <div :style="{height: total + 'px', position: 'relative'}">
      <div :style="{transform: `translateY(${offsets[start]}px)`}">
        <div v-for="(item, index) in visible" :key="item.id" :data-row-id="item.id" :data-index="start + index"><slot :item="item"/></div>
      </div>
    </div>
  </div>
</template>
<script>
export default {
  props: {items: {type: Array, required: true}},
  data: () => ({heights: {}, top: 0, height: 700, observer: null}),
  computed: {
    offsets() {const out=[0];for(const item of this.items) out.push(out[out.length-1]+(this.heights[item.id] || 136));return out;},
    total() {return this.offsets[this.items.length];},
    start() {let i=0;while(i<this.items.length && this.offsets[i+1]<this.top) i++;return Math.max(0,i-3);},
    end() {let i=this.start;while(i<this.items.length && this.offsets[i]<this.top+this.height) i++;return Math.min(this.items.length,i+3);},
    visible() {return this.items.slice(this.start,this.end);},
  },
  watch: {items() {const keep={};for(const item of this.items) if(this.heights[item.id]) keep[item.id]=this.heights[item.id];this.heights=keep;this.observeRows();}, start: 'observeRows', end: 'observeRows'},
  mounted() {this.observer=new ResizeObserver(entries=>{for(const entry of entries){if(entry.target===this.$refs.viewport){this.height=entry.contentRect.height;}else{const h=entry.target.getBoundingClientRect().height;if(h>0 && this.heights[entry.target.dataset.rowId]!==h)this.$set(this.heights,entry.target.dataset.rowId,h);}}});this.observeRows();},
  beforeDestroy() {this.observer?.disconnect();},
  methods: {
    observeRows() {this.$nextTick(()=>{if(!this.observer)return;this.observer.disconnect();this.observer.observe(this.$refs.viewport);this.$el.querySelectorAll('[data-row-id]').forEach(el=>this.observer.observe(el));});},
    onScroll() {this.top=this.$refs.viewport.scrollTop;},
    scrollToIndex(i) {this.$refs.viewport.scrollTop=this.offsets[Math.max(0,i)] || 0;this.onScroll();},
  },
};
</script>
<style scoped>.virtual-viewport {overflow-y:auto;overflow-x:hidden;height:calc(100vh - 165px);}</style>
