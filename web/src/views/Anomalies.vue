<template>
 <section class="workspace-page anomalies">
  <header class="anomaly-header">
   <div><h1>Аномалии</h1><p>Раскройте строку → проверьте причины → откройте сессию с трафиком.</p></div>
   <div class="anomaly-actions">
    <b-button size="sm" variant="outline-secondary" :aria-expanded="String(help)" aria-controls="anomaly-help" @click="help=!help">Как работать</b-button>
    <b-button size="sm" variant="outline-secondary" :aria-pressed="String(paused)" :aria-label="paused ? 'Продолжить автообновление' : 'Пауза'" @click="paused=!paused">{{ paused ? 'Продолжить' : 'Пауза' }}</b-button>
    <b-button size="sm" variant="outline-primary" :disabled="loading || busy" @click="load()">Обновить</b-button>
   </div>
  </header>

  <div v-if="help" id="anomaly-help" class="gm-panel anomaly-help">
   <ol>
    <li><b>Выберите сервис и период.</b> По умолчанию выше сессии с большей необычностью. Баллы — сумма отличий от изученного трафика, а не вероятность атаки.</li>
    <li><b>Раскройте строку.</b> Посмотрите, какое поле отличается и почему. В группе можно выбрать конкретную сессию. Пока причины открыты, список не обновляется.</li>
    <li><b>Откройте сессию</b> и проверьте запрос и ответ. Высокая оценка сама по себе не подтверждает атаку; повторяющиеся атаки могут стать привычными для модели.</li>
    <li><b>«Это норма»</b> скрывает только выбранную сессию и дополнительно обучает модель на её отличиях. Применяйте после проверки. «Вернуть в список» отменяет скрытие, но не обучение.</li>
   </ol>
   <p>Для просмотра скрытых включите «Включая скрытые». Первые {{ stats.warmAfter }} сообщений сервиса нужны для прогрева; после перезапуска приложения обучение начинается заново.</p>
  </div>
  <div v-if="error" role="alert" class="anomaly-notice error">{{ error }}</div>
  <div v-if="stats.drops>0" role="status" class="anomaly-notice">Детектор пропустил {{ stats.drops }} частей сессий из-за перегрузки. Захват трафика продолжался.</div>

  <div class="anomaly-toolbar">
   <div class="anomaly-filters">
    <b-form-group label="Сервис" label-for="anomaly-service"><b-form-select id="anomaly-service" v-model="service" size="sm" :options="serviceOptions"/></b-form-group>
    <b-form-group label="Период" label-for="anomaly-period"><b-form-select id="anomaly-period" v-model="period" size="sm" :options="periods"/></b-form-group>
    <b-form-group label="Минимум баллов" label-for="anomaly-level"><b-form-select id="anomaly-level" v-model="level" size="sm" :options="levels"/></b-form-group>
    <b-form-group label="Порядок" label-for="anomaly-sort"><b-form-select id="anomaly-sort" v-model="sort" size="sm" :options="sorts"/></b-form-group>
   </div>
   <div class="anomaly-view-options">
    <b-form-checkbox v-model="group">Группировать похожие</b-form-checkbox>
    <b-form-checkbox v-model="showDismissed">Включая скрытые</b-form-checkbox>
    <b-button v-if="service||period!==3600||level!==6||showDismissed||sort!=='score'" size="sm" variant="link" @click="resetFilters">Сбросить фильтры</b-button>
    <button type="button" class="anomaly-status-toggle" :aria-expanded="String(modelInfo)" aria-controls="anomaly-model" @click="modelInfo=!modelInfo">Модель: {{ stats.services.filter(s=>s.warm).length }}/{{ serviceCards.length }} сервисов прогрето<span v-if="!stats.flagPattern"> · кража флагов не проверяется</span> {{ modelInfo ? '▴' : '▾' }}</button>
   </div>
  </div>
  <div v-if="modelInfo" id="anomaly-model" class="gm-panel anomaly-model">
   <p v-if="!stats.flagPattern">Для проверки кражи флагов создайте FIND-шаблон с «flag» или «флаг» в названии или группе, например <code>[A-Z0-9]{31}=</code>.</p>
   <ul><li v-for="s in serviceCards" :key="s.port"><b>{{ s.name }} #{{ s.port }}</b> · {{ s.state }} · {{ s.messages }} сообщений · {{ s.reported }} срабатываний с запуска<span v-if="s.flagsPut || s.flagsChecked || s.flagsStolen || s.flagsUnknown"> · флаги: положено {{ s.flagsPut || 0 }}, забрал чекер {{ s.flagsChecked || 0 }}, украдено {{ s.flagsStolen || 0 }}, без закладки {{ s.flagsUnknown || 0 }}</span></li></ul>
  </div>

  <div class="anomaly-results-heading">
   <span>{{ summaryLine }}</span>
   <span role="status">{{ loading ? 'Загрузка…' : expanded ? 'Пауза на время просмотра' : paused ? 'Автообновление на паузе' : 'Автообновление' }}</span>
  </div>
  <div v-if="!groups.length && !loading" class="gm-panel anomaly-empty">
   <h2>{{ error ? 'Результаты недоступны' : 'Нет срабатываний по этим фильтрам' }}</h2>
   <p v-if="error">Повторите загрузку кнопкой «Обновить».</p>
   <p v-else-if="!stats.services.some(s => s.warm)">Модель ещё учится: нужно хотя бы {{ stats.warmAfter }} сообщений в сервисе.</p>
   <p v-else>Увеличьте период или снизьте порог. Отсутствие срабатываний не означает отсутствие атак.</p>
  </div>

  <div class="anomaly-list">
   <article v-for="(g, index) in groups" :key="g.key" class="anomaly" :class="{expanded:expanded===g.key, theft:g.top.kind==='theft', dismissed:g.top.dismissed}">
    <button type="button" class="anomaly-row" :aria-expanded="String(expanded===g.key)" :aria-controls="'anomaly-detail-'+index" @click="toggle(g)">
     <span class="anomaly-score"><strong>{{ Math.round(g.top.score) }}</strong><small>баллов</small></span>
     <span class="anomaly-description">
      <span class="anomaly-title"><b v-if="g.top.kind==='theft'" class="theft-label">Кража флага</b><code :title="g.top.summary">{{ endpoint(g.top) }}</code><span v-if="g.top.dismissed" class="hidden-label">Скрыто</span></span>
      <span class="anomaly-reason-preview">{{ reasonPreview(g.top) }}</span>
     </span>
     <span class="anomaly-meta"><span>{{ serviceName(g.top.service) }}</span><span>{{ time(g.top.startTimestamp) }} · {{ g.items.length>1 ? g.items.length+' сессий' : '#'+g.top.streamId }}</span></span>
     <span class="anomaly-chevron" aria-hidden="true">{{ expanded===g.key ? '▴' : '▾' }}</span>
    </button>
    <div v-if="expanded===g.key" :id="'anomaly-detail-'+index" class="anomaly-detail">
     <div class="anomaly-detail-toolbar">
      <label v-if="g.items.length>1" class="anomaly-session-picker">Сессия в группе
       <select v-model="selectedId" class="custom-select custom-select-sm"><option v-for="a in g.items" :key="a.streamId" :value="a.streamId">#{{ a.streamId }} · {{ time(a.startTimestamp) }} · {{ Math.round(a.score) }} баллов</option></select>
      </label>
      <span v-else class="anomaly-session-label">Сессия #{{ selected(g).streamId }} · {{ time(selected(g).startTimestamp) }}</span>
      <div class="anomaly-actions">
       <b-button size="sm" variant="primary" :to="streamLink(selected(g))">Открыть сессию</b-button>
       <b-button size="sm" variant="outline-secondary" :disabled="busy" :title="selected(g).dismissed ? 'Отменить скрытие этой сессии; обучение сохранится' : 'Скрыть эту сессию и обучить модель на её отличиях'" @click="mark(selected(g))">{{ selected(g).dismissed ? 'Вернуть в список' : 'Это норма' }}</b-button>
      </div>
     </div>
     <code class="anomaly-full-summary">{{ selected(g).summary }}</code>
     <ul class="anomaly-reasons">
      <li v-for="(r, i) in selected(g).reasons" :key="i">
       <span class="anomaly-bits">+{{ r.bits.toFixed(1) }}</span>
       <div><b>{{ detectors[r.detector] || r.detector }}</b> <code v-if="r.field">{{ r.field }}</code><p>{{ r.text }}</p><small v-if="r.template && r.template!==selected(g).summary">{{ r.template }}</small></div>
      </li>
     </ul>
     <p class="anomaly-detail-note">Баллы показывают необычность, а не вероятность атаки. «Это норма» действует только на сессию #{{ selected(g).streamId }}.</p>
    </div>
   </article>
  </div>
 </section>
</template>

<script>
const message = e => e.response?.data?.error || e.message;
export default {
 data: () => ({
  items: [], stats: {services: [], drops: 0, flagPattern: true, warmAfter: 100},
  loading: false, busy: false, error: '', help: false, modelInfo: false, expanded: '', selectedId: null,
  paused: false, timer: null, loadedAt: 0, destroyed: false, requestId: 0,
  service: 0, period: 3600, level: 6, sort: 'score', group: true, showDismissed: false,
  detectors: {theft: 'Кража флага', flag: 'Флаг', template: 'Эндпоинт', field: 'Новое поле', missing: 'Нет поля', chars: 'Символы', enum: 'Значение', shape: 'Форма', length: 'Длина'},
  periods: [{value: 900, text: '15 минут'}, {value: 3600, text: 'Час'}, {value: 6 * 3600, text: '6 часов'}, {value: 0, text: 'Всё время'}],
  levels: [6, 12, 20, 40].map(value => ({value, text: '≥ ' + value + ' баллов'})),
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
    const state = !x.messages ? 'Нет трафика' : x.warm ? 'Прогрета' : `Учится ${x.messages}/${this.stats.warmAfter}`;
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
   return `${n} сессий${this.group ? ` · ${this.groups.length} групп` : ''}${theft ? ` · краж флагов: ${theft}` : ''}${n>=200 ? ' · лимит 200, сузьте фильтры' : ''}`;
  },
 },
 watch: {
  service: 'reloadFilters', period: 'reloadFilters', level: 'reloadFilters', sort: 'reloadFilters', showDismissed: 'reloadFilters',
  group() { this.expanded = ''; },
 },
 mounted() { this.load(); this.timer = setInterval(this.refreshLive, 1000); },
 beforeDestroy() { this.destroyed = true; clearInterval(this.timer); },
 methods: {
  resetFilters() { this.service=0; this.period=3600; this.level=6; this.showDismissed=false; this.sort='score'; },
  reloadFilters() { this.items = []; return this.load(); },
  refreshLive() {
   if (!document.hidden && !this.paused && !this.expanded && !this.busy && !this.loading && Date.now()-this.loadedAt>3000) return this.load(false);
  },
  async load(closeDetails = true) {
   if (closeDetails) this.expanded = '';
   const id = ++this.requestId;
   this.loading = true;
   try {
    const params = {minScore: this.level, period: this.period, sort: this.sort};
    if (this.service) params.service = this.service;
    if (this.showDismissed) params.dismissed = 1;
    const [list, stats] = await Promise.all([this.$http.get('/v2/anomalies', {params}), this.$http.get('/v2/anomalies/stats')]);
    if (this.destroyed || id !== this.requestId) return;
    this.items = list.data;
    this.stats = stats.data;
    this.error = '';
   } catch (e) {
    if (!this.destroyed && id === this.requestId) this.error = 'Не удалось загрузить аномалии: ' + message(e);
   } finally {
    if (id === this.requestId) { this.loading = false; this.loadedAt = Date.now(); }
   }
  },
  signature(a) {
   const parts = a.reasons.map(r => `${r.detector}:${r.field || ''}`).sort();
   const tpl = a.reasons.find(r => r.template)?.template || a.summary;
   return `${a.service}|${a.kind}|${!!a.dismissed}|${tpl}|${[...new Set(parts)].join(',')}`;
  },
  async mark(a) {
   if (this.busy) return;
   this.busy = true;
   try {
    await this.$http.post(`/v2/anomalies/${a.streamId}/${a.dismissed ? 'restore' : 'normal'}`);
    await this.load();
   } catch (e) {
    this.$bvToast.toast(message(e), {title: 'Не удалось сохранить', variant: 'danger'});
   } finally { this.busy = false; }
  },
  toggle(g) {
   // Ignore an in-flight live response so opening details always freezes this list.
   ++this.requestId;
   this.loading = false;
   this.expanded = this.expanded === g.key ? '' : g.key;
   this.selectedId = g.top.streamId;
  },
  selected(g) { return g.items.find(a => a.streamId === this.selectedId) || g.top; },
  endpoint(a) { return a.reasons.find(r => r.template)?.template || a.summary.split('?')[0]; },
  reasonPreview(a) {
   const r = a.reasons[0];
   return r ? `${this.detectors[r.detector] || r.detector}${r.field ? ': '+r.field : ''} · ${r.text}` : 'Раскройте для просмотра сессии';
  },
  streamLink(a) { return {name: 'stream', params: {servicePort: String(a.service), streamId: String(a.streamId)}}; },
  serviceName(port) { const s = this.$store.state.services.find(x => x.port === port); return s ? `${s.name} #${port}` : `#${port}`; },
  time(ms) { return new Date(ms).toLocaleTimeString(); },
 },
};
</script>

<style scoped>
.anomalies{max-width:1500px;margin:auto;padding-bottom:16px}
.anomaly-header{display:flex;justify-content:space-between;align-items:center;gap:12px;margin-bottom:12px}
.anomaly-header h1{font-size:21px;line-height:1.3;margin:0 0 4px;font-weight:600}
.anomaly-header p{font-size:13px;color:var(--gm-muted);margin:0}
.anomaly-actions{display:flex;flex-wrap:wrap;align-items:center;gap:6px}
.anomaly-help,.anomaly-model{padding:12px 16px;margin:10px 0;font-size:13px;line-height:1.6}
.anomaly-help ol{padding-left:20px;margin:0}.anomaly-help li+li{margin-top:6px}.anomaly-help p{margin:8px 0 0}
.anomaly-model ul{padding-left:18px;margin:0}.anomaly-model p{margin:0 0 8px}
.anomaly-notice{padding:8px 12px;margin:8px 0;background:var(--gm-warning-soft);border:1px solid var(--gm-warning);border-radius:6px;font-size:13px;overflow-wrap:anywhere}
.anomaly-notice.error{background:var(--gm-danger-soft);border-color:var(--gm-danger)}
.anomaly-toolbar{border:1px solid var(--gm-border);border-radius:8px;padding:10px 12px;background:var(--gm-panel)}
.anomaly-filters{display:grid;grid-template-columns:1.2fr .7fr .8fr 1fr;gap:12px}
.anomaly-filters .form-group{margin:0}.anomaly-filters ::v-deep label{padding:0;margin:0 0 3px;font-size:12px}
.anomaly-filters ::v-deep select{font-size:13px;height:32px;padding-top:4px;padding-bottom:4px}
.anomaly-view-options{display:flex;align-items:center;flex-wrap:wrap;gap:6px 18px;margin-top:8px;font-size:12px}
.anomaly-view-options ::v-deep .custom-control-label{font-size:12px;line-height:24px}
.anomaly-status-toggle{margin-left:auto;padding:4px 0;background:none;border:0;color:var(--gm-muted);font-size:12px;text-align:left}
.anomaly-status-toggle:hover{color:var(--gm-text)}
.anomaly-results-heading{display:flex;justify-content:space-between;flex-wrap:wrap;gap:4px 12px;margin:10px 0 6px;color:var(--gm-muted);font-size:12px}
.anomaly-empty{padding:24px 16px;text-align:center;color:var(--gm-muted)}.anomaly-empty h2{font-size:16px;color:var(--gm-text)}.anomaly-empty p{font-size:13px;margin:0}
.anomaly-list{border:1px solid var(--gm-border);border-radius:8px;overflow:hidden;background:var(--gm-panel)}.anomaly-list:empty{display:none}
.anomaly+.anomaly{border-top:1px solid var(--gm-border)}
.anomaly-row{display:grid;grid-template-columns:52px minmax(0,1fr) minmax(120px,200px) 16px;gap:12px;align-items:center;width:100%;padding:9px 12px;background:transparent;border:0;color:var(--gm-text);text-align:left;cursor:pointer}
.anomaly-row:hover,.anomaly.expanded>.anomaly-row{background:var(--gm-panel-alt)}
.anomaly-row:focus-visible,.anomaly-status-toggle:focus-visible{outline:2px solid var(--gm-accent);outline-offset:-2px}
.anomaly-score{display:flex;flex-direction:column;align-items:center;font-variant-numeric:tabular-nums}.anomaly-score strong{font-size:19px;line-height:1.2;font-weight:600}.anomaly-score small{font-size:10px;color:var(--gm-muted)}
.theft>.anomaly-row{box-shadow:inset 3px 0 var(--gm-danger)}.theft-label{color:var(--gm-danger);font-size:12px;white-space:nowrap}
.anomaly-description,.anomaly-meta{min-width:0}.anomaly-title{display:flex;gap:8px;align-items:center;min-width:0}
.anomaly-title code{font-family:var(--gm-mono);font-size:13px;color:var(--gm-text);overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.hidden-label{font-size:11px;color:var(--gm-muted)}
.anomaly-reason-preview{display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--gm-muted);font-size:12px;margin-top:3px}
.anomaly-meta{display:flex;flex-direction:column;gap:3px;color:var(--gm-muted);font-size:12px}.anomaly-meta>span{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.anomaly-chevron{color:var(--gm-muted)}
.anomaly-detail{padding:12px 16px 12px 76px;border-top:1px solid var(--gm-border)}
.anomaly-detail-toolbar{display:flex;align-items:center;justify-content:space-between;flex-wrap:wrap;gap:10px;margin-bottom:10px}
.anomaly-session-picker{display:flex;align-items:center;flex-wrap:wrap;gap:6px;margin:0;font-size:12px;color:var(--gm-muted)}.anomaly-session-picker select{width:auto;max-width:100%;font-size:12px}.anomaly-session-label{color:var(--gm-muted);font-size:12px}
.anomaly-full-summary{display:block;overflow-wrap:anywhere;white-space:pre-wrap;color:var(--gm-text);font-size:12px;background:var(--gm-panel-alt);border-radius:4px;padding:8px}
.anomaly-reasons{list-style:none;padding:0;margin:10px 0}.anomaly-reasons li{display:grid;grid-template-columns:44px minmax(0,1fr);gap:8px;padding:6px 0;font-size:13px;overflow-wrap:anywhere}.anomaly-reasons li+li{border-top:1px solid var(--gm-border)}.anomaly-reasons p{margin:3px 0 0}.anomaly-reasons small{color:var(--gm-muted)}.anomaly-reasons code{color:var(--gm-text);font-size:12px}
.anomaly-bits{font-family:var(--gm-mono);font-size:12px;color:var(--gm-muted);padding-top:2px}.anomaly-detail-note{margin:8px 0 0;font-size:12px;color:var(--gm-muted)}
@media(max-width:900px){.anomaly-header{align-items:flex-start;flex-wrap:wrap}.anomaly-status-toggle{margin-left:0}.anomaly-row{grid-template-columns:44px minmax(0,1fr) 150px 12px;gap:8px}.anomaly-detail{padding-left:16px}}
@media(max-width:600px){.anomaly-filters{grid-template-columns:1fr 1fr;gap:8px}.anomaly-header p{font-size:12px}.anomaly-view-options{gap:4px 12px}.anomaly-status-toggle{width:100%}.anomaly-row{grid-template-columns:36px minmax(0,1fr) 12px;padding:10px 8px;gap:3px 8px}.anomaly-score{grid-row:1/3}.anomaly-meta{grid-column:2;flex-direction:row;font-size:11px;gap:6px}.anomaly-meta>span:first-child{flex:1}.anomaly-chevron{grid-column:3;grid-row:1/3}.anomaly-detail{padding:10px}.anomaly-title{flex-wrap:wrap;gap:3px 6px}.anomaly-score strong{font-size:17px}.anomaly-reason-preview{font-size:11px}.anomaly-header .anomaly-actions{gap:5px}}
</style>
