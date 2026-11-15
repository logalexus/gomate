<template>
 <span class="behavior-feature" :class="[{compact,variable},'feature-'+kind.tone]" :title="field">
  <span class="feature-kind">{{ kind.label }}</span>
  <span class="feature-name">{{ name }}</span>
  <code v-if="hasValue" class="feature-value">{{ value || '(empty)' }}</code>
  <small v-if="message && !compact" class="feature-message">message {{ message }}</small>
 </span>
</template>
<script>
export default {
 name:'BehaviorFeature',
 props:{field:{type:String,required:true},compact:Boolean,variable:Boolean},
 computed:{
  parts(){const offset=this.field.indexOf(' = ');return {key:offset<0?this.field:this.field.slice(0,offset),value:offset<0?'':this.field.slice(offset+3)}},
  message(){return this.parts.key.match(/^(\d+):/)?.[1]||''},
  key(){return this.parts.key.replace(/^\d+:[^/]+\//,'')},
  kind(){
   if(this.variable)return {label:'Variable',tone:'variable'};
   if(this.key==='json.structure'||this.key==='separators')return {label:'Structure',tone:'structure'};
   if(this.key.startsWith('json.'))return {label:'JSON',tone:'payload'};
   if(this.key.startsWith('form.'))return {label:'Shape',tone:'payload'};
   if(this.key.startsWith('query.'))return {label:'Parameter',tone:'payload'};
   if(this.key.startsWith('cookie.'))return {label:'Cookie',tone:'meta'};
   if(this.key.startsWith('header.'))return {label:'Header',tone:'meta'};
   if(this.key.startsWith('token.'))return {label:'TCP',tone:'payload'};
   return {label:/^body/.test(this.key)?'Body':'Request',tone:'meta'};
  },
  name(){if(this.variable)return this.key;return ({'json.structure':'JSON','method':'Method','path':'Path','separators':'Delimiters','body':'Content'})[this.key]||this.key.replace(/^(json|form|query|cookie|header|token)\./,'')},
  hasValue(){return this.field.includes(' = ')},
  value(){return this.parts.value},
 },
};
</script>
<style scoped>
.behavior-feature{--feature-color:var(--gm-muted);--feature-bg:var(--gm-panel-alt);display:flex;align-items:baseline;gap:5px 8px;flex-wrap:wrap;min-width:0;font-size:12px;overflow-wrap:anywhere}
.feature-value{color:var(--gm-text);font-size:12px;white-space:pre-wrap;min-width:0;flex:1 1 160px;font-family:var(--gm-mono)}
.feature-name{font-family:var(--gm-mono);color:var(--gm-text);min-width:0;overflow-wrap:anywhere}
.feature-kind{font-size:11px;font-weight:600;color:var(--feature-color);background:var(--feature-bg);border-radius:4px;padding:1px 5px;flex-shrink:0}
.feature-message{color:var(--gm-muted);font-size:11px;margin-left:auto;white-space:nowrap}
.feature-payload{--feature-color:var(--gm-accent);--feature-bg:var(--gm-accent-soft)}
.feature-structure{--feature-color:var(--gm-info);--feature-bg:var(--gm-info-soft)}
.feature-variable{--feature-color:var(--gm-warning);--feature-bg:var(--gm-warning-soft)}
.compact{display:inline-flex;flex-wrap:nowrap;max-width:100%;vertical-align:middle;gap:5px}
.compact .feature-name{flex-shrink:0;max-width:120px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.compact .feature-value{flex:0 1 auto;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.variable .feature-name{color:var(--gm-warning)}
</style>
