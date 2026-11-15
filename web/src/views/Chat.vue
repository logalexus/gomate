<template>
 <section class="chat-page" aria-label="Team chat">
  <header class="chat-room-heading">
   <div class="chat-room-name"><span class="chat-room-symbol" aria-hidden="true">#</span><h1 class="gm-page-title">general</h1></div>
   <div class="chat-room-actions">
    <b-button class="chat-game-button" size="sm" variant="outline-secondary" aria-label="Open mini shooter" @click="arenaOpen=true"><i class="fas fa-gamepad" aria-hidden="true"/><span class="chat-game-label"> Arena</span></b-button>
    <b-button class="chat-history-button" size="sm" variant="link" :disabled="loading||!hasOlder||messages.length>=500" @click="older"><i class="fas fa-history" aria-hidden="true"/> History</b-button>
    <b-dropdown right no-caret variant="link" class="chat-info-menu" :toggle-attrs="{'aria-label':'About this channel'}">
     <template #button-content><i class="fas fa-info-circle" aria-hidden="true"/></template>
     <b-dropdown-text><strong>{{ author || 'Connecting…' }}</strong><p>Your anonymous name in this browser.</p><p>A shared channel for everyone with access to gomate. History is kept for 7 days, up to 10,000 messages.</p><p>Everyone can edit or delete any message.</p></b-dropdown-text>
    </b-dropdown>
   </div>
  </header>
  <b-alert :show="!!error" variant="danger" class="chat-error">{{ error }}</b-alert>
  <div v-if="browsing" class="chat-history-banner" role="status"><span>{{ messages.length>=500?'500 messages loaded':'Browsing history' }}</span><b-button size="sm" variant="link" :disabled="loading" @click="latest">Jump to new messages <i class="fas fa-arrow-down" aria-hidden="true"/></b-button></div>
  <div class="chat-timeline">
   <VirtualList ref="list" :items="messages" class="chat-list" role="region" tabindex="0" aria-label="Team messages" @scroll.native.passive="onScroll">
    <template #default="{item}"><article class="chat-message" :class="{'chat-own':item.author===author}" :data-message-id="item.id">
     <span class="chat-avatar" aria-hidden="true">{{ item.author.slice(-2).toUpperCase() }}</span>
     <div class="chat-message-body"><div class="chat-message-meta"><strong>{{ item.author }}<span v-if="item.author===author" class="chat-you">you</span></strong><time :datetime="item.createdAt" :title="new Date(item.createdAt).toLocaleString('en-US')">{{ time(item.createdAt) }}</time><span v-if="item.editedAt" class="chat-edited" :title="'Edited '+new Date(item.editedAt).toLocaleString('en-US')">(edited)</span></div>
      <div v-if="editingId===item.id" class="chat-inline-editor">
       <textarea :id="'edit-message-'+item.id" v-model="editDraft" rows="3" maxlength="2000" aria-label="Edit message" :aria-describedby="editError?'chat-edit-error':null" :disabled="mutating" @keydown="editKey"/>
       <p v-if="editError" id="chat-edit-error" class="text-danger chat-edit-error" role="alert">{{ editError }}</p>
       <div class="chat-edit-actions"><b-button size="sm" variant="outline-secondary" :disabled="mutating" @click="cancelEdit">Cancel</b-button><b-button size="sm" variant="primary" :disabled="mutating||!editDraft.trim()" @click="saveEdit">{{ mutating?'Saving…':'Save' }}</b-button></div>
      </div>
      <ChatMarkdown v-else :text="item.text"/>
     </div>
     <b-dropdown right no-caret boundary="viewport" variant="link" class="chat-message-menu" :disabled="mutating" :toggle-attrs="{'aria-label':'Message actions'}">
      <template #button-content><i class="fas fa-ellipsis-h" aria-hidden="true"/></template>
      <b-dropdown-item-button @click="startEdit(item)"><i class="fas fa-pen mr-2" aria-hidden="true"/>Edit</b-dropdown-item-button>
      <b-dropdown-item-button variant="danger" @click="deleteMessage(item)"><i class="fas fa-trash mr-2" aria-hidden="true"/>Delete</b-dropdown-item-button>
     </b-dropdown>
    </article></template>
   </VirtualList>
   <div v-if="!messages.length" class="chat-empty"><span class="chat-empty-mark" aria-hidden="true">#</span><strong>{{ loading?'Loading history…':'Start of #general' }}</strong></div>
   <b-button v-if="!browsing&&!atBottom" class="chat-jump" size="sm" variant="primary" @click="bottom"><span role="status">{{ unseen?'New messages: '+unseen:'Jump to latest messages' }}</span> <i class="fas fa-arrow-down" aria-hidden="true"/></b-button>
  </div>
  <b-form @submit.prevent="send" class="chat-composer">
   <div class="chat-input-shell">
    <b-dropdown dropup no-caret variant="link" class="chat-format-menu" :toggle-attrs="{'aria-label':'Message formatting'}">
     <template #button-content><i class="fas fa-plus" aria-hidden="true"/></template>
     <b-dropdown-item-button v-for="format in formats" :key="format.kind" :disabled="sending||preview" @mousedown.native.prevent @click="formatDraft(format.kind)">{{ format.label }}</b-dropdown-item-button>
     <b-dropdown-divider/>
     <b-dropdown-item-button @click="togglePreview">{{ preview?'Edit':'Preview' }}</b-dropdown-item-button>
    </b-dropdown>
    <label class="sr-only" for="chat-editor">Chat message</label>
    <div v-if="preview" class="chat-preview" aria-label="Message preview"><ChatMarkdown v-if="draft.trim()" :text="draft"/><span v-else class="text-muted">Empty message</span><b-button type="button" size="sm" variant="link" @click="togglePreview">Edit</b-button></div>
    <textarea v-show="!preview" id="chat-editor" ref="editor" v-model="draft" :disabled="sending" rows="1" maxlength="2000" placeholder="Message #general" aria-label="Chat message" @keydown="onKey"></textarea>
    <b-button class="chat-send" type="submit" variant="link" :disabled="sending||!author||!draft.trim()" :aria-label="sending?'Sending…':'Send'"><i :class="sending?'fas fa-circle-notch fa-spin':'fas fa-paper-plane'" aria-hidden="true"/></b-button>
   </div>
   <p v-if="sendError" class="chat-send-error text-danger" role="alert">{{ sendError }} Your text is saved. Try sending again.</p>
  </b-form>
  <ArenaGame v-if="arenaOpen" @close="arenaOpen=false"/>
 </section>
</template>
<script>
import VirtualList from '@/components/VirtualList';
import ChatMarkdown from '@/components/ChatMarkdown';
import {mergeChatMessages} from '../chatMessages.mjs';
import {insideCodeFence} from '../chatMarkdown.mjs';
const formats=[{kind:'bold',label:'Bold',hint:'**text**'},{kind:'italic',label:'Italic',hint:'*text*'},{kind:'inline',label:'Code',hint:'Inline code'},{kind:'block',label:'Code block',hint:'Block with triple backticks'},{kind:'quote',label:'Quote',hint:'> text'},{kind:'list',label:'List',hint:'- item'}];
const message=e=>e.response?.data?.error||e.message;
const key=()=>{const b=new Uint8Array(16);crypto.getRandomValues(b);return [...b].map(x=>x.toString(16).padStart(2,'0')).join('')};
export default {
 components:{VirtualList,ChatMarkdown,ArenaGame:()=>import('@/components/ArenaGame.vue')},data:()=>({revision:null,versions:new Map(),editingId:null,editDraft:'',editRevision:0,editError:'',mutating:false,arenaOpen:false,formats,preview:false,messages:[],author:'',loading:false,sending:false,error:'',sendError:'',draft:'',pendingText:'',pendingKey:'',timer:null,controller:null,destroyed:false,hasOlder:false,browsing:false,atBottom:true,unseen:0}),
 watch:{draft(){this.$nextTick(this.resizeEditor)}},
 mounted(){window.addEventListener('resize',this.resizeEditor);this.resizeEditor();this.latest();this.timer=setInterval(()=>{if(!document.hidden)this.fetch(this.browsing?'changes':'new')},2000);document.addEventListener('visibilitychange',this.visibility)},
 beforeDestroy(){window.removeEventListener('resize',this.resizeEditor);this.destroyed=true;clearInterval(this.timer);this.controller?.abort();document.removeEventListener('visibilitychange',this.visibility)},
 methods:{
  applyChanges(changes){
   this.messages=mergeChatMessages(this.messages,changes,this.versions,{insert:false});
   if(this.editingId&&!this.messages.some(m=>m.id===this.editingId)){this.editingId=null;this.editDraft='';this.editError='';this.error='The message being edited was deleted.'}
  },
  startEdit(item){if(this.mutating)return;this.editingId=item.id;this.editDraft=item.text;this.editRevision=item.revision||0;this.editError='';this.$nextTick(()=>{const el=document.getElementById('edit-message-'+item.id);el?.focus();el?.setSelectionRange(el.value.length,el.value.length)})},
  cancelEdit(){if(this.mutating)return;this.editingId=null;this.editDraft='';this.editError=''},
  editKey(e){if(e.isComposing||e.keyCode===229)return;if(e.key==='Escape'){e.preventDefault();this.cancelEdit()}else if(e.key==='Enter'&&(e.ctrlKey||e.metaKey)){e.preventDefault();this.saveEdit()}},
  async saveEdit(){
   const text=this.editDraft.trim();if(!this.editingId||!text||this.mutating)return;
   this.mutating=true;this.editError='';
   try{const {data}=await this.$http.patch('/v2/chat/'+this.editingId,{text,revision:this.editRevision});this.applyChanges([data]);this.mutating=false;this.cancelEdit()}
   catch(e){this.editError=message(e)}finally{this.mutating=false}
  },
  async deleteMessage(item){
   if(this.mutating)return;
   const confirmed=await this.$bvModal.msgBoxConfirm('Delete this message for everyone?',{title:'Delete message',okTitle:'Delete',okVariant:'danger',cancelTitle:'Cancel',centered:true});
   if(!confirmed||this.destroyed||this.mutating)return;
   this.mutating=true;this.error='';
   try{const {data}=await this.$http.delete('/v2/chat/'+item.id);if(this.editingId===item.id){this.mutating=false;this.cancelEdit()}this.applyChanges([data])}
   catch(e){this.error=message(e)}finally{this.mutating=false}
  },
  resizeEditor(){const editor=this.$refs.editor;if(editor&&!this.preview){editor.style.height='auto';editor.style.height=editor.scrollHeight+'px'}},
  time(t){const date=new Date(t);return date.toLocaleDateString('en-US')===new Date().toLocaleDateString('en-US')?date.toLocaleTimeString('en-US', {hour:'2-digit',minute:'2-digit'}):date.toLocaleString('en-US', {day:'numeric',month:'short',hour:'2-digit',minute:'2-digit'})},
  togglePreview(){this.preview=!this.preview;if(!this.preview)this.$nextTick(()=>{this.resizeEditor();this.$refs.editor?.focus()})},visibility(){if(document.hidden)this.controller?.abort();else this.fetch(this.browsing?'changes':'new')},
  onScroll(){const el=this.$refs.list?.$el;if(el){this.atBottom=el.scrollHeight-el.scrollTop-el.clientHeight<70;if(this.atBottom)this.unseen=0}},
  bottom(){this.$nextTick(()=>{this.$refs.list?.scrollToIndex(Math.max(0,this.messages.length-1));this.unseen=0;this.atBottom=true})},
  async fetch(mode='new'){
   if(this.loading||this.destroyed)return;this.loading=true;this.controller=new AbortController();const shouldFollow=this.atBottom;
   const params={};if(this.revision!==null)params.revision=this.revision;if(mode==='changes')params.changesOnly=1;if(mode==='older')params.before=this.messages[0]?.id;else if(mode==='new'&&this.messages.length)params.after=this.messages[this.messages.length-1].id;
   try{const {data}=await this.$http.get('/v2/chat',{params,signal:this.controller.signal});if(this.destroyed)return;this.author=data.author;this.error='';
    if(mode==='latest'){this.messages=mergeChatMessages(this.messages,data.messages,this.versions,{replace:true});this.hasOlder=data.hasMore;this.bottom()}
    else if(mode==='older'){this.messages=mergeChatMessages(this.messages,data.messages,this.versions,{prepend:true});this.hasOlder=data.hasMore;this.$nextTick(()=>this.$refs.list?.scrollToIndex(data.messages.length))}
    else if(mode!=='changes'){const ids=new Set(this.messages.map(m=>m.id));const fresh=data.messages.filter(m=>!ids.has(m.id));if(!this.messages.length)this.hasOlder=data.hasMore;this.messages=mergeChatMessages(this.messages,data.messages,this.versions);if(fresh.length){if(shouldFollow)this.bottom();else this.unseen+=fresh.length}}
    this.applyChanges(data.changes||[]);if(Number.isSafeInteger(data.revision))this.revision=data.revision;
   }catch(e){if(!this.controller.signal.aborted)this.error=message(e)}finally{this.loading=false}
  },
  async latest(){if(this.loading)return;this.browsing=false;await this.fetch('latest')},async older(){if(this.loading)return;this.browsing=true;await this.fetch('older')},
  formatDraft(kind){
   const editor=this.$refs.editor,start=editor.selectionStart,end=editor.selectionEnd;
   const selected=this.draft.slice(start,end),body=selected||(kind==='block'||kind==='inline'?'code':'text');
   let before='',after='',content=body;
   if(kind==='bold'){before='**';after='**'}else if(kind==='italic'){before='*';after='*'}
   else if(kind==='inline'){const runs=body.match(/`+/g)||[];before=after='`'.repeat(Math.max(0,...runs.map(v=>v.length))+1);if(body.startsWith('`')||body.endsWith('`')){before+=' ';after=' '+after}}
   else if(kind==='block'){const runs=body.match(/`+/g)||[],fence='`'.repeat(Math.max(3,...runs.map(v=>v.length+1)));before=(start&&this.draft[start-1]!=='\n'?'\n':'')+fence+'\n';after='\n'+fence+(end<this.draft.length&&this.draft[end]!=='\n'?'\n':'')}
   else{before=start&&this.draft[start-1]!=='\n'?'\n':'';const prefix=kind==='quote'?'> ':'- ';content=body.split('\n').map(line=>prefix+line).join('\n');after=end<this.draft.length&&this.draft[end]!=='\n'?'\n':''}
   const next=this.draft.slice(0,start)+before+content+after+this.draft.slice(end);
   if([...next].length>2000){this.sendError='Formatting exceeds the 2000-character limit.';return}
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
.chat-room-heading{display:flex;flex:none;align-items:center;justify-content:space-between;gap:12px;min-height:44px;padding:4px 14px;border-bottom:1px solid var(--gm-border)}
.chat-room-name,.chat-room-actions{display:flex;align-items:center;gap:12px;min-width:0}
.chat-room-symbol{font-size:26px;color:var(--gm-muted);font-weight:500}
.chat-room-name .gm-page-title{font-size:16px;margin:0;font-weight:700;letter-spacing:0}
.chat-room-actions{gap:4px}.chat-history-button{font-size:12px;color:var(--gm-muted)}
.chat-info-menu ::v-deep .dropdown-toggle{width:32px;height:32px;color:var(--gm-muted)}
.chat-info-menu ::v-deep .dropdown-menu{width:280px;max-width:calc(100vw - 24px);font-size:13px}
.chat-info-menu p{margin:6px 0;color:var(--gm-muted);line-height:1.5}
.chat-error{flex:none;margin:8px 16px;padding:8px 12px;font-size:13px}
.chat-history-banner{flex:none;display:flex;align-items:center;justify-content:space-between;gap:8px;padding:4px 16px;background:var(--gm-accent-soft);font-size:12px}.chat-history-banner .btn{font-size:12px}
.chat-timeline{position:relative;flex:1;min-height:0}
.chat-list{height:100%;min-height:0;scrollbar-gutter:stable}
.chat-message{display:flex;align-items:flex-start;gap:10px;padding:6px 14px;min-width:0}
.chat-message:hover{background:var(--gm-panel-alt)}
.chat-avatar{display:grid;place-items:center;flex:0 0 30px;height:30px;border-radius:50%;background:var(--gm-panel-alt);color:var(--gm-muted);font-family:var(--gm-mono);font-size:12px;font-weight:600}
.chat-own .chat-avatar{color:var(--gm-accent);background:var(--gm-accent-soft)}
.chat-message-body{min-width:0;flex:1}.chat-message-meta{display:flex;gap:10px;align-items:baseline;flex-wrap:wrap;margin-bottom:2px}
.chat-message-meta strong{font-size:13px;display:flex;gap:8px;align-items:center}.chat-message-meta time{font-size:11px;color:var(--gm-muted);font-variant-numeric:tabular-nums}.chat-you{font-size:10px;color:var(--gm-muted);font-weight:400}
.chat-message ::v-deep .chat-markdown{font-size:14px;line-height:1.5}
.chat-edited{font-size:10px;color:var(--gm-muted)}
.chat-message-menu{flex:none}.chat-message-menu ::v-deep .dropdown-toggle{display:grid;place-items:center;width:32px;height:28px;padding:0;color:var(--gm-muted);border-radius:5px}.chat-message-menu ::v-deep .dropdown-menu{font-size:13px;min-width:130px}
.chat-message-menu ::v-deep .dropdown-toggle:focus-visible,.chat-inline-editor textarea:focus-visible{outline:2px solid var(--gm-accent);outline-offset:2px}
.chat-inline-editor textarea{display:block;width:100%;min-height:80px;max-height:280px;resize:vertical;background:var(--gm-panel-alt);color:var(--gm-text);border:1px solid var(--gm-border);border-radius:6px;padding:8px;font-size:14px;line-height:1.5;font-family:var(--gm-mono)}
.chat-edit-actions{display:flex;justify-content:flex-end;gap:6px;margin-top:6px}.chat-edit-error{font-size:12px;margin:5px 0}
@media(pointer:coarse){.chat-message-menu ::v-deep .dropdown-toggle{width:44px;height:44px}}
.chat-empty{position:absolute;inset:0;display:flex;justify-content:flex-end;align-items:flex-start;flex-direction:column;gap:10px;padding:20px;pointer-events:none;overflow:hidden}
.chat-empty-mark{display:grid;place-items:center;width:44px;height:44px;flex:none;border-radius:50%;font-size:28px;background:var(--gm-panel-alt);color:var(--gm-muted)}.chat-empty strong{font-size:18px}
.chat-jump{position:absolute;bottom:8px;left:50%;transform:translateX(-50%);border-radius:20px;max-width:calc(100% - 32px);font-size:12px;white-space:normal;box-shadow:0 2px 8px #0002}
.chat-composer{flex:none;padding:6px 14px 10px}
.chat-input-shell{display:flex;align-items:flex-end;gap:4px;border:1px solid var(--gm-border);border-radius:10px;padding:3px;background:var(--gm-panel-alt)}
.chat-input-shell:focus-within{border-color:var(--gm-accent);outline:2px solid var(--gm-accent);outline-offset:1px}
.chat-format-menu,.chat-send{flex:none}.chat-format-menu ::v-deep .dropdown-toggle,.chat-send{display:grid;place-items:center;width:32px;height:32px;padding:0;color:var(--gm-muted);border-radius:7px}
.chat-format-menu ::v-deep .dropdown-menu{min-width:190px;max-height:min(320px,55dvh);overflow-y:auto;font-size:13px}
.chat-format-menu ::v-deep .dropdown-item{padding:8px 16px}
.chat-composer #chat-editor{flex:1;min-width:0;border:0;box-shadow:none;background:transparent;color:var(--gm-text);font-size:14px;line-height:22px;padding:5px 4px;min-height:32px;max-height:128px;overflow-y:auto!important;resize:none}
.chat-composer #chat-editor:focus-visible{outline:none!important}
.chat-composer #chat-editor::placeholder{color:var(--gm-muted)}
.chat-send:not(:disabled){color:var(--gm-accent)}.chat-send:disabled{opacity:.4}
.chat-preview{flex:1;min-width:0;padding:8px 4px;max-height:160px;overflow:auto;font-size:14px}.chat-preview>.btn{font-size:12px;padding:4px 0}
.chat-send-error{font-size:12px;margin:6px 4px 0;max-height:70px;overflow:auto}
@media(max-width:600px){.chat-room-heading{padding:4px 12px}.chat-message{padding:6px 12px;gap:8px}.chat-avatar{flex-basis:28px;height:28px}.chat-message-meta{gap:6px}.chat-message-meta strong{font-size:12px}.chat-composer{padding:6px 10px 10px}.chat-format-menu ::v-deep .dropdown-toggle,.chat-send{width:44px;height:44px}.chat-composer #chat-editor{padding-top:11px;padding-bottom:11px;min-height:44px}.chat-history-banner{padding:4px 12px}}
</style>
