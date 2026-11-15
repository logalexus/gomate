<template>
 <section class="monitor-page">
  <header class="monitor-heading">
   <h1>Monitoring</h1>
   <span class="monitor-status" :class="{'text-warning':stale||paused}">{{ paused?'Pause':stale?'No recent data':'LIVE · 5 s' }}<span v-if="data.sampledAt"> · {{ time(data.sampledAt) }}</span></span>
   <div class="monitor-controls"><label for="monitor-period">Period</label><select id="monitor-period" v-model.number="minutes" class="custom-select custom-select-sm"><option :value="5">5 minutes</option><option :value="15">15 minutes</option><option :value="60">1 hour</option></select><button class="btn btn-sm btn-outline-secondary" :aria-pressed="String(paused)" @click="togglePause">{{ paused?'Resume':'Pause' }}</button><button class="btn btn-sm btn-outline-primary" :disabled="loading" @click="load">Refresh</button></div>
  </header>
  <b-alert :show="!!error" variant="danger">{{ error }}</b-alert>
  <b-alert :show="!!data.error" variant="warning">{{ data.error }}</b-alert>
  <b-alert :show="!!host.error" variant="warning">{{ host.error }}</b-alert>
  <div v-if="!data.sampledAt&&loading" class="monitor-loading" role="status">Loading metrics…</div>
  <template v-if="data.sampledAt">
   <div class="monitor-section-heading"><h2>Host</h2><span>{{ host.cpus||'—' }} logical CPUs · Load {{ number(host.load1) }} · uptime {{ uptime }}</span><div class="monitor-load-legend" aria-label="Load levels"><span class="gm-load-normal"><i aria-hidden="true"/> &lt; 70%</span><span class="gm-load-warning"><i aria-hidden="true"/> ≥ 70%</span><span class="gm-load-danger"><i aria-hidden="true"/> ≥ 90%</span></div></div>
   <div class="monitor-kpis">
    <div class="monitor-kpi" :class="loadClass(host.cpu,100)"><span>Host CPU</span><strong>{{ format(host.cpu,'percent') }}</strong><small>100% = all cores</small><ResourceLoad :value="host.cpu" :capacity="100" label="Host CPU"/></div>
    <div class="monitor-kpi" :class="loadClass(host.memoryUsed,host.memoryLimit)"><span>Memory</span><strong>{{ format(host.memoryUsed) }}</strong><small>of {{ format(host.memoryLimit) }}</small><ResourceLoad :value="host.memoryUsed" :capacity="host.memoryLimit" label="Host memory"/></div>
    <div class="monitor-kpi" :class="loadClass(host.diskUsed,host.diskTotal)"><span>Disk /</span><strong>{{ format(host.diskUsed) }}</strong><small>available {{ format(host.diskAvailable) }}</small><ResourceLoad :value="host.diskUsed" :capacity="host.diskTotal" label="Host disk"/></div>
    <div class="monitor-kpi monitor-kpi-containers"><span>Containers</span><strong>{{ running }} <small>/ {{ containers.length }}</small></strong><small>running / total</small></div>
   </div>
   <div class="monitor-charts">
    <ResourceChart title="CPU" subtitle="Share of host CPU capacity" :points="data.hostHistory" :series="cpu" unit="percent" :ceiling="100" :capacity="100" :start="start" :end="end"/>
    <ResourceChart title="RAM" subtitle="Excludes reclaimable cache" :points="data.hostHistory" :series="memory" :ceiling="host.memoryLimit||0" :capacity="host.memoryLimit" :start="start" :end="end"/>
    <ResourceChart title="Network" :subtitle="(host.interfaces||[]).join(', ')||'Interfaces unavailable'" :points="data.hostHistory" :series="network" unit="rate" :start="start" :end="end"/>
    <ResourceChart title="Disk /" subtitle="Used filesystem space" :points="data.hostHistory" :series="disk" :ceiling="host.diskTotal||0" :capacity="host.diskTotal" :start="start" :end="end"/>
   </div>
   <p v-if="host.diskError||host.networkError" class="monitor-note text-warning">{{ [host.diskError,host.networkError].filter(Boolean).join(' · ') }}</p>
   <div class="monitor-section-heading monitor-container-heading"><h2>Containers</h2><span>CPU: 100% = one core · color shows share of all host cores</span><label class="sr-only" for="monitor-search">Search containers</label><input id="monitor-search" v-model="query" class="form-control form-control-sm" placeholder="Name or image"/><label class="sr-only" for="monitor-sort">Sort containers</label><select id="monitor-sort" v-model="sort" class="custom-select custom-select-sm"><option value="cpu">By CPU</option><option value="memoryUsed">By RAM</option><option value="name">By name</option></select></div>
   <p v-if="data.truncated" class="monitor-note text-warning">Showing the first 128 containers by name: the metric collection limit.</p>
   <div class="monitor-table-wrap gm-panel"><table class="table monitor-table">
    <caption class="sr-only">Container resources. Select a name to view charts.</caption>
    <thead><tr><th scope="col">Container</th><th scope="col">CPU</th><th scope="col">RAM / limit</th><th scope="col">Network ↓ / ↑</th><th scope="col">Disk read / write</th><th scope="col">PID</th></tr></thead>
    <tbody><tr v-for="c in pageItems" :key="c.id" :class="{'is-selected':selectedId===c.id}" :data-monitor-container="c.name">
     <td><button class="monitor-container-select" :aria-pressed="String(selectedId===c.id)" @click="select(c.id)">{{ c.name }}</button><span class="monitor-container-state" :class="{'text-warning':c.error}">{{ c.error||state(c.state) }}</span></td>
     <td class="monitor-resource-cell" :class="loadClass(c.point.cpu,cpuCapacity)"><strong>{{ format(c.point.cpu,'percent') }}</strong><ResourceLoad :value="c.point.cpu" :capacity="cpuCapacity" :label="'CPU '+c.name" basis="of host"/></td><td class="monitor-resource-cell" :class="loadClass(c.point.memoryUsed,c.point.memoryLimit)"><strong>{{ format(c.point.memoryUsed) }}</strong> <span class="text-muted">/ {{ format(c.point.memoryLimit) }}</span><ResourceLoad :value="c.point.memoryUsed" :capacity="c.point.memoryLimit" :label="'RAM '+c.name" basis="of limit"/></td><td><span class="monitor-direction-in">{{ format(c.point.rx,'rate') }}</span> / <span class="monitor-direction-out">{{ format(c.point.tx,'rate') }}</span></td><td><span class="monitor-direction-in">{{ format(c.point.read,'rate') }}</span> / <span class="monitor-direction-out">{{ format(c.point.write,'rate') }}</span></td><td>{{ c.error||!['running','paused'].includes(c.state)?'—':c.point.pids }}</td>
    </tr><tr v-if="!filtered.length"><td colspan="6">{{ containers.length?'No matches':'No containers yet' }}</td></tr></tbody>
   </table></div>
   <div class="monitor-table-footer"><span>{{ filtered.length }} containers · RAM excludes inactive file cache · — metric unavailable</span><div v-if="filtered.length>20"><button class="btn btn-sm btn-outline-secondary" :disabled="page<=1" aria-label="Previous page" @click="page--">‹</button><span>{{ page }} / {{ Math.ceil(filtered.length/20) }}</span><button class="btn btn-sm btn-outline-secondary" :disabled="page>=Math.ceil(filtered.length/20)" aria-label="Next page" @click="page++">›</button></div></div>
   <section v-if="selected" class="monitor-detail" aria-label="Container charts">
    <div class="monitor-section-heading"><h2>{{ selected.name }}</h2><span>{{ state(selected.state) }}</span><router-link :to="{name:'containers'}">Manage containers <i class="fas fa-arrow-right" aria-hidden="true"/></router-link><button class="btn btn-sm btn-outline-secondary" aria-label="Close container charts" @click="select('')">Close</button></div>
    <p class="monitor-note">{{ selected.image }}</p>
    <div v-if="detailLoading" class="monitor-loading" role="status">Loading container charts…</div>
    <div v-else class="monitor-charts">
     <ResourceChart title="Container CPU" subtitle="100% = one core · color relative to host capacity" :points="data.containerHistory" :series="cpu" unit="percent" :ceiling="cpuCapacity||100" :capacity="cpuCapacity" :start="start" :end="end"/>
     <ResourceChart title="Container RAM" :subtitle="'Limit '+format(selected.point.memoryLimit)" :points="data.containerHistory" :series="memory" :ceiling="selected.point.memoryLimit||0" :capacity="selected.point.memoryLimit" :start="start" :end="end"/>
     <ResourceChart title="Container network" subtitle="Receive and transmit" :points="data.containerHistory" :series="network" unit="rate" :start="start" :end="end"/>
     <ResourceChart title="Container disk" subtitle="Block I/O" :points="data.containerHistory" :series="io" unit="rate" :start="start" :end="end"/>
    </div>
   </section>
   <p class="monitor-note">The last hour of history is held in memory and resets when the gateway restarts. Host network excludes loopback, veth and Docker bridges. Separate network counters may be unavailable for containers using host networking.</p>
  </template>
 </section>
</template>
<script>
import ResourceChart from '../components/ResourceChart.vue';
import ResourceLoad from '../components/ResourceLoad.vue';
import {formatMetric,resourceLoad} from '../monitor.mjs';
export default {
 name:'Monitoring',components:{ResourceChart,ResourceLoad},
 data:()=>({data:{containers:[],hostHistory:[],containerHistory:[]},minutes:15,selectedId:'',query:'',sort:'cpu',page:1,paused:false,loading:false,error:'',now:Date.now(),timer:null,controller:null,sequence:0,destroyed:false,
  cpu:[{key:'cpu',label:'CPU'}],memory:[{key:'memoryUsed',label:'Used'}],network:[{key:'rx',label:'Receive'},{key:'tx',label:'Transmit'}],disk:[{key:'diskUsed',label:'Used'}],io:[{key:'read',label:'Read'},{key:'write',label:'Write'}]}),
 computed:{
  host(){return this.data.host||{}},containers(){return this.data.containers||[]},running(){return this.containers.filter(c=>c.state==='running').length},
  cpuCapacity(){return Number.isFinite(this.host.cpus)&&this.host.cpus>0?this.host.cpus*100:null},
  stale(){return !this.data.sampledAt||this.now-this.data.sampledAt>15000||!!this.error},
  end(){return this.data.sampledAt||this.now},start(){return this.end-this.minutes*60000},
  selected(){return this.containers.find(c=>c.id===this.selectedId)},
  detailLoading(){return this.data.selectedId!==this.selectedId},
  filtered(){const q=this.query.trim().toLowerCase();return this.containers.filter(c=>(c.name+' '+c.image).toLowerCase().includes(q)).sort((a,b)=>this.sort==='name'?a.name.localeCompare(b.name):(b.point[this.sort]??-1)-(a.point[this.sort]??-1)||a.name.localeCompare(b.name))},
  pageItems(){return this.filtered.slice((this.page-1)*20,this.page*20)},
  uptime(){const s=this.host.uptimeSeconds;if(s==null)return '—';return s>=86400?Math.floor(s/86400)+' d '+Math.floor(s%86400/3600)+' h':Math.floor(s/3600)+' h '+Math.floor(s%3600/60)+' min'},
 },
 watch:{minutes(){this.load()},query(){this.page=1},sort(){this.page=1},'containers.length'(){this.page=Math.min(this.page,Math.max(1,Math.ceil(this.filtered.length/20)))}},
 mounted(){this.load();this.timer=setInterval(()=>{this.now=Date.now();if(!this.paused&&!document.hidden&&!this.loading)this.load()},5000);document.addEventListener('visibilitychange',this.visibility)},
 beforeDestroy(){this.destroyed=true;clearInterval(this.timer);this.controller?.abort();document.removeEventListener('visibilitychange',this.visibility)},
 methods:{
  format:formatMetric,number(v){return v==null?'—':v.toFixed(2)},time(v){return new Date(v).toLocaleTimeString('en-US')},
  loadClass(value,capacity){return 'gm-load-'+resourceLoad(value,capacity).level},
  state(v){return ({running:'Running',paused:'Paused',exited:'Stopped',created:'Created',restarting:'Restarting',dead:'Error'})[v]||v},
  select(id){this.selectedId=id;this.load()},
  togglePause(){this.paused=!this.paused;if(this.paused)this.controller?.abort();else this.load()},
  visibility(){if(document.hidden)this.controller?.abort();else if(!this.paused)this.load()},
  async load(){
   if(this.destroyed)return;this.controller?.abort();const seq=++this.sequence;this.controller=new AbortController();this.loading=true;
   try{const {data}=await this.$http.get('/v2/monitor',{params:{minutes:this.minutes,container:this.selectedId},signal:this.controller.signal});if(seq!==this.sequence||this.destroyed)return;this.data=data;this.error='';this.now=Date.now();}
   catch(e){if(seq===this.sequence&&e.code!=='ERR_CANCELED'&&!this.destroyed)this.error=e.response?.data?.error||'Could not refresh monitoring. Last data retained.'}
   finally{if(seq===this.sequence)this.loading=false}
  },
 },
};
</script>
<style scoped>
.monitor-page{max-width:1680px;margin:0 auto}.monitor-heading{display:flex;align-items:center;gap:12px;flex-wrap:wrap;margin-bottom:14px}.monitor-heading h1{font-size:20px;margin:0;font-weight:700}.monitor-status{font-size:12px;color:var(--gm-accent)}.monitor-controls{margin-left:auto;display:flex;gap:8px;align-items:center}.monitor-controls label{margin:0;color:var(--gm-muted);font-size:12px}.monitor-controls select{width:110px}.monitor-section-heading{display:flex;align-items:center;gap:12px;margin:14px 0 10px;flex-wrap:wrap}.monitor-section-heading h2{font-size:15px;font-weight:650;margin:0;overflow-wrap:anywhere}.monitor-section-heading>span{color:var(--gm-muted);font-size:12px}.monitor-kpis{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:10px;margin-bottom:10px}.monitor-kpis>div{background:var(--gm-panel);border:1px solid var(--gm-border);border-radius:var(--gm-radius);padding:10px 12px;display:flex;flex-direction:column;gap:3px}.monitor-kpis span,.monitor-kpis small{color:var(--gm-muted);font-size:12px}.monitor-kpis strong{font-size:23px;line-height:1.2;font-variant-numeric:tabular-nums;font-weight:650}.monitor-charts{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:10px}.monitor-container-heading input{max-width:220px;margin-left:auto}.monitor-container-heading select{width:130px}.monitor-table-wrap{overflow:auto}.monitor-table{margin:0;font-size:12px;white-space:nowrap}.monitor-table th{color:var(--gm-muted);font-weight:600}.monitor-table td,.monitor-table th{padding:8px 10px;vertical-align:middle}.monitor-table tr.is-selected{background:var(--gm-accent-soft)}.monitor-table td:first-child{min-width:180px;max-width:360px}.monitor-container-select{display:block;background:none;border:0;padding:0;max-width:100%;white-space:normal;overflow-wrap:anywhere;text-align:left;color:var(--gm-accent);font-weight:600;cursor:pointer;min-height:24px}.monitor-container-state{display:block;font-size:11px;color:var(--gm-muted)}.monitor-table-footer{display:flex;align-items:center;justify-content:space-between;gap:12px;margin-top:6px;color:var(--gm-muted);font-size:12px}.monitor-table-footer>div{display:flex;align-items:center;gap:8px}.monitor-detail{border-top:1px solid var(--gm-border);margin-top:16px}.monitor-detail .monitor-section-heading a{margin-left:auto;font-size:12px}.monitor-note{color:var(--gm-muted);font-size:12px;margin:12px 0;overflow-wrap:anywhere}.monitor-loading{padding:20px;color:var(--gm-muted)}
.monitor-kpis>.monitor-kpi{border-top:3px solid var(--gm-load-color);background:linear-gradient(135deg,var(--gm-load-soft),var(--gm-panel) 75%)}.monitor-kpi>strong{color:var(--gm-load-color)}.monitor-kpi-containers{--gm-load-color:var(--gm-info);--gm-load-soft:var(--gm-info-soft)}.monitor-kpi .resource-load{margin-top:5px}.monitor-load-legend{display:flex;gap:10px;align-items:center;margin-left:auto;font-size:11px;flex-wrap:wrap}.monitor-load-legend span{display:inline-flex;align-items:center;gap:5px;color:var(--gm-load-color)}.monitor-load-legend i{width:6px;height:6px;border-radius:50%;background:currentColor}.monitor-resource-cell{min-width:160px}.monitor-resource-cell>strong{color:var(--gm-load-color);font-weight:600}.monitor-resource-cell .resource-load{margin-top:4px}.monitor-direction-in{color:var(--gm-info)}.monitor-direction-out{color:var(--gm-accent)}
@media(min-width:1400px){.monitor-charts{grid-template-columns:repeat(4,minmax(0,1fr))}}
@media(max-width:700px){.monitor-heading{gap:8px}.monitor-controls{margin-left:0;flex-wrap:wrap;width:100%}.monitor-kpis{grid-template-columns:repeat(2,minmax(0,1fr))}.monitor-charts{grid-template-columns:1fr}.monitor-container-heading input{margin-left:0;max-width:none;flex:1;min-width:120px}.monitor-table-footer{align-items:flex-start;flex-direction:column}.monitor-detail .monitor-section-heading a{margin-left:0}.monitor-status{font-size:11px}}
</style>
