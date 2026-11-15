<template>
 <div class="chat-markdown" @click="copyCode">
  <div v-html="html"/>
  <span v-if="copyState" class="small text-muted" role="status">{{ copyState }}</span>
 </div>
</template>
<script>
import {renderChatMarkdown} from '../chatMarkdown.mjs';
export default {
 props:{text:{type:String,default:''}},
 data:()=>({copyState:''}),
 computed:{html(){return renderChatMarkdown(this.text)}},
 watch:{text(){this.copyState=''}},
 methods:{
  async copyCode(event){
   const button=event.target.closest('.chat-copy-code');
   if(!button||!this.$el.contains(button))return;
   const text=button.closest('.chat-code').querySelector('code').textContent;
   try{
    if(navigator.clipboard&&window.isSecureContext)await navigator.clipboard.writeText(text);
    else{
     const active=document.activeElement,el=document.createElement('textarea');
     el.value=text;el.style.cssText='position:fixed;left:-9999px;top:0';document.body.appendChild(el);
     try{el.select();if(!document.execCommand('copy'))throw Error('copy')}finally{el.remove();active?.focus({preventScroll:true})}
    }
    this.copyState='Code copied';
   }catch{this.copyState='Could not copy. Select the code and copy it manually.'}
  },
 },
};
</script>
<style>
.chat-markdown{overflow-wrap:anywhere;line-height:1.65;min-width:0;font-size:.85rem;color:var(--gm-text)}
.chat-markdown p,.chat-markdown ul,.chat-markdown ol,.chat-markdown blockquote{margin:0 0 .6rem}
.chat-markdown>div>:last-child{margin-bottom:0}
.chat-markdown blockquote{border-left:3px solid var(--gm-accent);padding-left:12px;color:inherit;color:var(--gm-muted)}
.chat-markdown h1,.chat-markdown h2,.chat-markdown h3,.chat-markdown h4,.chat-markdown h5,.chat-markdown h6{font-size:1.1rem;margin:.5rem 0}
.chat-markdown :not(pre)>code{color:inherit;background:var(--gm-panel-alt);padding:2px 5px;border-radius:3px;white-space:pre-wrap}
.chat-markdown .chat-code{margin:.5rem 0;border:1px solid var(--gm-border);border-radius:var(--gm-radius);overflow:hidden}
.chat-markdown .chat-code-toolbar{display:flex;justify-content:space-between;align-items:center;gap:12px;padding:5px 10px;background:var(--gm-panel-alt);font-size:.8rem}
.chat-markdown .chat-copy-code{background:transparent;color:inherit;border:1px solid var(--gm-border);border-radius:4px;padding:2px 8px;cursor:pointer}
.chat-markdown pre{max-height:280px;overflow:auto;padding:12px;margin:0;background:var(--gm-panel);color:inherit;line-height:1.45;white-space:pre;tab-size:4}
.chat-markdown pre code{color:inherit;overflow-wrap:normal;word-break:normal;font-size:.8rem}
.chat-markdown table{display:block;max-width:100%;overflow:auto;border-collapse:collapse;margin:.5rem 0}
.chat-markdown th,.chat-markdown td{border:1px solid var(--gm-border);padding:4px 8px}
.chat-markdown code,.chat-markdown pre{font-family:var(--gm-mono)}
.chat-markdown .chat-copy-code:hover{background:var(--gm-accent-soft);color:var(--gm-accent);border-color:var(--gm-accent)}
.chat-markdown .chat-copy-code:focus-visible{outline:2px solid var(--gm-accent);outline-offset:2px}
</style>
