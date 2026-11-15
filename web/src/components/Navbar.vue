<template>
 <header class="gm-topbar">
  <router-link class="gm-brand" :to="{name:'stream'}" aria-label="Gomate — traffic"><svg class="gm-brand-mark" viewBox="0 0 32 32" fill="none" aria-hidden="true"><rect width="32" height="32" rx="9" fill="currentColor"/><path d="M9 11h14M9 16h9M9 21h14" stroke="var(--gm-on-accent)" stroke-width="2.5" stroke-linecap="round"/><circle cx="23" cy="16" r="1.5" fill="var(--gm-on-accent)"/></svg><span>gomate<span class="gm-brand-caption">TRAFFIC WORKSPACE</span></span></router-link>
  <nav class="gm-navigation" aria-label="Main navigation">
   <router-link v-for="item in navigation" :key="item.name" :to="{name:item.name}" :class="{'gm-nav-active':$route.name===item.name}" :aria-current="$route.name===item.name?'page':null"><i :class="item.icon" aria-hidden="true"/><span>{{ item.label }}</span></router-link>
  </nav>
  <div class="gm-nav-tools"><PatternsDropdown ref="patternsDropdown"/><b-button class="gm-settings-trigger" variant="outline-secondary" @click="showSettings" aria-label="Settings"><i class="fas fa-sliders-h" aria-hidden="true"/><span class="gm-action-label">Settings</span></b-button><ThemeButton/></div>
  <ServiceModal :creating="serviceModalIsCreating" :initial-service="serviceModalEditingService" @service-update-needed="updateServices"/>
 </header>
</template>
<script>
import PatternsDropdown from './PatternsDropdown';
import ThemeButton from './ThemeButton';
import ServiceModal from '../views/ServiceModal';
export default {
 name:'Navbar',components:{PatternsDropdown,ThemeButton,ServiceModal},
 data:()=>({serviceModalIsCreating:true,serviceModalEditingService:{},navigation:[{name:'stream',label:'Traffic',icon:'fas fa-stream'},{name:'anomalies',label:'Anomalies',icon:'fas fa-wave-square'},{name:'containers',label:'Containers',icon:'fas fa-cubes'},{name:'monitoring',label:'Monitoring',icon:'fas fa-chart-line'},{name:'farm',label:'Farm',icon:'fas fa-flag'},{name:'chat',label:'Team chat',icon:'far fa-comment-alt'}]}),
 mounted(){this.updateServices();this.$root.$on('gm:add-service',this.showAddService);this.$root.$on('gm:edit-service',this.editService)},
 beforeDestroy(){this.$root.$off('gm:add-service',this.showAddService);this.$root.$off('gm:edit-service',this.editService)},
 methods:{
  async editService(service){this.serviceModalIsCreating=false;this.serviceModalEditingService=service;await this.$nextTick();this.$bvModal.show('serviceModal')},
  showSettings(){this.$bvModal.show('settingsModal')},
  async showAddService(){this.serviceModalIsCreating=true;this.serviceModalEditingService={};await this.$nextTick();this.$bvModal.show('serviceModal')},
  async updateServices(){try{const {data}=await this.$http.get('service/');this.$store.commit('setServices',data)}catch{this.$bvToast.toast('Could not load services. Check your connection.',{title:'Services',variant:'danger'})}},
 },
};
</script>
