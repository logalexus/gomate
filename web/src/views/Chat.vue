<template>
 <section class="chat-page" aria-label="Общий чат">
  <header class="chat-room-heading">
   <div class="chat-room-name"><span class="chat-room-symbol" aria-hidden="true">#</span><h1 class="gm-page-title">общий</h1></div>
   <div class="chat-room-actions">
    <b-button class="chat-game-button" size="sm" variant="outline-secondary" aria-label="Открыть мини-шутер" @click="arenaOpen=true"><i class="fas fa-gamepad" aria-hidden="true"/><span class="chat-game-label"> Арена</span></b-button>
    <b-button class="chat-history-button" size="sm" variant="link" :disabled="loading||!hasOlder||messages.length>=500" @click="older"><i class="fas fa-history" aria-hidden="true"/> История</b-button>
    <b-dropdown right no-caret variant="link" class="chat-info-menu" :toggle-attrs="{'aria-label':'О канале'}">
     <template #button-content><i class="fas fa-info-circle" aria-hidden="true"/></template>
     <b-dropdown-text><strong>{{ author || 'Подключение…' }}</strong><p>Ваше анонимное имя в этом браузере.</p><p>Общий канал для всех с доступом к gomate. История хранится 7 дней, до 10 000 сообщений.</p></b-dropdown-text>
    </b-dropdown>
   </div>
  </header>
  <b-alert :show="!!error" variant="danger" class="chat-error">{{ error }}</b-alert>
  <div v-if="browsing" class="chat-history-banner" role="status"><span>{{ messages.length>=500?'Загружено 500 сообщений':'Просмотр истории' }}</span><b-button size="sm" variant="link" :disabled="loading" @click="latest">К новым сообщениям <i class="fas fa-arrow-down" aria-hidden="true"/></b-button></div>
  <div class="chat-timeline">
   <VirtualList ref="list" :items="messages" class="chat-list" role="region" tabindex="0" aria-label="Сообщения команды" @scroll.native.passive="onScroll">
    <template #default="{item}"><article class="chat-message" :class="{'chat-own':item.author===author}" :data-message-id="item.id">
     <span class="chat-avatar" aria-hidden="true">{{ item.author.slice(-2).toUpperCase() }}</span>
     <div class="chat-message-body"><div class="chat-message-meta"><strong>{{ item.author }}<span v-if="item.author===author" class="chat-you">вы</span></strong><time :datetime="item.createdAt" :title="new Date(item.createdAt).toLocaleString()">{{ time(item.createdAt) }}</time></div><ChatMarkdown :text="item.text"/></div>
    </article></template>
   </VirtualList>
   <div v-if="!messages.length" class="chat-empty"><span class="chat-empty-mark" aria-hidden="true">#</span><strong>{{ loading?'Загружаем историю…':'Начало #общий' }}</strong></div>
   <b-button v-if="!browsing&&!atBottom" class="chat-jump" size="sm" variant="primary" @click="bottom"><span role="status">{{ unseen?'Новых сообщений: '+unseen:'К последним сообщениям' }}</span> <i class="fas fa-arrow-down" aria-hidden="true"/></b-button>
  </div>
  <b-form @submit.prevent="send" class="chat-composer">
   <div class="chat-input-shell">
    <b-dropdown dropup no-caret variant="link" class="chat-format-menu" :toggle-attrs="{'aria-label':'Форматирование сообщения'}">
     <template #button-content><i class="fas fa-plus" aria-hidden="true"/></template>
     <b-dropdown-item-button v-for="format in formats" :key="format.kind" :disabled="sending||preview" @mousedown.native.prevent @click="formatDraft(format.kind)">{{ format.label }}</b-dropdown-item-button>
     <b-dropdown-divider/>
     <b-dropdown-item-button @click="togglePreview">{{ preview?'Редактировать':'Предпросмотр' }}</b-dropdown-item-button>
    </b-dropdown>
    <label class="sr-only" for="chat-editor">Сообщение в чат</label>
    <div v-if="preview" class="chat-preview" aria-label="Предпросмотр сообщения"><ChatMarkdown v-if="draft.trim()" :text="draft"/><span v-else class="text-muted">Пустое сообщение</span><b-button type="button" size="sm" variant="link" @click="togglePreview">Редактировать</b-button></div>
    <textarea v-show="!preview" id="chat-editor" ref="editor" v-model="draft" :disabled="sending" rows="1" maxlength="2000" placeholder="Написать в #общий" aria-label="Сообщение в чат" @keydown="onKey"></textarea>
    <b-button class="chat-send" type="submit" variant="link" :disabled="sending||!author||!draft.trim()" :aria-label="sending?'Отправка…':'Отправить'"><i :class="sending?'fas fa-circle-notch fa-spin':'fas fa-paper-plane'" aria-hidden="true"/></b-button>
   </div>
   <p v-if="sendError" class="chat-send-error text-danger" role="alert">{{ sendError }} Текст сохранён, можно повторить отправку.</p>
  </b-form>
  <ArenaGame v-if="arenaOpen" @close="arenaOpen=false"/>
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
 components:{VirtualList,ChatMarkdown,ArenaGame:()=>import('@/components/ArenaGame.vue')},data:()=>({arenaOpen:false,formats,preview:false,messages:[],author:'',loading:false,sending:false,error:'',sendError:'',draft:'',pendingText:'',pendingKey:'',timer:null,controller:null,destroyed:false,hasOlder:false,browsing:false,atBottom:true,unseen:0}),
 watch:{draft(){this.$nextTick(this.resizeEditor)}},
 mounted(){window.addEventListener('resize',this.resizeEditor);this.resizeEditor();this.latest();this.timer=setInterval(()=>{if(!document.hidden&&!this.browsing)this.fetch()},2000);document.addEventListener('visibilitychange',this.visibility)},
 beforeDestroy(){window.removeEventListener('resize',this.resizeEditor);this.destroyed=true;clearInterval(this.timer);this.controller?.abort();document.removeEventListener('visibilitychange',this.visibility)},
 methods:{
  resizeEditor(){const editor=this.$refs.editor;if(editor&&!this.preview){editor.style.height='auto';editor.style.height=editor.scrollHeight+'px'}},
  time(t){const date=new Date(t);return date.toLocaleDateString()===new Date().toLocaleDateString()?date.toLocaleTimeString([], {hour:'2-digit',minute:'2-digit'}):date.toLocaleString([], {day:'numeric',month:'short',hour:'2-digit',minute:'2-digit'})},
  togglePreview(){this.preview=!this.preview;if(!this.preview)this.$nextTick(()=>{this.resizeEditor();this.$refs.editor?.focus()})},visibility(){if(document.hidden)this.controller?.abort();else if(!this.browsing)this.fetch()},
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
   const editor=this.$refs.editor,start=editor.selectionStart,end=editor.selectionEnd;
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
  async send(){const text=this.draft.trim();if(!text||this.sending||!this.author)return;if(text!==this.pendingText){this.pendingText=text;this.pendingKey=key()}this.sending=true;this.sendError='';try{await this.$http.post('/v2/chat',{text,key:this.pendingKey});this.draft='';this.preview=false;this.pendingText='';this.pendingKey='';if(this.browsing){this.browsing=false;await this.fetch('latest')}else await this.fetch();this.bottom()}catch(e){this.sendError=message(e)}finally{this.sending=false;this.$nextTick(()=>this.$refs.editor?.focus())}},
 }
}
</script>
<style scoped>
.chat-page{height:100%;min-height:0;display:flex;flex-direction:column;background:var(--gm-panel)}
.chat-room-heading{display:flex;flex:none;align-items:center;justify-content:space-between;gap:12px;min-height:52px;padding:4px 20px;border-bottom:1px solid var(--gm-border)}
.chat-room-name,.chat-room-actions{display:flex;align-items:center;gap:12px;min-width:0}
.chat-room-symbol{font-size:26px;color:var(--gm-muted);font-weight:500}
.chat-room-name .gm-page-title{font-size:16px;margin:0;font-weight:700;letter-spacing:0}
.chat-room-actions{gap:4px}.chat-history-button{font-size:12px;color:var(--gm-muted)}
.chat-info-menu ::v-deep .dropdown-toggle{width:40px;height:40px;color:var(--gm-muted)}
.chat-info-menu ::v-deep .dropdown-menu{width:280px;max-width:calc(100vw - 24px);font-size:13px}
.chat-info-menu p{margin:6px 0;color:var(--gm-muted);line-height:1.5}
.chat-error{flex:none;margin:8px 16px;padding:8px 12px;font-size:13px}
.chat-history-banner{flex:none;display:flex;align-items:center;justify-content:space-between;gap:8px;padding:4px 16px;background:var(--gm-accent-soft);font-size:12px}.chat-history-banner .btn{font-size:12px}
.chat-timeline{position:relative;flex:1;min-height:0}
.chat-list{height:100%;min-height:0;scrollbar-gutter:stable}
.chat-message{display:flex;align-items:flex-start;gap:14px;padding:9px 24px;min-width:0}
.chat-message:hover{background:var(--gm-panel-alt)}
.chat-avatar{display:grid;place-items:center;flex:0 0 36px;height:36px;border-radius:50%;background:var(--gm-panel-alt);color:var(--gm-muted);font-family:var(--gm-mono);font-size:12px;font-weight:600}
.chat-own .chat-avatar{color:var(--gm-accent);background:var(--gm-accent-soft)}
.chat-message-body{min-width:0;flex:1}.chat-message-meta{display:flex;gap:10px;align-items:baseline;flex-wrap:wrap;margin-bottom:2px}
.chat-message-meta strong{font-size:13px;display:flex;gap:8px;align-items:center}.chat-message-meta time{font-size:11px;color:var(--gm-muted);font-variant-numeric:tabular-nums}.chat-you{font-size:10px;color:var(--gm-muted);font-weight:400}
.chat-message ::v-deep .chat-markdown{font-size:14px;line-height:1.5}
.chat-empty{position:absolute;inset:0;display:flex;justify-content:flex-end;align-items:flex-start;flex-direction:column;gap:12px;padding:28px;pointer-events:none;overflow:hidden}
.chat-empty-mark{display:grid;place-items:center;width:64px;height:64px;flex:none;border-radius:50%;font-size:40px;background:var(--gm-panel-alt);color:var(--gm-muted)}.chat-empty strong{font-size:22px}
.chat-jump{position:absolute;bottom:8px;left:50%;transform:translateX(-50%);border-radius:20px;max-width:calc(100% - 32px);font-size:12px;white-space:normal;box-shadow:0 2px 8px #0002}
.chat-composer{flex:none;padding:8px 20px 16px}
.chat-input-shell{display:flex;align-items:flex-end;gap:4px;border:1px solid var(--gm-border);border-radius:10px;padding:3px;background:var(--gm-panel-alt)}
.chat-input-shell:focus-within{border-color:var(--gm-accent);outline:2px solid var(--gm-accent);outline-offset:1px}
.chat-format-menu,.chat-send{flex:none}.chat-format-menu ::v-deep .dropdown-toggle,.chat-send{display:grid;place-items:center;width:40px;height:40px;padding:0;color:var(--gm-muted);border-radius:7px}
.chat-format-menu ::v-deep .dropdown-menu{min-width:190px;max-height:min(320px,55dvh);overflow-y:auto;font-size:13px}
.chat-format-menu ::v-deep .dropdown-item{padding:8px 16px}
.chat-composer #chat-editor{flex:1;min-width:0;border:0;box-shadow:none;background:transparent;color:var(--gm-text);font-size:14px;line-height:22px;padding:9px 4px;min-height:40px;max-height:128px;overflow-y:auto!important;resize:none}
.chat-composer #chat-editor:focus-visible{outline:none!important}
.chat-composer #chat-editor::placeholder{color:var(--gm-muted)}
.chat-send:not(:disabled){color:var(--gm-accent)}.chat-send:disabled{opacity:.4}
.chat-preview{flex:1;min-width:0;padding:8px 4px;max-height:160px;overflow:auto;font-size:14px}.chat-preview>.btn{font-size:12px;padding:4px 0}
.chat-send-error{font-size:12px;margin:6px 4px 0;max-height:70px;overflow:auto}
@media(max-width:600px){.chat-room-heading{padding:4px 12px}.chat-message{padding:8px 12px;gap:10px}.chat-avatar{flex-basis:32px;height:32px}.chat-message-meta{gap:6px}.chat-message-meta strong{font-size:12px}.chat-composer{padding:6px 10px 10px}.chat-format-menu ::v-deep .dropdown-toggle,.chat-send{width:44px;height:44px}.chat-composer #chat-editor{padding-top:11px;padding-bottom:11px;min-height:44px}.chat-history-banner{padding:4px 12px}}
</style>
