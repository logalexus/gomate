<template>
 <section class="chat-page">
  <header class="gm-page-header"><div><div class="gm-eyebrow">Командная работа</div><h1 class="gm-page-title">Общий чат</h1><p class="gm-page-description">Обсуждайте находки, делитесь кодом и держите команду в курсе.</p></div><b-badge :variant="error?'warning':'success'">{{ error?'Переподключение':'LIVE · каждые 2 с' }}</b-badge></header>
  <div class="chat-workspace gm-panel">
   <div class="chat-room-heading"><div class="chat-room-name"><span class="chat-room-symbol" aria-hidden="true">#</span><div><strong>Команда gomate</strong><span>Общий канал · без регистрации</span></div></div><details class="chat-room-info"><summary>О канале</summary><p>Сообщения видят все с доступом к gomate. История хранится 7 дней, до 10 000 сообщений. Анонимное имя закрепляется за этим браузером.</p></details></div>
   <b-alert :show="!!error" variant="danger" class="chat-error">{{ error }}</b-alert>
   <div class="chat-history-toolbar"><div class="chat-history-actions"><b-button size="sm" variant="outline-secondary" :disabled="loading||!hasOlder||messages.length>=500" @click="older">Предыдущие сообщения</b-button><b-button size="sm" variant="outline-primary" @click="latest">{{ browsing?'Вернуться к новым сообщениям':'К последним сообщениям' }}</b-button></div><span v-if="browsing" class="chat-history-state">Просмотр истории · LIVE приостановлен</span><span v-else-if="unseen" class="chat-history-state" role="status">Новых сообщений: {{ unseen }}</span><span v-else class="chat-history-count">{{ messages.length }} сообщений загружено</span></div>
   <p v-if="messages.length>=500&&browsing" class="chat-history-limit">Загружено 500 сообщений. Вернитесь к новым сообщениям, чтобы продолжить LIVE.</p>
   <div class="chat-timeline">
    <VirtualList ref="list" :items="messages" class="chat-list" role="region" tabindex="0" aria-label="Сообщения команды" @scroll.native.passive="onScroll"><template #default="{item}"><article class="chat-message" :class="{'chat-own':item.author===author}" :data-message-id="item.id"><span class="chat-avatar" aria-hidden="true">{{ item.author.slice(-2).toUpperCase() }}</span><div class="chat-message-body"><div class="chat-message-meta"><strong>{{ item.author }}<span v-if="item.author===author" class="chat-you">вы</span></strong><time :datetime="item.createdAt">{{ time(item.createdAt) }}</time></div><ChatMarkdown :text="item.text"/></div></article></template></VirtualList>
    <div v-if="!messages.length" class="chat-empty"><span class="chat-empty-mark" aria-hidden="true">#</span><strong>{{ loading?'Загружаем историю…':'Здесь начинается работа команды' }}</strong><p v-if="!loading">Напишите первое сообщение. Можно добавить код, ссылку или цитату — Markdown уже включён.</p></div>
   </div>
  <b-form @submit.prevent="send" class="chat-composer">
   <div class="composer-heading"><label for="chat-editor">Сообщение команде</label><span>{{ author || 'Подключение…' }}</span></div>
   <div class="chat-formatting mb-2" role="toolbar" aria-label="Форматирование сообщения">
    <b-button v-for="format in formats" :key="format.kind" type="button" size="sm" variant="outline-secondary" :disabled="sending||preview" :title="format.hint" @mousedown.prevent @click="formatDraft(format.kind)">{{ format.label }}</b-button>
    <b-button type="button" size="sm" variant="outline-primary" :aria-pressed="String(preview)" @click="preview=!preview">{{ preview?'Редактировать':'Предпросмотр' }}</b-button>
   </div>
   <div v-if="preview" class="chat-preview" aria-label="Предпросмотр сообщения"><ChatMarkdown v-if="draft.trim()" :text="draft"/><span v-else class="text-muted">Напишите сообщение, чтобы увидеть форматирование.</span></div>
   <b-form-textarea v-show="!preview" id="chat-editor" ref="editor" v-model="draft" :disabled="sending" rows="3" max-rows="6" maxlength="2000" placeholder="Сообщение команде… Markdown поддерживается" aria-label="Сообщение в чат" aria-describedby="chat-input-help" @keydown="onKey"/>
   <div class="composer-footer"><span id="chat-input-help" class="text-muted small">{{ [...draft].length }}/2000 · Enter — отправить · Shift+Enter — новая строка<br>В блоке кода Enter — новая строка · Ctrl/⌘+Enter — отправить</span><b-button type="submit" variant="primary" :disabled="sending||!author||!draft.trim()">{{ sending?'Отправка…':'Отправить' }}</b-button></div>
   <details class="chat-format-help"><summary>Как форматировать сообщение</summary><p class="mt-2 mb-1"><code>**жирный**</code> · <code>*курсив*</code> · <code>~~зачёркнутый~~</code> · <code>`код`</code> · <code>[ссылка](https://example.com)</code></p><p class="mb-1">Список: <code>- пункт</code> · Цитата: <code>&gt; текст</code>. Блок кода — три обратные кавычки на отдельных строках; после открывающих можно указать язык.</p><pre class="mb-0">```go
fmt.Println("hello")
```</pre></details>
   <p v-if="sendError" class="text-danger mt-2" role="alert">{{ sendError }} Текст сохранён, можно повторить отправку.</p>
  </b-form>
  </div>
 </section>
</template>
<script>
import VirtualList from '@/components/VirtualList';
import ChatMarkdown from '@/components/ChatMarkdown';
import {insideCodeFence} from '../chatMarkdown.mjs';
const formats=[{kind:'bold',label:'Жирный',hint:'**текст**'},{kind:'italic',label:'Курсив',hint:'*текст*'},{kind:'inline',label:'Код',hint:'Код внутри строки'},{kind:'block',label:'Блок кода',hint:'Блок с тройными обратными кавычками'},{kind:'quote',label:'Цитата',hint:'> текст'},{kind:'list',label:'Список',hint:'- пункт'}];
const message=e=>e.response?.data?.error||e.message;
const key=()=>{const b=new Uint8Array(16);crypto.getRandomValues(b);return [...b].map(x=>x.toString(16).padStart(2,'0')).join('')};
export default {
 components:{VirtualList,ChatMarkdown},data:()=>({formats,preview:false,messages:[],author:'',loading:false,sending:false,error:'',sendError:'',draft:'',pendingText:'',pendingKey:'',timer:null,controller:null,destroyed:false,hasOlder:false,browsing:false,atBottom:true,unseen:0}),
 mounted(){this.latest();this.timer=setInterval(()=>{if(!document.hidden&&!this.browsing)this.fetch()},2000);document.addEventListener('visibilitychange',this.visibility)},
 beforeDestroy(){this.destroyed=true;clearInterval(this.timer);this.controller?.abort();document.removeEventListener('visibilitychange',this.visibility)},
 methods:{
  time(t){return new Date(t).toLocaleString()},visibility(){if(document.hidden)this.controller?.abort();else if(!this.browsing)this.fetch()},
  onScroll(){const el=this.$refs.list?.$el;if(el){this.atBottom=el.scrollHeight-el.scrollTop-el.clientHeight<70;if(this.atBottom)this.unseen=0}},
  bottom(){this.$nextTick(()=>{this.$refs.list?.scrollToIndex(Math.max(0,this.messages.length-1));this.unseen=0;this.atBottom=true})},
  async fetch(mode='new'){
   if(this.loading||this.destroyed)return;this.loading=true;this.controller=new AbortController();const shouldFollow=this.atBottom;
   const params={};if(mode==='older')params.before=this.messages[0]?.id;else if(mode==='new'&&this.messages.length)params.after=this.messages[this.messages.length-1].id;
   try{const {data}=await this.$http.get('/v2/chat',{params,signal:this.controller.signal});if(this.destroyed)return;this.author=data.author;this.error='';
    if(mode==='latest'){this.messages=data.messages;this.hasOlder=data.hasMore;this.bottom()}
    else if(mode==='older'){this.messages=[...data.messages,...this.messages].slice(0,500);this.hasOlder=data.hasMore;this.$nextTick(()=>this.$refs.list?.scrollToIndex(data.messages.length))}
    else{const ids=new Set(this.messages.map(m=>m.id));const fresh=data.messages.filter(m=>!ids.has(m.id));if(!this.messages.length)this.hasOlder=data.hasMore;this.messages=[...this.messages,...fresh].slice(-500);if(fresh.length){if(shouldFollow)this.bottom();else this.unseen+=fresh.length}}
   }catch(e){if(!this.controller.signal.aborted)this.error=message(e)}finally{this.loading=false}
  },
  async latest(){if(this.loading)return;this.browsing=false;await this.fetch('latest')},async older(){if(this.loading)return;this.browsing=true;await this.fetch('older')},
  formatDraft(kind){
   const editor=this.$refs.editor.$el,start=editor.selectionStart,end=editor.selectionEnd;
   const selected=this.draft.slice(start,end),body=selected||(kind==='block'||kind==='inline'?'код':'текст');
   let before='',after='',content=body;
   if(kind==='bold'){before='**';after='**'}else if(kind==='italic'){before='*';after='*'}
   else if(kind==='inline'){const runs=body.match(/`+/g)||[];before=after='`'.repeat(Math.max(0,...runs.map(v=>v.length))+1);if(body.startsWith('`')||body.endsWith('`')){before+=' ';after=' '+after}}
   else if(kind==='block'){const runs=body.match(/`+/g)||[],fence='`'.repeat(Math.max(3,...runs.map(v=>v.length+1)));before=(start&&this.draft[start-1]!=='\n'?'\n':'')+fence+'\n';after='\n'+fence+(end<this.draft.length&&this.draft[end]!=='\n'?'\n':'')}
   else{before=start&&this.draft[start-1]!=='\n'?'\n':'';const prefix=kind==='quote'?'> ':'- ';content=body.split('\n').map(line=>prefix+line).join('\n');after=end<this.draft.length&&this.draft[end]!=='\n'?'\n':''}
   const next=this.draft.slice(0,start)+before+content+after+this.draft.slice(end);
   if([...next].length>2000){this.sendError='Форматирование превышает лимит 2000 символов.';return}
   this.draft=next;this.sendError='';this.$nextTick(()=>{editor.focus();editor.setSelectionRange(start+before.length,start+before.length+content.length)});
  },
  onKey(e){if(e.key!=='Enter'||e.isComposing||e.keyCode===229)return;
   if(e.ctrlKey||e.metaKey){e.preventDefault();this.send();return}
   if(!e.shiftKey&&!insideCodeFence(this.draft,e.target.selectionStart)){e.preventDefault();this.send()}
  },
  async send(){const text=this.draft.trim();if(!text||this.sending||!this.author)return;if(text!==this.pendingText){this.pendingText=text;this.pendingKey=key()}this.sending=true;this.sendError='';try{await this.$http.post('/v2/chat',{text,key:this.pendingKey});this.draft='';this.preview=false;this.pendingText='';this.pendingKey='';if(this.browsing){this.browsing=false;await this.fetch('latest')}else await this.fetch();this.bottom()}catch(e){this.sendError=message(e)}finally{this.sending=false}},
 }
}
</script>
<style scoped>
.chat-page{max-width:1180px;margin:auto;padding-bottom:28px}.chat-workspace{overflow:hidden}.chat-room-heading{display:flex;align-items:center;justify-content:space-between;gap:20px;padding:20px 24px;border-bottom:1px solid var(--gm-border)}.chat-room-name{display:flex;align-items:center;gap:12px;min-width:0}.chat-room-name>div{display:flex;flex-direction:column;gap:2px}.chat-room-name strong{font-size:.9rem}.chat-room-name span:not(.chat-room-symbol){font-size:.75rem;color:var(--gm-muted)}.chat-room-symbol,.chat-empty-mark{font-size:1.5rem;font-family:var(--gm-mono);font-weight:700;color:var(--gm-accent);background:var(--gm-accent-soft);border:1px solid var(--gm-border);border-radius:12px;padding:5px 14px}.chat-room-info{max-width:400px;font-size:.8rem;color:var(--gm-muted)}.chat-room-info summary{cursor:pointer;color:var(--gm-text);text-align:right}.chat-room-info p{margin:8px 0 0;line-height:1.6}.chat-error{margin:16px 20px 0}.chat-history-toolbar{padding:12px 20px;display:flex;justify-content:space-between;align-items:center;gap:12px;flex-wrap:wrap;background:var(--gm-panel-alt);border-bottom:1px solid var(--gm-border)}.chat-history-actions{display:flex;gap:8px;flex-wrap:wrap}.chat-history-count,.chat-history-state,.chat-history-limit{font-size:.75rem;color:var(--gm-muted)}.chat-history-state{color:var(--gm-accent)}.chat-history-limit{margin:0;padding:10px 20px;background:var(--gm-panel-alt)}.chat-timeline{position:relative}.chat-list{height:clamp(300px,calc(100vh - 470px),660px);min-height:300px;background:var(--gm-panel);border:0}.chat-message{display:flex;align-items:flex-start;gap:14px;padding:17px 24px;border-bottom:1px solid var(--gm-border);min-width:0}.chat-own{background:var(--gm-accent-soft)}.chat-avatar{display:grid;place-items:center;flex:0 0 32px;height:32px;border-radius:9px;background:var(--gm-panel-alt);border:1px solid var(--gm-border);color:var(--gm-muted);font-family:var(--gm-mono);font-size:.7rem;font-weight:600}.chat-own .chat-avatar{color:var(--gm-accent);border-color:var(--gm-accent)}.chat-message-body{min-width:0;flex:1}.chat-message-meta{display:flex;gap:10px;align-items:center;justify-content:space-between;flex-wrap:wrap;margin-bottom:6px}.chat-message-meta strong{font-size:.8rem;display:flex;gap:8px;align-items:center}.chat-message-meta time{font-size:.7rem;color:var(--gm-muted);font-variant-numeric:tabular-nums}.chat-you{font-size:.65rem;line-height:1.5;padding:0 5px;border-radius:4px;border:1px solid var(--gm-border);color:var(--gm-accent);font-weight:500}.chat-empty{position:absolute;inset:0;display:flex;justify-content:center;align-items:center;flex-direction:column;gap:14px;padding:24px;pointer-events:none}.chat-empty strong{font-size:1rem}.chat-empty p{max-width:420px;text-align:center;font-size:.85rem;color:var(--gm-muted);line-height:1.7;margin:0}.chat-empty-mark{font-size:2rem;padding:8px 20px}.chat-composer{padding:20px 24px;border-top:1px solid var(--gm-border);background:var(--gm-panel-alt)}.composer-heading{display:flex;justify-content:space-between;gap:12px;flex-wrap:wrap;margin-bottom:12px}.composer-heading label{font-size:.85rem;font-weight:600;margin:0}.composer-heading>span{font-size:.75rem;color:var(--gm-muted)}.chat-formatting{display:flex;flex-wrap:wrap;gap:6px}.chat-formatting .btn{font-size:.75rem}.chat-formatting .btn:last-child{margin-left:auto}.chat-preview{min-height:100px;max-height:260px;overflow:auto;padding:14px;border:1px solid var(--gm-border);border-radius:var(--gm-radius);background:var(--gm-panel)}.composer-footer{display:flex;justify-content:space-between;align-items:center;gap:12px;margin-top:12px}.composer-footer>span{font-size:.7rem;line-height:1.7}.composer-footer>.btn{min-width:110px;flex-shrink:0}.chat-format-help{font-size:.75rem;color:var(--gm-muted);margin-top:12px}.chat-format-help summary{cursor:pointer}.chat-format-help pre{font-family:var(--gm-mono);color:var(--gm-text)}
@media(max-width:600px){.chat-room-heading{padding:16px;align-items:flex-start;flex-wrap:wrap;gap:12px}.chat-room-info{max-width:none;width:100%}.chat-room-info summary{text-align:left}.chat-history-toolbar{padding:12px}.chat-history-actions{width:100%;display:grid;grid-template-columns:1fr 1fr;gap:6px}.chat-history-actions .btn{font-size:.7rem;padding:7px;white-space:normal}.chat-message{padding:14px 12px;gap:9px}.chat-avatar{flex-basis:25px;height:25px;font-size:.6rem;border-radius:7px}.chat-message-meta{gap:4px}.chat-message-meta strong{font-size:.75rem}.chat-message-meta time{font-size:.65rem}.chat-composer{padding:14px 12px}.composer-footer{align-items:flex-start;flex-wrap:wrap}.composer-footer .btn{margin-left:auto}.chat-formatting .btn:last-child{margin-left:0}.chat-list{height:340px}.chat-empty{padding:20px}}
/* Keep the editor anchored while only the message timeline scrolls. */
.chat-page{height:100%;min-height:0;display:flex;flex-direction:column;padding-bottom:0}
.chat-page>.gm-page-header{flex:none;margin-bottom:16px}
.chat-workspace{padding:0;flex:1;min-height:0;display:flex;flex-direction:column}
.chat-room-heading,.chat-history-toolbar,.chat-history-limit,.chat-error,.chat-composer{flex:none}
.chat-room-heading{padding:12px 20px}
.chat-history-toolbar{padding:8px 16px}
.chat-timeline{flex:1;min-height:120px}
.chat-list{height:100%;min-height:0}
.chat-empty{overflow:auto;gap:8px;padding:12px}
.chat-empty-mark{font-size:1.25rem;padding:3px 12px}
.chat-empty p{font-size:.8rem;line-height:1.5}
.chat-composer{padding:12px 20px}
.composer-heading{margin-bottom:8px}
.chat-composer #chat-editor{max-height:100px;overflow-y:auto!important}
.chat-preview{min-height:72px;max-height:100px}
.composer-footer{margin-top:8px}
.chat-format-help{margin-top:8px}
.chat-format-help[open]{max-height:100px;overflow:auto}
@media(max-width:800px),(max-height:700px){.chat-page{height:auto;min-height:100%}.chat-workspace{flex:none}.chat-timeline{flex:none;height:260px;min-height:260px}.chat-list{height:100%;min-height:0}.chat-composer{padding:14px 12px}}
</style>
