<template>
 <section class="workspace-page anomalies">
  <header class="gm-page-header"><div><div class="gm-eyebrow">Приоритеты анализа</div><h1 class="gm-page-title">Аномалии</h1><p class="gm-page-description">Замечайте необычные сессии и переходите к трафику, который стоит проверить первым.</p></div><div class="anomaly-header-actions"><b-button variant="outline-secondary" :aria-expanded="String(help)" aria-controls="anomaly-help" @click="help=!help">{{ help ? 'Скрыть справку' : 'Как читать' }}</b-button><b-button variant="outline-primary" :disabled="loading" @click="load">Обновить</b-button></div></header>
  <div class="anomaly-overview"><div class="gm-panel anomaly-stat"><span>Необычные сессии</span><strong>{{ items.length }}</strong><small>В выбранном периоде и пороге</small></div><div class="gm-panel anomaly-stat"><span>Сигналы кражи флагов</span><strong :class="{'text-danger':items.some(a=>a.kind==='theft')}">{{ items.filter(a=>a.kind==='theft').length }}</strong><small>Проверьте связанные сессии</small></div><div class="gm-panel anomaly-stat"><span>Модель готова</span><strong>{{ stats.services.filter(s=>s.warm).length }}<span> / {{ serviceCards.length }}</span></strong><small>Сервисов с накопленной нормой</small></div></div>

  <b-collapse id="anomaly-help" v-model="help">
   <div class="gm-panel anomaly-help mb-3 small">
    <p class="mb-2"><b>Очки</b> показывают, насколько сессия необычна. Каждое отличие от нормы даёт очки, они складываются. +7 значит «такое встречается реже, чем 1 раз на 128 запросов», +10 — реже 1 на 1000. Красные карточки с пометкой «кража флага» — флаг ушёл сессии, которая не связана с той, где его положил чекер.</p>
    <p class="mb-2"><b>Что проверяется</b> для каждого эндпоинта и поля:
     <span v-for="(label, key) in detectors" :key="key" class="mr-2"><b-badge variant="light">{{ label }}</b-badge></span>
    </p>
    <p class="mb-0"><b>«Это норма»</b> скрывает сессию и запоминает её отличия как нормальные, чтобы похожие больше не всплывали. Первые {{ stats.warmAfter }} сообщений каждого сервиса модель только учится; проверки полей включаются примерно после 30 запросов к эндпоинту. После перезапуска gomate обучение начинается заново.</p>
   </div>
  </b-collapse>

  <b-alert :show="!stats.flagPattern" variant="warning">
   Шаблон флага не найден. Чтобы ловить кражу флагов, создайте FIND-шаблон, в названии или группе которого есть «flag» или «флаг», например regex <code>[A-Z0-9]{31}=</code>.
  </b-alert>
  <b-alert :show="!!error" variant="danger">{{ error }}</b-alert>
  <b-alert :show="stats.drops>0" variant="warning">Детектор не успевал за трафиком и пропустил {{ stats.drops }} частей сессий. Захват при этом не пострадал.</b-alert>

  <div class="anomaly-section-heading"><h2>Сервисы</h2><span>Выберите сервис, чтобы сузить результаты</span></div>
  <div class="service-cards mb-3">
   <button v-for="s in serviceCards" :key="s.port" type="button" class="gm-panel service-card" :class="{selected: service===s.port}" :aria-pressed="String(service===s.port)" @click="service = service===s.port ? 0 : s.port">
    <div class="card-body p-2">
     <div class="d-flex justify-content-between">
      <b>{{ s.name }} <span class="text-muted">#{{ s.port }}</span></b>
      <b-badge :variant="s.state.variant">{{ s.state.text }}</b-badge>
     </div>
     <div class="small text-muted">{{ s.messages }} сообщений · {{ s.endpoints }} эндпоинтов · {{ s.reported }} необычных</div>
     <div class="small" v-if="s.flagsPut || s.flagsChecked || s.flagsStolen || s.flagsUnknown">
      Флаги: положено {{ s.flagsPut }} · забрал чекер {{ s.flagsChecked }} ·
      <span :class="{'text-danger font-weight-bold': s.flagsStolen}">украдено {{ s.flagsStolen }}</span>
      <span v-if="s.flagsUnknown" class="text-muted"> · без закладки {{ s.flagsUnknown }}</span>
     </div>
    </div>
   </button>
  </div>

  <div class="gm-panel anomaly-filter-panel">
   <div class="anomaly-filters">
    <b-form-group label="Сервис" label-for="anomaly-service"><b-form-select id="anomaly-service" v-model="service" :options="serviceOptions" aria-label="Сервис"/></b-form-group>
    <b-form-group label="Период" label-for="anomaly-period"><b-form-select id="anomaly-period" v-model="period" :options="periods" aria-label="Период"/></b-form-group>
    <b-form-group label="Чувствительность" label-for="anomaly-level"><b-form-select id="anomaly-level" v-model="level" :options="levels" aria-label="Чувствительность"/></b-form-group>
    <b-form-group label="Порядок" label-for="anomaly-sort"><b-form-select id="anomaly-sort" v-model="sort" :options="sorts" aria-label="Сортировка"/></b-form-group>
   </div>
   <div class="anomaly-view-options"><b-form-checkbox v-model="group" switch>Группировать похожие</b-form-checkbox><b-form-checkbox v-model="showDismissed" switch>Скрытые</b-form-checkbox><b-button v-if="service||period!==3600||level!==6||showDismissed" size="sm" variant="outline-secondary" @click="resetFilters">Сбросить фильтры</b-button></div>
  </div>
  <div class="anomaly-results-heading"><h2>Результаты</h2><span>{{ summaryLine }}</span></div>

  <div v-if="!groups.length && !loading" class="gm-panel anomaly-empty">
   <h3>Ничего необычного</h3>
   <div v-if="!stats.services.some(s => s.warm)">Модель ещё учится: нужно хотя бы {{ stats.warmAfter }} сообщений в сервисе.</div>
   <div v-else>Попробуйте увеличить период или снизить порог.</div>
  </div>

  <article v-for="g in groups" :key="g.key" class="gm-panel anomaly mb-2" :class="cardClass(g.top)">
   <div class="card-body p-3">
    <div class="anomaly-row">
     <div class="score text-center">
      <div class="score-value" :class="'text-' + variant(g.top)">{{ Math.round(g.top.score) }}</div>
      <div class="small text-muted">очков</div>
     </div>
     <div class="flex-grow-1 min-w-0">
      <div class="d-flex flex-wrap align-items-center mb-1">
       <b-badge v-if="g.top.kind==='theft'" variant="danger" class="mr-2">КРАЖА ФЛАГА</b-badge>
       <code class="summary mr-2">{{ g.top.summary }}</code>
       <span class="text-muted small">{{ serviceName(g.top.service) }} · {{ time(g.top.startTimestamp) }} · сессия #{{ g.top.streamId }}</span>
       <b-badge v-if="g.items.length>1" variant="info" class="ml-2">×{{ g.items.length }} похожих</b-badge>
       <b-badge v-if="g.top.dismissed" variant="secondary" class="ml-2">скрыто</b-badge>
      </div>
      <ul class="reasons list-unstyled mb-2">
       <li v-for="(r, i) in g.top.reasons" :key="i">
        <span class="bits">+{{ r.bits.toFixed(1) }}</span>
        <b-badge :variant="r.detector==='theft' ? 'danger' : 'light'" class="mr-1">{{ detectors[r.detector] || r.detector }}</b-badge>
        <code v-if="r.field" class="mr-1">{{ r.field }}</code>
        <span>{{ r.text }}</span>
        <span v-if="r.template && r.template!==g.top.summary" class="text-muted small"> · {{ r.template }}</span>
       </li>
      </ul>
      <div class="anomaly-actions">
       <b-button size="sm" variant="outline-primary" class="mr-1" :to="streamLink(g.top)">Открыть сессию</b-button>
       <b-button v-if="!g.top.dismissed" size="sm" variant="outline-success" class="mr-1" :disabled="busy" title="Скрыть и считать такие отличия нормальными" @click="normal(g.items)">
        Это норма{{ g.items.length>1 ? ' (все ' + g.items.length + ')' : '' }}
       </b-button>
       <b-button v-else size="sm" variant="outline-secondary" class="mr-1" :disabled="busy" @click="restore(g.items)">Вернуть</b-button>
       <b-button v-if="g.items.length>1" size="sm" variant="link" @click="toggle(g.key)">{{ open[g.key] ? 'Свернуть' : 'Показать все сессии' }}</b-button>
      </div>
      <div v-if="open[g.key]" class="table-responsive"><table class="table table-sm small mt-2 mb-0"><thead><tr><th scope="col">Очки</th><th scope="col">Время</th><th scope="col">Запрос</th><th scope="col">Причины</th><th scope="col">Сессия</th></tr></thead><tbody>
       <tr v-for="a in g.items" :key="a.streamId">
        <td class="text-nowrap"><b :class="'text-' + variant(a)">{{ Math.round(a.score) }}</b></td>
        <td class="text-nowrap">{{ time(a.startTimestamp) }}</td>
        <td><code>{{ a.summary }}</code></td>
        <td class="text-muted">{{ a.reasons.map(r => (detectors[r.detector] || r.detector) + (r.field ? ' ' + r.field : '')).join(', ') }}</td>
        <td class="text-nowrap"><router-link :to="streamLink(a)">#{{ a.streamId }}</router-link></td>
       </tr>
      </tbody></table></div>
     </div>
    </div>
   </div>
  </article>
 </section>
</template>

<script>
const message = e => e.response?.data?.error || e.message;
export default {
 data: () => ({
  items: [], stats: {services: [], drops: 0, flagPattern: true, warmAfter: 100},
  loading: false, busy: false, error: '', help: false, open: {}, timer: null, loadedAt: 0, destroyed: false,
  service: 0, period: 3600, level: 6, sort: 'score', group: true, showDismissed: false,
  detectors: {theft: 'Кража флага', flag: 'Флаг', template: 'Эндпоинт', field: 'Новое поле', missing: 'Нет поля', chars: 'Символы', enum: 'Значение', shape: 'Форма', length: 'Длина'},
  periods: [{value: 900, text: '15 минут'}, {value: 3600, text: 'Час'}, {value: 6 * 3600, text: '6 часов'}, {value: 0, text: 'Всё время'}],
  levels: [{value: 6, text: 'Всё необычное (≥ 6)'}, {value: 12, text: 'Заметное (≥ 12)'}, {value: 20, text: 'Сильное (≥ 20)'}, {value: 40, text: 'Только кражи флагов (≥ 40)'}],
  sorts: [{value: 'score', text: 'Сначала необычные'}, {value: 'time', text: 'Сначала новые'}],
 }),
 computed: {
  serviceOptions() {
   return [{value: 0, text: 'Все сервисы'}, ...this.$store.state.services.map(s => ({value: s.port, text: `${s.name} #${s.port}`}))];
  },
  serviceCards() {
   const byPort = Object.fromEntries(this.stats.services.map(s => [s.port, s]));
   return this.$store.state.services.map(s => {
    const x = byPort[s.port] || {messages: 0, endpoints: 0, reported: 0, warm: false};
    const state = !x.messages ? {text: 'Нет трафика', variant: 'light'} : x.warm ? {text: 'Работает', variant: 'success'} : {text: `Учится ${x.messages}/${this.stats.warmAfter}`, variant: 'secondary'};
    return {...x, port: s.port, name: s.name, state};
   });
  },
  groups() {
   if (!this.group) return this.items.map(a => ({key: String(a.streamId), top: a, items: [a]}));
   const out = new Map();
   for (const a of this.items) {
    const key = this.signature(a);
    if (!out.has(key)) out.set(key, {key, top: a, items: []});
    out.get(key).items.push(a);
   }
   return [...out.values()];
  },
  summaryLine() {
   const n = this.items.length;
   const theft = this.items.filter(a => a.kind === 'theft').length;
   return `${n} сессий${this.group ? `, ${this.groups.length} групп` : ''}${theft ? `, из них краж флагов: ${theft}` : ''}. Показаны не больше 200 самых ${this.sort === 'time' ? 'новых' : 'необычных'}.`;
  },
 },
 watch: {service: 'load', period: 'load', level: 'load', sort: 'load', showDismissed: 'load'},
 mounted() {
  this.load();
  this.timer = setInterval(() => { if (!document.hidden && Date.now() - this.loadedAt > 3000) this.load(); }, 1000);
 },
 beforeDestroy() { this.destroyed = true; clearInterval(this.timer); },
 methods: {
  resetFilters(){this.service=0;this.period=3600;this.level=6;this.showDismissed=false},
  refreshLive() { if (Date.now() - this.loadedAt > 1500) return this.load(); },
  async load() {
   if (this.loading) return;
   this.loading = true;
   try {
    const params = {minScore: this.level, period: this.period, sort: this.sort};
    if (this.service) params.service = this.service;
    if (this.showDismissed) params.dismissed = 1;
    const [list, stats] = await Promise.all([this.$http.get('/v2/anomalies', {params}), this.$http.get('/v2/anomalies/stats')]);
    if (this.destroyed) return;
    this.items = list.data;
    this.stats = stats.data;
    this.error = '';
   } catch (e) {
    this.error = 'Не удалось загрузить аномалии: ' + message(e);
   } finally {
    this.loading = false;
    this.loadedAt = Date.now();
   }
  },
  signature(a) {
   const parts = a.reasons.map(r => `${r.detector}:${r.field || ''}`).sort();
   const tpl = a.reasons.find(r => r.template)?.template || a.summary;
   return `${a.service}|${a.kind}|${tpl}|${[...new Set(parts)].join(',')}`;
  },
  async normal(items) { await this.mark(items, 'normal'); },
  async restore(items) { await this.mark(items, 'restore'); },
  async mark(items, action) {
   this.busy = true;
   try {
    for (const a of items) await this.$http.post(`/v2/anomalies/${a.streamId}/${action}`);
    await this.load();
   } catch (e) {
    this.$bvToast.toast(message(e), {title: 'Не удалось сохранить', variant: 'danger'});
   } finally {
    this.busy = false;
   }
  },
  toggle(key) { this.$set(this.open, key, !this.open[key]); },
  streamLink(a) { return {name: 'stream', params: {servicePort: String(a.service), streamId: String(a.streamId)}}; },
  serviceName(port) { const s = this.$store.state.services.find(x => x.port === port); return s ? `${s.name} #${port}` : `#${port}`; },
  time(ms) { return new Date(ms).toLocaleTimeString(); },
  variant(a) { return a.kind === 'theft' || a.score >= 20 ? 'danger' : a.score >= 12 ? 'warning' : 'secondary'; },
  cardClass(a) { return {'border-danger': a.kind === 'theft' || a.score >= 20, 'border-warning': a.kind !== 'theft' && a.score >= 12 && a.score < 20, dismissed: a.dismissed}; },
 },
};
</script>

<style scoped>
.workspace-page{max-width:1500px;margin:auto;padding-bottom:28px}.anomaly-header-actions{display:flex;gap:8px;flex-wrap:wrap}.anomaly-overview{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:12px;margin-bottom:24px}.anomaly-stat{padding:18px 20px;display:flex;flex-direction:column;gap:5px}.anomaly-stat>span{font-size:.8rem;color:var(--gm-muted)}.anomaly-stat strong{font-size:1.8rem;line-height:1.3;font-variant-numeric:tabular-nums}.anomaly-stat strong span{font-size:1rem;color:var(--gm-muted);font-weight:400}.anomaly-stat small{font-size:.7rem;color:var(--gm-muted)}.anomaly-help{padding:20px;line-height:1.7}.anomaly-section-heading,.anomaly-results-heading{display:flex;justify-content:space-between;align-items:center;gap:12px;flex-wrap:wrap;margin:22px 0 12px}.anomaly-section-heading h2,.anomaly-results-heading h2{font-size:.95rem;margin:0;font-weight:600}.anomaly-section-heading>span,.anomaly-results-heading>span{font-size:.75rem;color:var(--gm-muted)}.service-cards{display:grid;grid-template-columns:repeat(auto-fill,minmax(260px,1fr));gap:10px}.service-card{cursor:pointer;text-align:left;color:var(--gm-text);background:var(--gm-panel);border:1px solid var(--gm-border);border-radius:var(--gm-radius);padding:10px;min-width:0}.service-card:hover,.service-card.selected{border-color:var(--gm-accent);background:var(--gm-accent-soft)}.service-card .card-body>div:first-child{gap:8px;flex-wrap:wrap;margin-bottom:8px}.service-card .card-body>b{overflow-wrap:anywhere}.service-card .card-body .small{line-height:1.7;font-size:.75rem}.anomaly-filter-panel{padding:18px 20px;margin-top:20px}.anomaly-filters{display:grid;grid-template-columns:1fr .8fr 1.3fr 1fr;gap:14px}.anomaly-filters .form-group{margin-bottom:0}.anomaly-view-options{display:flex;align-items:center;gap:20px;flex-wrap:wrap;border-top:1px solid var(--gm-border);padding-top:14px;margin-top:16px}.anomaly-view-options>.btn{margin-left:auto}.anomaly-empty{padding:50px 20px;text-align:center;color:var(--gm-muted);font-size:.85rem}.anomaly-empty h3{font-size:1.1rem;color:var(--gm-text);margin-bottom:12px}.anomaly{border-left:3px solid var(--gm-border);overflow:hidden}.anomaly.dismissed{opacity:.75}.anomaly-row{display:flex;gap:18px;align-items:flex-start}.score{min-width:64px;border-radius:10px;background:var(--gm-panel-alt);padding:12px 8px;flex-shrink:0}.score-value{font-size:1.6rem;font-weight:600;line-height:1.2;font-variant-numeric:tabular-nums}.score .small{font-size:.65rem;margin-top:4px}.summary{overflow-wrap:anywhere;font-family:var(--gm-mono);font-size:.85rem;color:var(--gm-text)}.min-w-0{min-width:0}.reasons{margin-top:12px}.reasons li{overflow-wrap:anywhere;margin-bottom:7px;font-size:.8rem;line-height:1.6}.bits{display:inline-block;min-width:3.6em;font-family:var(--gm-mono);font-variant-numeric:tabular-nums;font-weight:600;color:var(--gm-muted);font-size:.7rem}.anomaly-actions{display:flex;gap:7px;flex-wrap:wrap;margin-top:14px}.anomaly-actions .btn{margin:0!important}.anomaly .border-danger{border-color:var(--gm-danger)}
@media(max-width:900px){.anomaly-filters{grid-template-columns:1fr 1fr}.anomaly-stat{padding:14px}.anomaly-stat small{display:none}}
@media(max-width:600px){.anomaly-overview{grid-template-columns:1fr 1fr;gap:8px}.anomaly-stat:last-child{grid-column:1/-1;flex-direction:row;align-items:center;justify-content:space-between}.anomaly-stat:last-child strong{font-size:1.1rem}.service-cards,.anomaly-filters{grid-template-columns:1fr}.anomaly-filter-panel{padding:14px}.anomaly-row{gap:10px}.score{min-width:44px;padding:9px 5px}.score-value{font-size:1.2rem}.anomaly .card-body{padding:12px!important}.anomaly-view-options{gap:12px}.anomaly-view-options>.btn{margin-left:0}.anomaly-results-heading{align-items:flex-start}.service-card{padding:7px}.anomaly-header-actions{width:100%}}
.anomaly-overview>.gm-panel,.service-cards>.gm-panel{margin:0}.anomaly{padding:0}
</style>
