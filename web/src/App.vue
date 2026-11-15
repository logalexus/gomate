<template>
 <div id="app" class="gm-app">
  <a href="#workspace-main" class="gm-skip-link" @click.prevent="$refs.main.focus()">Skip to content</a>
  <Navbar/>
  <div class="gm-workspace" :class="{'gm-traffic-workspace':$route.name==='stream'}">
   <router-view v-if="$route.name==='stream'" name="sidebar" ref="sidebar"/>
   <main id="workspace-main" ref="main" class="gm-main" tabindex="-1" :class="{'gm-main-traffic':$route.name==='stream','gm-main-chat':$route.name==='chat','gm-main-anomalies':$route.name==='anomalies'}"><router-view name="content" ref="content"/></main>
  </div>
  <Settings/>
  <AnomalyNotifications/>
  <footer class="capture-status gm-statusbar" aria-label="Capture status">
   <span class="gm-capture-state" :class="{'gm-capture-warning':!captureState.startsWith('LIVE')}"><span class="gm-status-dot" aria-hidden="true"/>{{ captureState }}</span>
   <span class="gm-status-rps" title="Decoded HTTP/1, HTTP/2 and gRPC requests per second · 5-second average · configured services">RPS <strong>{{ rps===null ? '—' : rps.toLocaleString('en-US',{minimumFractionDigits:1,maximumFractionDigits:1}) }}</strong></span>
   <div class="gm-status-metrics"><span>Packets <strong>{{ $store.state.currentPacketsCount.toLocaleString('en-US') }}</strong></span><span>Blocks <strong>{{ $store.state.currentStreamsCount.toLocaleString('en-US') }}</strong></span><span :class="{'text-danger':drops}">Dropped <strong>{{ drops }}</strong></span><span :class="{'text-warning':decodeErrors}">Decode errors <strong>{{ decodeErrors }}</strong></span><span v-if="evictions" class="text-warning">Evictions {{ evictions }}</span></div>
   <span v-if="job&&['queued','running'].includes(job.status)" class="gm-status-job">Search #{{ job.id }} · {{ job.processedPackets }} blocks · {{ job.matchedPackets }} matches <button class="btn btn-sm btn-outline-secondary" @click="cancelJob">Cancel</button></span>
  </footer>
 </div>
</template>
<script>
import Navbar from './components/Navbar';
import Settings from './views/Settings';
import AnomalyNotifications from './components/AnomalyNotifications.vue';
export default {
  components: {Navbar, Settings, AnomalyNotifications},
  data: () => ({socket: null, retry: null, timer: null, stopped: false, dirty: true, refreshing: false, lastRefresh: 0, captureState: 'Connecting…', rps: null, drops: 0, decodeErrors: 0, evictions: 0, job: null, jobsAt: 0}),
  mounted() {
    this.connect();
    this.timer = setInterval(this.refresh, 1000);
    this.refresh();
  },
  beforeDestroy() {
    this.stopped = true;
    clearInterval(this.timer); clearTimeout(this.retry);
    if (this.socket) this.socket.close();
  },
  watch: {
    '$route.name'(){this.$nextTick(()=>{this.$refs.main?.scrollTo(0,0);this.$refs.main?.focus({preventScroll:true})})},
    '$route.params.streamId'(id){if(!id)return;this.$nextTick(()=>{const main=this.$refs.main;main?.scrollTo(0,0);if(window.matchMedia('(max-width:800px)').matches){main?.scrollIntoView({block:'start'});main?.focus({preventScroll:true})}})},
  },
  methods: {
    async cancelJob() {try {await this.$http.delete(`/v2/jobs/${this.job.id}`);this.jobsAt=0;this.dirty=true;} catch(e) {this.$bvToast.toast('Could not cancel the search. Please try again.', {variant:'danger'});}},
    connect() {
      if (this.stopped) return;
      const scheme = location.protocol === 'https:' ? 'wss:' : 'ws:';
      this.socket = new WebSocket(`${scheme}//${location.host}/api/v2/events`);
      this.socket.onopen = () => { this.dirty = true; };
      this.socket.onmessage = () => { this.dirty = true; };
      this.socket.onclose = () => {
        this.captureState = 'Reconnecting…'; this.rps = null;
        if (!this.stopped) this.retry = setTimeout(this.connect, 2000);
      };
    },
    async refresh() {
      if (this.refreshing || (!this.dirty && Date.now() - this.lastRefresh < 5000)) return;
      this.refreshing = true; this.dirty = false;
      try {
        const {data} = await this.$http.get('/v2/snapshot');
        this.rps = typeof data.requestsPerSecond==='number' && Number.isFinite(data.requestsPerSecond) && data.requestsPerSecond>=0 ? data.requestsPerSecond : null;
        this.decodeErrors = data.metrics.decode_errors_total || 0;
 this.evictions = data.metrics.analysis_evictions_total || 0;
 if (Date.now()-this.jobsAt>5000) {const jobs=await this.$http.get('/v2/jobs');this.job=jobs.data[0] || null;this.jobsAt=Date.now();}
 this.$store.commit('setServices', data.services);
        this.$store.commit('setPatterns', data.patterns);
        this.$store.commit('setCurrentPacketsCount', data.metrics.captured_total);
        this.$store.commit('setCurrentStreamsCount', data.metrics.persisted_chunks_total);
        this.$store.state.pcapStarted = data.captureRunning;
        this.drops = ['capture_drops_total', 'queue_drops_total', 'flow_drops_total', 'write_drops_total'].reduce((n, k) => n + data.metrics[k], 0);
        this.captureState = data.capture?.error || (data.stalled ? 'Analysis paused — check storage' : data.captureRunning ? 'LIVE · ' + (data.capture?.interface || '') : 'Capture stopped');
        await Promise.all([this.$refs.sidebar?.refreshLive?.(), this.$refs.content?.refreshLive?.()]);
        this.lastRefresh = Date.now();
      } catch (e) { this.dirty = true; this.captureState = 'Server disconnected'; this.rps = null; }
      finally { this.refreshing = false; }
    },
  },
};
</script>
