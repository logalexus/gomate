<template>
 <span class="session-share">
  <button type="button" class="btn btn-sm btn-outline-secondary" :disabled="!streamId" @click="open"><i class="fas fa-comment" aria-hidden="true"/> Share in chat</button>
  <b-modal ref="modal" title="Share session in team chat" hide-footer :return-focus="returnFocus" :no-close-on-backdrop="sending" :no-close-on-esc="sending" :hide-header-close="sending" @shown="focusComment">
   <form class="session-share-form" @submit.prevent="send">
    <p>A link to session #{{ selectedId }} will be sent to chat. Content is available while the session is stored on the server.</p>
    <label :for="'session-share-comment-'+_uid">Comment <span class="text-muted">(optional)</span></label>
    <textarea :id="'session-share-comment-'+_uid" ref="comment" v-model="comment" class="form-control" rows="3" maxlength="1500" :disabled="sending||sent" placeholder="What to look for"/>
    <div class="session-share-preview" aria-label="Message preview"><ChatMarkdown :text="message"/></div>
    <p v-if="error" class="text-danger" role="alert">{{ error }}</p>
    <p v-if="sent" class="text-success" role="status">Session shared in team chat. <router-link to="/chat" @click.native="$refs.modal.hide()">Open chat</router-link></p>
    <div class="session-share-actions"><button type="button" class="btn btn-sm btn-outline-secondary" :disabled="sending" @click="$refs.modal.hide()">{{ sent?'Close':'Cancel' }}</button><button v-if="!sent" type="submit" class="btn btn-sm btn-primary" :disabled="sending">{{ sending?'Sending…':'Send to chat' }}</button></div>
   </form>
  </b-modal>
 </span>
</template>
<script>
import ChatMarkdown from './ChatMarkdown.vue';
const messageKey=()=>{const bytes=new Uint8Array(16);crypto.getRandomValues(bytes);return Array.from(bytes,n=>n.toString(16).padStart(2,'0')).join('')};
export default {
 components:{ChatMarkdown},props:['streamId','servicePort'],
 data:()=>({selectedId:'',url:'',comment:'',sending:false,sent:false,error:'',returnFocus:null,pendingText:'',pendingKey:'',identityReady:false}),
 computed:{message(){return `[Session #${this.selectedId}](${this.url})`+(this.comment.trim()?'\n\n'+this.comment.trim():'')}},
 methods:{
  open(event){this.returnFocus=event.currentTarget;this.selectedId=String(this.streamId);const href=this.$router.resolve({name:'stream',params:{servicePort:String(this.servicePort||'0'),streamId:this.selectedId}}).href;this.url=new URL(href,location.href).href;this.comment='';this.error='';this.sent=false;this.pendingText='';this.pendingKey='';this.$refs.modal.show()},
  focusComment(){this.$refs.comment?.focus()},
  async send(){if(this.sending||this.sent)return;const text=this.message;if(text!==this.pendingText){this.pendingText=text;this.pendingKey=messageKey()}this.sending=true;this.error='';try{
   // Establish the anonymous author cookie before POST, so a lost POST response
   // can be retried with the same author and idempotency key.
   if(!this.identityReady){await this.$http.get('/v2/chat');this.identityReady=true}
   await this.$http.post('/v2/chat',{text,key:this.pendingKey});this.sent=true;
  }catch(e){this.error=e.response?.status===429?'Too many messages. Wait a second and try again.':'Could not send the message. Your comment is saved; try again.'}finally{this.sending=false}}
 }
};
</script>
<style scoped>
.session-share{display:inline-flex}.session-share>.btn{font-size:.75rem;min-height:30px}.session-share-form{font-size:.8125rem}.session-share-form>p{margin-bottom:8px}.session-share-form textarea{font-size:.85rem;background:var(--gm-panel);color:var(--gm-text);border-color:var(--gm-border)}.session-share-preview{margin:8px 0;padding:8px;border:1px solid var(--gm-border);border-radius:6px;background:var(--gm-panel-alt);overflow-wrap:anywhere}.session-share-actions{display:flex;justify-content:flex-end;gap:8px}
</style>
