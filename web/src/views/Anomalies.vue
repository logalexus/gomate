<template>
 <section class="workspace-page anomalies">
  <header class="anomaly-header">
   <div><h1>Anomalies</h1><p>Behavior fingerprints across saved sessions. Expand a group → select a session → inspect its traffic.</p></div>
   <div class="anomaly-actions">
    <b-button size="sm" variant="outline-secondary" :aria-expanded="String(help)" aria-controls="anomaly-help" @click="help=!help">How it works</b-button>
    <b-button size="sm" variant="outline-secondary" :aria-pressed="String(paused)" :aria-label="paused ? 'Resume auto-refresh' : 'Pause'" @click="paused=!paused">{{ paused ? 'Resume' : 'Pause' }}</b-button>
    <b-button size="sm" variant="outline-primary" :disabled="loading || busy" @click="load()">Refresh</b-button>
   </div>
  </header>

  <div v-if="help" id="anomaly-help" class="gm-panel anomaly-help">
   <ol>
    <li><b>“All sessions”</b> shows both normal traffic and alerts. “By behavior” compares the structure and values of the first 8 incoming messages: parameters, headers and request bodies. Frequently changing values are marked as variable; recurring payloads keep a separate fingerprint. Similar traffic does not prove an attack or identify a team. “By endpoint” groups only by the first method and path. Deleting sessions also deletes their alerts. Purging packet content preserves sessions, anomalies and behavior features. Training the model does not hide groups. Labels show matching FIND patterns: for the whole group in its row, and for the selected session inside it. Matches do not add score and survive packet content cleanup.</li>
    <li><b>Choose a service and period.</b> More unusual sessions appear first by default. Scores add up differences from learned traffic; they are not attack probabilities.</li>
    <li><b>Expand a row.</b> Inspect which field differs and why. Select a specific session within a group. The list stops refreshing while details are open.</li>
    <li><b>“Session stream”</b> shows requests and responses on the right, or in a separate window on narrow screens. Browse block pages to inspect the full stream. A high score alone does not confirm an attack; repeated attacks can become familiar to the model.</li>
    <li><b>“Mark as normal”</b> hides only the selected session and trains the model on its differences. Use it after inspection. “Restore to list” reverses hiding, but not training.</li>
   </ol>
   <p>Enable “Include hidden” to see hidden sessions. The first {{ stats.warmAfter }} service messages are used for warm-up; learning starts over when the application restarts.</p>
  </div>
  <div v-if="error" role="alert" class="anomaly-notice error">{{ error }}</div>
  <div v-if="stats.drops>0" role="status" class="anomaly-notice">The detector skipped {{ stats.drops }} session segments due to overload. Traffic capture continued.</div>

  <div class="anomaly-toolbar">
   <div class="anomaly-filters">
    <b-form-group label="Show" label-for="anomaly-scope"><b-form-select id="anomaly-scope" v-model="scope" size="sm" :options="[{value: 'all', text: 'All sessions'}, {value: 'alerts', text: 'Anomalies only'}]"/></b-form-group>
    <b-form-group v-if="scope==='all'" label="Group" label-for="anomaly-grouping"><b-form-select id="anomaly-grouping" v-model="grouping" size="sm" :options="[{value: 'behavior', text: 'By behavior'}, {value: 'endpoint', text: 'By endpoint'}]"/></b-form-group>
    <b-form-group label="Service" label-for="anomaly-service"><b-form-select id="anomaly-service" v-model="service" size="sm" :options="serviceOptions"/></b-form-group>
    <b-form-group label="Period" label-for="anomaly-period"><b-form-select id="anomaly-period" v-model="period" size="sm" :options="periods"/></b-form-group>
    <b-form-group v-if="scope==='alerts'" label="Minimum score" label-for="anomaly-level"><b-form-select id="anomaly-level" v-model="level" size="sm" :options="levels"/></b-form-group>
    <b-form-group label="Order" label-for="anomaly-sort"><b-form-select id="anomaly-sort" v-model="sort" size="sm" :options="scope==='all' && grouping==='behavior' ? [...sorts, {value: 'count', text: 'Largest groups first'}] : sorts"/></b-form-group>
   </div>
   <div class="anomaly-view-options">
    <span v-if="scope==='all'">{{ grouping==='behavior' ? 'Structure + recurring values · independent of training' : 'Method and path of the first request' }} · until sessions are deleted</span>
    <b-form-checkbox v-if="scope==='alerts'" v-model="group">Group similar sessions</b-form-checkbox>
    <b-form-checkbox v-if="scope==='alerts'" v-model="showDismissed">Include hidden</b-form-checkbox>
    <b-button v-if="service||period!==3600||level!==6||showDismissed||sort!=='score'" size="sm" variant="link" @click="resetFilters">Reset filters</b-button>
    <button type="button" class="anomaly-status-toggle" :aria-expanded="String(modelInfo)" aria-controls="anomaly-model" @click="modelInfo=!modelInfo">Model: {{ stats.services.filter(s=>s.warm).length }}/{{ serviceCards.length }} services warmed up {{ modelInfo ? '▴' : '▾' }}</button>
   </div>
  </div>
  <div v-if="modelInfo" id="anomaly-model" class="gm-panel anomaly-model">
   <ul><li v-for="s in serviceCards" :key="s.port"><b>{{ s.name }} #{{ s.port }}</b> · {{ s.state }} · {{ s.messages }} messages · {{ s.reported }} alerts since startup</li></ul>
  </div>

  <div class="anomaly-results-heading">
   <span>{{ summaryLine }}</span>
   <span role="status">{{ loading ? 'Loading…' : expanded ? 'Paused while inspecting' : paused ? 'Auto-refresh paused' : 'Auto-refresh' }}</span>
  </div>
  <div v-if="!groups.length && !loading" class="gm-panel anomaly-empty">
   <h2>{{ error ? 'Results unavailable' : scope==='all' ? 'No saved sessions match these filters' : 'No alerts match these filters' }}</h2>
   <p v-if="error">Click Refresh to try loading again.</p>
   <p v-else-if="scope==='all'">Check the service, period and capture status. Deleted sessions are not shown here.</p>
   <p v-else-if="!stats.services.some(s => s.warm)">The model is still learning: at least {{ stats.warmAfter }} messages per service are required.</p>
   <p v-else>Increase the period or lower the threshold. No alerts does not mean no attacks.</p>
  </div>

  <div class="anomaly-workspace" :class="{'has-flow':flowOpen && activeSession}">
  <div class="anomaly-list">
   <article v-for="(g, index) in groups" :key="g.key" class="anomaly" :class="{expanded:expanded===g.key, dismissed:g.top.dismissed, alerted:g.anomalyCount>0}">
    <button type="button" class="anomaly-row" :aria-expanded="String(expanded===g.key)" :aria-controls="'anomaly-detail-'+index" @click="toggle(g)">
     <span class="anomaly-score"><strong>{{ g.top.score ? Math.round(g.top.score) : '—' }}</strong><small>{{ scope==='all' && g.top.score ? 'max.' : 'points' }}</small></span>
     <span class="anomaly-description">
      <span class="anomaly-title"><code :title="g.top.summary">{{ endpoint(g.top) }}</code><small v-if="g.fingerprint" class="fingerprint-id">{{ g.fingerprint.slice(0,8) }}</small><span v-if="g.top.dismissed" class="hidden-label">Hidden</span></span>
      <span class="anomaly-reason-preview" :title="g.fingerprint ? behaviorPreview(g) : reasonPreview(g.top)">
       <template v-if="g.fingerprint && !g.missing"><BehaviorFeature v-for="f in previewFields(g)" :key="f" :field="f" compact/><span v-if="!previewFields(g).length" class="preview-structure">Shared structure and values</span></template>
       <span v-else-if="g.fingerprint">No saved behavior features</span>
       <span v-else-if="scope==='all'">{{ g.anomalyCount }} with alerts · {{ g.count-g.anomalyCount-g.dismissedCount }} without alerts</span>
       <template v-else><span v-if="g.top.reasons[0]" class="detector-tag" :class="detectorTone(g.top.reasons[0].detector)">{{ detectors[g.top.reasons[0].detector] || g.top.reasons[0].detector }}</span><span>{{ g.top.reasons[0]?.field }}{{ g.top.reasons[0]?.field ? ' · ' : '' }}{{ g.top.reasons[0]?.text || 'Expand to inspect a session' }}</span></template>
      </span>
      <span v-if="g.fingerprint && !g.missing && g.variableFields?.length" class="anomaly-variable-preview">Variable fields: {{ g.variableFields.length }}</span>
      <PatternLabels v-if="scope==='all'" :ids="g.foundPatternsIds" compact title="Patterns matched in at least one session in this group"/>
     </span>
     <span class="anomaly-meta"><span>{{ serviceName(g.top.service) }}</span><span>{{ time(g.top.startTimestamp) }} · {{ scope==='all' ? g.count+' sessions' : g.items.length>1 ? g.items.length+' sessions' : '#'+g.top.streamId }}</span></span>
     <span class="anomaly-chevron" aria-hidden="true">{{ expanded===g.key ? '▴' : '▾' }}</span>
    </button>
    <div v-if="expanded===g.key" :id="'anomaly-detail-'+index" class="anomaly-detail">
     <section v-if="g.fingerprint" class="behavior-details" aria-label="Group behavior">
      <header class="detail-section-heading"><h2>Shared behavior</h2><code class="fingerprint-id">{{ g.fingerprint.slice(0,8) }}</code></header>
      <div class="behavior-counts"><span><b>{{ g.count }}</b> sessions</span><span class="count-alerts"><b>{{ g.anomalyCount }}</b> with alerts</span><span v-if="g.dismissedCount"><b>{{ g.dismissedCount }}</b> hidden</span></div>
      <p v-if="g.missing" class="anomaly-detail-note">No saved features: the session predates this update or incoming traffic was not parsed. Behavior similarity is unknown.</p>
      <template v-else>
       <details class="common-features" open><summary>Shared features ({{ (g.fields || []).length }})</summary><ul><li v-for="f in g.fields" :key="f"><BehaviorFeature :field="f"/></li></ul><p v-if="!g.fields?.length" class="anomaly-detail-note">No shared saved values.</p></details>
       <div v-if="g.variableFields?.length" class="variable-features"><h3>Variable fields <span>{{ g.variableFields.length }}</span></h3><p>Up to three different examples from recent group sessions. These are saved fragments; long values may have been truncated during capture.</p><dl class="variable-feature-list"><div v-for="f in g.variableFields" :key="f" class="variable-feature"><dt><BehaviorFeature :field="f" variable/></dt><dd><code v-for="(value, i) in g.variableExamples?.[f] || []" :key="i">{{ value === '' ? '(empty)' : value }}</code><span v-if="!g.variableExamples?.[f]?.length">No saved examples</span></dd></div></dl></div>
       <p v-if="g.limited" class="anomaly-detail-note">Comparison limit: first 8 messages, up to 48 fields per message. Other differences are not compared.</p>
      </template>
     </section>
     <section class="selected-session-section" aria-label="Selected session">
     <header class="detail-section-heading"><h2>Selected session</h2><span v-if="g.items.length" class="loaded-session-count">{{ g.items.length }} loaded</span></header>
     <div v-if="scope==='all'" class="anomaly-member-controls">
      <b-form-checkbox v-if="g.anomalyCount" v-model="memberAlerts" @change="loadMembers(g)">With alerts only</b-form-checkbox>
      <span v-if="membersLoading" role="status">Loading sessions…</span>
      <span v-if="membersError" role="alert">{{ membersError }} <button type="button" class="btn btn-sm btn-link" @click="loadMembers(g, !!g.items.length)">Retry</button></span>
      <span v-if="!membersLoading && !membersError && !g.items.length">Sessions were deleted or grouping changed. Refresh the list.</span>
      <b-button v-if="g.nextBefore" size="sm" variant="outline-secondary" :disabled="membersLoading" @click="loadMembers(g, true)">More sessions (loaded {{ g.items.length }})</b-button>
     </div>
     <template v-if="scope!=='all' || g.items.length">
     <div class="anomaly-detail-toolbar">
      <label v-if="g.items.length>1" class="anomaly-session-picker">Session in group
       <select v-model="selectedId" class="custom-select custom-select-sm"><option v-for="a in g.items" :key="a.streamId" :value="a.streamId">#{{ a.streamId }} · {{ time(a.startTimestamp) }} · {{ a.hasAnomaly===false ? 'no alert' : Math.round(a.score)+' points'+(a.dismissed ? ' · hidden' : '') }}</option></select>
      </label>
      <span v-else class="anomaly-session-label">Session #{{ selected(g).streamId }} · {{ time(selected(g).startTimestamp) }}</span>
      <div class="anomaly-actions">
       <b-button size="sm" variant="primary" class="open-session-flow" :aria-expanded="String(flowOpen)" @click="openFlow($event)">Session stream</b-button>
       <b-button size="sm" variant="outline-secondary" :to="streamLink(selected(g))">Open session</b-button>
       <b-button v-if="selected(g).hasAnomaly!==false" size="sm" variant="outline-secondary" :disabled="busy" :title="selected(g).dismissed ? 'Restore this session to the list; training is retained' : 'Hide this session and train the model on its differences'" @click="mark(selected(g))">{{ selected(g).dismissed ? 'Restore to list' : 'Mark as normal' }}</b-button>
      </div>
     </div>
     <div v-if="scope==='all'" class="session-patterns"><span>Session patterns · do not affect the score</span><PatternLabels :ids="selected(g).foundPatternsIds"/><span v-if="!selected(g).foundPatternsIds?.length">No matches</span></div>
     <code class="anomaly-full-summary">{{ selected(g).request || selected(g).summary || g.top.summary }}</code>
     <p v-if="selected(g).hasAnomaly===false" class="anomaly-detail-note">No saved alert · the model may have been warming up or already familiar with this traffic.</p>
     <p v-if="selected(g).payloadPurged" class="anomaly-detail-note">Packet content was purged; session metadata is preserved.</p>
     <p v-if="scope==='all' && selected(g).hasAnomaly" class="anomaly-detail-note">Saved alert: {{ Math.round(selected(g).score) }} points. Scores of older sessions are not recalculated during training.</p>
     <p v-if="selected(g).dismissed" class="anomaly-detail-note">This session alert was hidden manually.</p>
     <h3 v-if="selected(g).reasons?.length" class="reasons-heading">Score reasons <span>{{ Math.round(selected(g).score) }} points</span></h3>
     <ul class="anomaly-reasons">
      <li v-for="(r, i) in selected(g).reasons" :key="i">
       <span class="anomaly-bits">+{{ r.bits.toFixed(1) }}</span>
       <div><b class="detector-tag" :class="detectorTone(r.detector)">{{ detectors[r.detector] || r.detector }}</b> <code v-if="r.field">{{ r.field }}</code><p>{{ r.text }}</p><small v-if="r.template && r.template!==selected(g).summary">{{ r.template }}</small></div>
      </li>
     </ul>
     <p v-if="selected(g).hasAnomaly!==false" class="anomaly-detail-note">Scores measure unusualness, not attack probability. “Mark as normal” applies only to session #{{ selected(g).streamId }}.</p>
    </template>
    </section>
    <p v-if="g.fingerprint && !g.missing" class="anomaly-detail-note group-explanation">Groups survive training. Similar traffic does not identify a team or confirm an attack.</p>
    </div>
   </article>
  </div>
  <AnomalyFlowPanel v-if="flowOpen && activeSession" :session="activeSession" :service-name="serviceName(activeSession.service)" @close="closeFlow"/>
  </div>
  <div v-if="scope==='all' && trafficPage.totalGroups>100" class="anomaly-pagination">
   <b-button size="sm" variant="outline-secondary" :disabled="loading || groupOffset===0" @click="changePage(-100)">Previous groups</b-button>
   <span>{{ groupOffset+1 }}–{{ Math.min(groupOffset+100, trafficPage.totalGroups) }} of {{ trafficPage.totalGroups }}</span>
   <b-button size="sm" variant="outline-secondary" :disabled="loading || groupOffset+100>=trafficPage.totalGroups" @click="changePage(100)">Next groups</b-button>
  </div>
 </section>
</template>

<script>
import PatternLabels from '../components/PatternLabels.vue';
import BehaviorFeature from '../components/BehaviorFeature.vue';
import AnomalyFlowPanel from '../components/AnomalyFlowPanel.vue';
const message = e => e.code === 'ECONNABORTED' ? 'The server did not respond in time. Choose one service or increase the timeout in application settings.' : e.response?.data?.error || e.message;
export default {
 components: {PatternLabels, BehaviorFeature, AnomalyFlowPanel},
 data: () => ({
  scope: 'all', grouping: 'behavior', trafficPage: {groups: [], totalGroups: 0, totalSessions: 0, totalAnomalies: 0}, groupOffset: 0,
  membersLoading: false, membersError: '', memberAlerts: false, memberRequestId: 0,
  items: [], stats: {services: [], drops: 0, warmAfter: 100},
  loading: false, busy: false, error: '', help: false, modelInfo: false, expanded: '', selectedId: null, flowOpen: false,
  paused: false, timer: null, loadedAt: 0, destroyed: false, requestId: 0,
  service: 0, period: 3600, level: 6, sort: 'score', group: true, showDismissed: false,
  detectors: {template: 'Endpoint', field: 'New field', missing: 'Missing field', chars: 'Characters', enum: 'Value', shape: 'Shape', length: 'Length'},
  periods: [{value: 900, text: '15 minutes'}, {value: 3600, text: 'Hour'}, {value: 6 * 3600, text: '6 hours'}, {value: 0, text: 'All time'}],
  levels: [6, 12, 20, 40].map(value => ({value, text: '≥ ' + value + ' points'})),
  sorts: [{value: 'score', text: 'Most unusual first'}, {value: 'time', text: 'Newest first'}],
 }),
 computed: {
  activeSession() { const g=this.groups.find(g=>g.key===this.expanded); return g && g.items.length ? this.selected(g) : null; },
  serviceOptions() {
   return [{value: 0, text: 'All services'}, ...this.$store.state.services.map(s => ({value: s.port, text: `${s.name} #${s.port}`}))];
  },
  serviceCards() {
   const byPort = Object.fromEntries(this.stats.services.map(s => [s.port, s]));
   return this.$store.state.services.map(s => {
    const x = byPort[s.port] || {messages: 0, endpoints: 0, reported: 0, warm: false};
    const state = !x.messages ? 'No traffic' : x.warm ? 'Warmed up' : `Learning ${x.messages}/${this.stats.warmAfter}`;
    return {...x, port: s.port, name: s.name, state};
   });
  },
  groups() {
   if (this.scope==='all') return this.trafficPage.groups;
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
   if (this.scope==='all') return `${this.trafficPage.totalSessions} saved sessions · ${this.trafficPage.totalGroups} groups · ${this.trafficPage.totalAnomalies} with alerts`;
   const n = this.items.length;
   return `${n} sessions${this.group ? ` · ${this.groups.length} groups` : ''}${n>=200 ? ' · limit 200, narrow your filters' : ''}`;
  },
 },
 watch: {
  scope: 'reloadFilters', grouping: 'reloadFilters', service: 'reloadFilters', period: 'reloadFilters', level: 'reloadFilters', sort: 'reloadFilters', showDismissed: 'reloadFilters',
  group() { this.expanded = ''; this.flowOpen=false; },
 },
 mounted() { this.load(); this.timer = setInterval(this.refreshLive, 1000); },
 beforeDestroy() { this.destroyed = true; clearInterval(this.timer); this.listController?.abort(); this.memberController?.abort(); },
 methods: {
  openFlow(event) { this.flowReturnFocus=event?.currentTarget; this.flowOpen=true; },
  closeFlow() { this.flowOpen=false; this.$nextTick(()=>{ const target=this.flowReturnFocus?.isConnected ? this.flowReturnFocus : this.$el.querySelector('.anomaly.expanded .open-session-flow'); target?.focus(); this.flowReturnFocus=null; }); },
  detectorTone(detector) { return ['field','missing','enum','chars'].includes(detector) ? 'tone-warning' : 'tone-info'; },
  previewFields(g) { const fields=(g.fields||[]).filter(f=>/\/(json\.|form\.|query\.|token\.|body)/.test(f) && !f.includes('/json.structure')); return fields.slice(0,2); },
  resetFilters() { this.service=0; this.period=3600; this.level=6; this.showDismissed=false; this.sort='score'; },
  reloadFilters() { this.items = []; this.trafficPage = {groups: [], totalGroups: 0, totalSessions: 0, totalAnomalies: 0}; this.groupOffset=0; return this.load(); },
  changePage(delta) { this.groupOffset=Math.max(0,this.groupOffset+delta); return this.load(); },
  refreshLive() {
   if (!document.hidden && !this.paused && !this.expanded && !this.busy && !this.loading && Date.now()-this.loadedAt>3000) return this.load(false);
  },
  async load(closeDetails = true) {
   if (closeDetails) { this.expanded = ''; this.flowOpen=false; ++this.memberRequestId; this.memberController?.abort(); }
   this.listController?.abort();
   const controller = new AbortController();
   this.listController = controller;
   const id = ++this.requestId;
   this.loading = true;
   try {
    const params = {minScore: this.level, period: this.period, sort: this.sort};
    if (this.service) params.service = this.service;
    if (this.showDismissed) params.dismissed = 1;
    const [list, stats] = await Promise.all([this.$http.get(this.scope==='all' ? '/v2/anomalies/groups' : '/v2/anomalies', {signal: controller.signal, params: this.scope==='all' ? {service: this.service, period: this.period, sort: this.sort, grouping: this.grouping, limit: 100, offset: this.groupOffset} : params}), this.$http.get('/v2/anomalies/stats', {signal: controller.signal})]);
    if (this.destroyed || id !== this.requestId) return;
    if (this.scope==='all') {
     this.trafficPage = {...list.data, groups: list.data.groups.map(g => ({...g,
      key: g.fingerprint || JSON.stringify([g.service, g.protocol, g.method, g.path]), items: [], nextBefore: 0,
      top: {service: g.service, protocol: g.protocol, score: g.score, startTimestamp: g.startTimestamp, reasons: [], summary: g.path ? `${g.method} ${g.path}`.trim() : `${g.protocol} · no request path`},
     }))};
     if (this.groupOffset && !this.trafficPage.groups.length && this.trafficPage.totalGroups<=this.groupOffset) {
      this.groupOffset=0; return this.load();
     }
    } else this.items = list.data;
    this.stats = stats.data;
    this.error = '';
   } catch (e) {
    if (!this.destroyed && id === this.requestId) {
     this.items = [];
     this.trafficPage = {groups: [], totalGroups: 0, totalSessions: 0, totalAnomalies: 0};
     this.error = 'Could not load list: ' + message(e);
    }
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
    this.$bvToast.toast(message(e), {title: 'Could not save', variant: 'danger'});
   } finally { this.busy = false; }
  },
  toggle(g) {
   // Ignore an in-flight live response so opening details always freezes this list.
   ++this.requestId;
   this.listController?.abort();
   this.memberController?.abort();
   this.loading = false;
   this.expanded = this.expanded === g.key ? '' : g.key;
   this.flowOpen=false;
   this.selectedId = g.top.streamId;
   ++this.memberRequestId;
   this.membersLoading=false;
   this.membersError='';
   this.memberAlerts=false;
   if (this.scope==='all' && this.expanded) this.loadMembers(g);
  },
  async loadMembers(g, more = false) {
   const id=++this.memberRequestId;
   this.memberController?.abort();
   const controller = new AbortController();
   this.memberController = controller;
   this.membersLoading=true; this.membersError='';
   if (!more) { g.items=[]; g.nextBefore=0; this.selectedId=null; }
   try {
    const {data}=await this.$http.get('/v2/anomalies/sessions', {signal: controller.signal, params: {fingerprint:g.fingerprint, service:g.service, protocol:g.protocol, method:g.method, path:g.path, period:this.period, limit:50, before:more ? g.nextBefore : 0, alerts:this.memberAlerts ? 1 : 0}});
    if (this.destroyed || id!==this.memberRequestId || this.expanded!==g.key) return;
    g.items=more ? [...g.items, ...data.items] : data.items; g.nextBefore=data.nextBefore;
    if (!more) this.selectedId=g.items[0]?.streamId;
   } catch(e) {
    if (!this.destroyed && id===this.memberRequestId) this.membersError='Could not load sessions: '+message(e);
   } finally { if (id===this.memberRequestId) this.membersLoading=false; }
  },
  selected(g) { return g.items.find(a => a.streamId === this.selectedId) || g.top; },
  behaviorPreview(g) {
   if (g.missing) return 'No behavior features · request endpoint only';
   const fields = (g.fields || []).filter(f=>/\/(json\.|form\.|query\.|token\.|body)/.test(f) && !f.includes('/json.structure'));
   return (fields.length ? fields.slice(0,2).join(' · ') : 'Shared structure and values') + (g.variableFields?.length ? ` · ${g.variableFields.length} variable fields` : '');
  },
  endpoint(a) { return a.reasons.find(r => r.template)?.template || a.summary.split('?')[0]; },
  reasonPreview(a) {
   const r = a.reasons[0];
   return r ? `${this.detectors[r.detector] || r.detector}${r.field ? ': '+r.field : ''} · ${r.text}` : 'Expand to inspect a session';
  },
  streamLink(a) { return {name: 'stream', params: {servicePort: String(a.service), streamId: String(a.streamId)}}; },
  serviceName(port) { const s = this.$store.state.services.find(x => x.port === port); return s ? `${s.name} #${port}` : `#${port}`; },
  time(ms) { return new Date(ms).toLocaleTimeString('en-US'); },
 },
};
</script>

<style scoped>
.session-patterns {font-size:11px;color:var(--gm-muted);margin-bottom:8px;display:flex;flex-wrap:wrap;align-items:center;gap:4px 8px}
.fingerprint-id {font-size:10px;color:var(--gm-muted);margin-left:6px;flex-shrink:0}
.anomaly-workspace{display:grid;grid-template-columns:minmax(0,1fr);gap:12px;align-items:start}
.anomaly-workspace.has-flow{grid-template-columns:minmax(0,.95fr) minmax(0,1.05fr)}
.behavior-details,.selected-session-section{font-size:12px;overflow-wrap:anywhere;border:1px solid var(--gm-border);border-radius:6px;padding:10px 12px;background:var(--gm-panel)}
.behavior-details{margin-bottom:10px}
.detail-section-heading{display:flex;justify-content:space-between;align-items:baseline;gap:8px;margin-bottom:8px}
.detail-section-heading h2{font-size:13px;font-weight:650;margin:0}
.loaded-session-count{color:var(--gm-muted);font-size:11px}
.behavior-counts{display:flex;flex-wrap:wrap;gap:5px 14px;color:var(--gm-muted);margin-bottom:10px;font-variant-numeric:tabular-nums}
.behavior-counts b{color:var(--gm-text)}.behavior-counts .count-alerts,.behavior-counts .count-alerts b{color:var(--gm-warning)}
.common-features summary{cursor:pointer;font-weight:600;padding:4px 0;color:var(--gm-info)}
.common-features ul{padding:0;margin:6px 0;list-style:none;max-height:200px;overflow:auto;overscroll-behavior:contain}
.common-features li{padding:6px 0}.common-features li+li{border-top:1px solid var(--gm-border)}
.variable-features{margin-top:8px;padding-top:10px;border-top:1px solid var(--gm-border)}
.variable-features h3,.reasons-heading{font-size:12px;font-weight:600;margin:0 0 6px;color:var(--gm-text)}
.variable-features h3 span,.reasons-heading span{margin-left:5px;color:var(--gm-muted);font-weight:400}
.variable-features p{color:var(--gm-muted);margin:0 0 8px}
.variable-feature-list{display:grid;gap:8px;margin:0}
.variable-feature{min-width:0}.variable-feature dt{font-weight:400}.variable-feature dd{display:flex;flex-wrap:wrap;gap:4px 12px;margin:3px 0 0;color:var(--gm-muted)}.variable-feature dd code{max-width:100%;color:var(--gm-text);font-size:11px;white-space:pre-wrap;overflow-wrap:anywhere}
.group-explanation{padding:0 2px}
.reasons-heading{display:flex;justify-content:space-between;align-items:baseline;margin:14px 0 0}
.detector-tag{display:inline-block;font-size:11px;font-weight:600;padding:1px 5px;border-radius:4px;flex-shrink:0}
.tone-warning{color:var(--gm-warning);background:var(--gm-warning-soft)}
.tone-info,.preview-structure{color:var(--gm-info);background:var(--gm-info-soft)}
.preview-structure{font-size:11px;border-radius:4px;padding:1px 5px;flex-shrink:0}
.anomaly-variable-preview{display:inline-block;margin-top:5px;padding:1px 5px;border-radius:4px;font-size:11px;line-height:1.5;color:var(--gm-warning);background:var(--gm-warning-soft)}

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
.anomaly-filters{display:grid;grid-template-columns:repeat(auto-fit,minmax(135px,1fr));gap:12px}
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
.anomaly-description,.anomaly-meta{min-width:0}.anomaly-title{display:flex;gap:8px;align-items:center;min-width:0}
.anomaly-title code{font-family:var(--gm-mono);font-size:13px;color:var(--gm-text);overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.hidden-label{font-size:11px;color:var(--gm-muted)}
.anomaly-reason-preview{display:flex;align-items:center;gap:5px 8px;overflow:hidden;white-space:nowrap;color:var(--gm-muted);font-size:12px;margin-top:3px;max-width:100%}.anomaly-reason-preview>span{min-width:0;overflow:hidden;text-overflow:ellipsis}.anomaly-reason-preview>.behavior-feature{flex:0 1 auto}
.anomaly-meta{display:flex;flex-direction:column;gap:3px;color:var(--gm-muted);font-size:12px}.anomaly-meta>span{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.anomaly-chevron{color:var(--gm-muted)}
.anomaly.alerted>.anomaly-row{box-shadow:inset 3px 0 var(--gm-warning)}
.anomaly-member-controls,.anomaly-pagination{display:flex;align-items:center;flex-wrap:wrap;gap:8px;font-size:12px;color:var(--gm-muted);margin:8px 0}
.anomaly-detail{padding:10px 12px;border-top:1px solid var(--gm-border);background:var(--gm-panel-alt)}
.anomaly-detail-toolbar{display:flex;align-items:center;justify-content:space-between;flex-wrap:wrap;gap:10px;margin-bottom:10px}
.anomaly-session-picker{display:flex;align-items:center;flex-wrap:wrap;gap:6px;margin:0;font-size:12px;color:var(--gm-muted)}.anomaly-session-picker select{width:auto;max-width:100%;font-size:12px}.anomaly-session-label{color:var(--gm-muted);font-size:12px}
.anomaly-full-summary{display:block;overflow-wrap:anywhere;white-space:pre-wrap;color:var(--gm-text);font-size:12px;background:var(--gm-panel-alt);border-radius:4px;padding:8px}
.anomaly-reasons{list-style:none;padding:0;margin:10px 0}.anomaly-reasons li{display:grid;grid-template-columns:44px minmax(0,1fr);gap:8px;padding:6px 0;font-size:13px;overflow-wrap:anywhere}.anomaly-reasons li+li{border-top:1px solid var(--gm-border)}.anomaly-reasons p{margin:3px 0 0}.anomaly-reasons small{color:var(--gm-muted)}.anomaly-reasons code{color:var(--gm-text);font-size:12px}
.anomaly-bits{font-family:var(--gm-mono);font-size:12px;color:var(--gm-muted);padding-top:2px}.anomaly-detail-note{margin:8px 0 0;font-size:12px;color:var(--gm-muted);line-height:1.5}
@media(min-width:1200px){.has-flow .anomaly-row{grid-template-columns:40px minmax(0,1fr) 12px;gap:4px 8px}.has-flow .anomaly-score{grid-row:1/3}.has-flow .anomaly-meta{grid-column:2;flex-direction:row;gap:6px}.has-flow .anomaly-chevron{grid-column:3;grid-row:1/3}.has-flow .anomaly-session-picker{width:100%}.has-flow .anomaly-session-picker select{flex:1;min-width:0}}
@media(max-width:1199px){.anomaly-workspace.has-flow{grid-template-columns:minmax(0,1fr)}}
@media(max-width:900px){.anomaly-header{align-items:flex-start;flex-wrap:wrap}.anomaly-status-toggle{margin-left:0}.anomaly-row{grid-template-columns:44px minmax(0,1fr) 150px 12px;gap:8px}.anomaly-detail{padding-left:16px}}
@media(max-width:600px){.anomaly-filters{grid-template-columns:1fr 1fr;gap:8px}.anomaly-header p{font-size:12px}.anomaly-view-options{gap:4px 12px}.anomaly-status-toggle{width:100%}.anomaly-row{grid-template-columns:36px minmax(0,1fr) 12px;padding:10px 8px;gap:3px 8px}.anomaly-score{grid-row:1/3}.anomaly-meta{grid-column:2;flex-direction:row;font-size:11px;gap:6px}.anomaly-meta>span:first-child{flex:1}.anomaly-chevron{grid-column:3;grid-row:1/3}.anomaly-detail{padding:10px}.anomaly-title{flex-wrap:wrap;gap:3px 6px}.anomaly-score strong{font-size:17px}.anomaly-reason-preview{font-size:11px}.anomaly-header .anomaly-actions{gap:5px}}
</style>
