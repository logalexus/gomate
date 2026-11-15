<template>
 <span v-if="patterns.length" class="pattern-labels" :class="{compact}">
  <span v-for="p in visible" :key="p.id" class="pattern-label" :title="p.label"><i aria-hidden="true" :style="{backgroundColor: color(p.color)}"/><span>{{ p.label }}</span></span>
  <span v-if="hidden.length" class="pattern-overflow" :title="hidden.map(p=>p.label).join(', ')">+{{ hidden.length }}</span>
 </span>
</template>

<script>
export default {
 props: {ids: {type: Array, default: () => []}, compact: Boolean},
 computed: {
  patterns() {
   const catalog = new Map(this.$store.state.patterns.map(p => [p.id, p]));
   return [...new Set(this.ids || [])].map(id => {
    const p = catalog.get(id);
    return {id, color: p?.color, label: (p?.name || `Pattern #${id}`) + (p?.deleted ? ' (deleted)' : '')};
   });
  },
  visible() { return this.compact ? this.patterns.slice(0, 3) : this.patterns; },
  hidden() { return this.compact ? this.patterns.slice(3) : []; },
 },
 methods: {color(value) { return /^#[0-9a-f]{6}$/i.test(value || '') ? value : 'var(--gm-muted)'; }},
};
</script>

<style scoped>
.pattern-labels{display:flex;flex-wrap:wrap;align-items:center;gap:3px 5px;min-width:0;max-width:100%}
.pattern-label{display:inline-flex;align-items:center;gap:4px;min-width:0;max-width:100%;border:1px solid var(--gm-border);border-radius:4px;padding:0 5px;font-size:11px;line-height:17px;color:var(--gm-text);background:var(--gm-panel-alt)}
.pattern-label>span{overflow-wrap:anywhere;min-width:0}.pattern-label i{width:6px;height:6px;border-radius:50%;flex-shrink:0}
.compact{margin-top:4px}.compact .pattern-label{max-width:150px}.compact .pattern-label>span{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.pattern-overflow{font-size:11px;color:var(--gm-muted)}
</style>
