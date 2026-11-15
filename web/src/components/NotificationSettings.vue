<template>
 <section class="notification-settings">
  <h6>High-score anomalies</h6>
  <p>Notifications appear on every page while gomate is open. Settings apply immediately and are saved in this browser.</p>
  <b-form-checkbox v-model="enabled" switch>Notify me about new anomalies</b-form-checkbox>
  <b-form-group label="Minimum score" label-for="notification-score" class="mt-3" description="An integer from 1 to 1000. Notifications appear when the score reaches or exceeds this threshold.">
   <b-form-input id="notification-score" v-model="draft" type="number" min="1" max="1000" step="1" :disabled="!enabled" :state="valid ? null : false" @input="apply"/>
   <b-form-invalid-feedback>Enter an integer from 1 to 1000. Current threshold: {{ score }}.</b-form-invalid-feedback>
  </b-form-group>
  <p class="mb-0">Current threshold: <strong>score ≥ {{ score }}</strong>. Existing alerts are not shown on startup. Repeated session updates are grouped; open the session stream from its notification.</p>
 </section>
</template>
<script>
import {notificationScore} from '../utils/anomaly-notifications.mjs';
export default {
 data(){return {draft:notificationScore(this.$store.state.anomalyNotificationScore)}},
 computed:{
  enabled:{get(){return this.$store.state.anomalyNotificationsEnabled !== false},set(v){this.$store.commit('setAnomalyNotificationsEnabled',v)}},
  score(){return notificationScore(this.$store.state.anomalyNotificationScore)},
  valid(){const n=Number(this.draft);return this.draft!=='' && Number.isInteger(n) && n>=1 && n<=1000},
 },
 watch:{score(v){this.draft=v}},
 methods:{apply(){if(this.valid)this.$store.commit('setAnomalyNotificationScore',Number(this.draft))}},
};
</script>
<style scoped>
.notification-settings{margin:8px 0 12px;padding:12px;border:1px solid var(--gm-border);border-radius:8px;background:var(--gm-panel)}
h6{font-size:13px;font-weight:700}p{font-size:12px;color:var(--gm-muted);max-width:75ch}input{max-width:160px}
</style>
