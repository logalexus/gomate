<template>
 <section class="notification-settings">
  <h6>Аномалии с высоким скором</h6>
  <p>Уведомления появляются во всех разделах, пока gomate открыт. Настройки применяются сразу и сохраняются в этом браузере.</p>
  <b-form-checkbox v-model="enabled" switch>Уведомлять о новых аномалиях</b-form-checkbox>
  <b-form-group label="Минимальный скор" label-for="notification-score" class="mt-3" description="Целое число от 1 до 1000. Уведомление появляется, когда скор достигает порога или превышает его.">
   <b-form-input id="notification-score" v-model="draft" type="number" min="1" max="1000" step="1" :disabled="!enabled" :state="valid ? null : false" @input="apply"/>
   <b-form-invalid-feedback>Введите целое число от 1 до 1000. Пока действует порог {{ score }}.</b-form-invalid-feedback>
  </b-form-group>
  <p class="mb-0">Текущий порог: <strong>скор ≥ {{ score }}</strong>. Старые срабатывания при открытии не показываются. Повторные обновления сессии объединяются; из уведомления можно перейти к её потоку.</p>
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
