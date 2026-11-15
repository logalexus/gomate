<template>
 <section class="monitor-page">
  <header class="monitor-heading">
   <h1>Мониторинг</h1>
   <span class="monitor-status" :class="{'text-warning':stale||paused}">{{ paused?'Пауза':stale?'Нет свежих данных':'LIVE · 5 с' }}<span v-if="data.sampledAt"> · {{ time(data.sampledAt) }}</span></span>
   <div class="monitor-controls"><label for="monitor-period">Период</label><select id="monitor-period" v-model.number="minutes" class="custom-select custom-select-sm"><option :value="5">5 минут</option><option :value="15">15 минут</option><option :value="60">1 час</option></select><button class="btn btn-sm btn-outline-secondary" :aria-pressed="String(paused)" @click="togglePause">{{ paused?'Продолжить':'Пауза' }}</button><button class="btn btn-sm btn-outline-primary" :disabled="loading" @click="load">Обновить</button></div>
  </header>
  <b-alert :show="!!error" variant="danger">{{ error }}</b-alert>
  <b-alert :show="!!data.error" variant="warning">{{ data.error }}</b-alert>
  <b-alert :show="!!host.error" variant="warning">{{ host.error }}</b-alert>
  <div v-if="!data.sampledAt&&loading" class="monitor-loading" role="status">Загружаем показатели…</div>
  <template v-if="data.sampledAt">
   <div class="monitor-section-heading"><h2>Хост</h2><span>{{ host.cpus||'—' }} логических CPU · Load {{ number(host.load1) }} · работает {{ uptime }}</span></div>
   <div class="monitor-kpis">
    <div><span>CPU хоста</span><strong>{{ format(host.cpu,'percent') }}</strong><small>100% — все ядра</small></div>
    <div><span>Память</span><strong>{{ format(host.memoryUsed) }}</strong><small>из {{ format(host.memoryLimit) }}</small></div>
    <div><span>Диск /</span><strong>{{ format(host.diskUsed) }}</strong><small>доступно {{ format(host.diskAvailable) }}</small></div>
    <div><span>Контейнеры</span><strong>{{ running }} <small>/ {{ containers.length }}</small></strong><small>работают / всего</small></div>
   </div>
   <div class="monitor-charts">
    <ResourceChart title="CPU" subtitle="Доля мощности хоста" :points="data.hostHistory" :series="cpu" unit="percent" :ceiling="100" :start="start" :end="end"/>
    <ResourceChart title="RAM" subtitle="Без доступного кеша" :points="data.hostHistory" :series="memory" :ceiling="host.memoryLimit||0" :start="start" :end="end"/>
    <ResourceChart title="Сеть" :subtitle="(host.interfaces||[]).join(', ')||'Интерфейсы недоступны'" :points="data.hostHistory" :series="network" unit="rate" :start="start" :end="end"/>
    <ResourceChart title="Диск /" subtitle="Занято на файловой системе" :points="data.hostHistory" :series="disk" :ceiling="host.diskTotal||0" :start="start" :end="end"/>
   </div>
   <p v-if="host.diskError||host.networkError" class="monitor-note text-warning">{{ [host.diskError,host.networkError].filter(Boolean).join(' · ') }}</p>
   <div class="monitor-section-heading monitor-container-heading"><h2>Контейнеры</h2><span>CPU: 100% = одно ядро</span><label class="sr-only" for="monitor-search">Поиск контейнеров</label><input id="monitor-search" v-model="query" class="form-control form-control-sm" placeholder="Имя или образ"/><label class="sr-only" for="monitor-sort">Сортировка контейнеров</label><select id="monitor-sort" v-model="sort" class="custom-select custom-select-sm"><option value="cpu">По CPU</option><option value="memoryUsed">По RAM</option><option value="name">По имени</option></select></div>
   <p v-if="data.truncated" class="monitor-note text-warning">Показаны первые 128 контейнеров по имени — предел сбора метрик.</p>
   <div class="monitor-table-wrap gm-panel"><table class="table monitor-table">
    <caption class="sr-only">Ресурсы контейнеров. Выберите имя для просмотра графиков.</caption>
    <thead><tr><th scope="col">Контейнер</th><th scope="col">CPU</th><th scope="col">RAM / лимит</th><th scope="col">Сеть ↓ / ↑</th><th scope="col">Диск чтение / запись</th><th scope="col">PID</th></tr></thead>
    <tbody><tr v-for="c in pageItems" :key="c.id" :class="{'is-selected':selectedId===c.id}" :data-monitor-container="c.name">
     <td><button class="monitor-container-select" :aria-pressed="String(selectedId===c.id)" @click="select(c.id)">{{ c.name }}</button><span class="monitor-container-state" :class="{'text-warning':c.error}">{{ c.error||state(c.state) }}</span></td>
     <td>{{ format(c.point.cpu,'percent') }}</td><td>{{ format(c.point.memoryUsed) }} <span class="text-muted">/ {{ format(c.point.memoryLimit) }}</span></td><td>{{ format(c.point.rx,'rate') }} / {{ format(c.point.tx,'rate') }}</td><td>{{ format(c.point.read,'rate') }} / {{ format(c.point.write,'rate') }}</td><td>{{ c.error||!['running','paused'].includes(c.state)?'—':c.point.pids }}</td>
    </tr><tr v-if="!filtered.length"><td colspan="6">{{ containers.length?'Нет совпадений':'Контейнеров пока нет' }}</td></tr></tbody>
   </table></div>
   <div class="monitor-table-footer"><span>{{ filtered.length }} контейнеров · RAM без неактивного файлового кеша · — нет метрики</span><div v-if="filtered.length>20"><button class="btn btn-sm btn-outline-secondary" :disabled="page<=1" aria-label="Предыдущая страница" @click="page--">‹</button><span>{{ page }} / {{ Math.ceil(filtered.length/20) }}</span><button class="btn btn-sm btn-outline-secondary" :disabled="page>=Math.ceil(filtered.length/20)" aria-label="Следующая страница" @click="page++">›</button></div></div>
   <section v-if="selected" class="monitor-detail" aria-label="Графики контейнера">
    <div class="monitor-section-heading"><h2>{{ selected.name }}</h2><span>{{ state(selected.state) }}</span><router-link :to="{name:'containers'}">Управление контейнерами <i class="fas fa-arrow-right" aria-hidden="true"/></router-link><button class="btn btn-sm btn-outline-secondary" aria-label="Закрыть графики контейнера" @click="select('')">Закрыть</button></div>
    <p class="monitor-note">{{ selected.image }}</p>
    <div v-if="detailLoading" class="monitor-loading" role="status">Загружаем графики контейнера…</div>
    <div v-else class="monitor-charts">
     <ResourceChart title="CPU контейнера" subtitle="100% — одно ядро" :points="data.containerHistory" :series="cpu" unit="percent" :ceiling="100" :start="start" :end="end"/>
     <ResourceChart title="RAM контейнера" :subtitle="'Лимит '+format(selected.point.memoryLimit)" :points="data.containerHistory" :series="memory" :ceiling="selected.point.memoryLimit||0" :start="start" :end="end"/>
     <ResourceChart title="Сеть контейнера" subtitle="Приём и передача" :points="data.containerHistory" :series="network" unit="rate" :start="start" :end="end"/>
     <ResourceChart title="Диск контейнера" subtitle="Блочный ввод-вывод" :points="data.containerHistory" :series="io" unit="rate" :start="start" :end="end"/>
    </div>
   </section>
   <p class="monitor-note">История за последний час хранится в памяти и начинается заново при перезапуске шлюза. Сеть хоста: без loopback, veth и Docker-мостов. Для контейнеров с host-сетью отдельные сетевые счётчики могут быть недоступны.</p>
  </template>
 </section>
</template>
<script>
import ResourceChart from '../components/ResourceChart.vue';
import {formatMetric} from '../monitor.mjs';
export default {
 name:'Monitoring',components:{ResourceChart},
 data:()=>({data:{containers:[],hostHistory:[],containerHistory:[]},minutes:15,selectedId:'',query:'',sort:'cpu',page:1,paused:false,loading:false,error:'',now:Date.now(),timer:null,controller:null,sequence:0,destroyed:false,
  cpu:[{key:'cpu',label:'CPU'}],memory:[{key:'memoryUsed',label:'Занято'}],network:[{key:'rx',label:'Приём'},{key:'tx',label:'Передача'}],disk:[{key:'diskUsed',label:'Занято'}],io:[{key:'read',label:'Чтение'},{key:'write',label:'Запись'}]}),
 computed:{
  host(){return this.data.host||{}},containers(){return this.data.containers||[]},running(){return this.containers.filter(c=>c.state==='running').length},
  stale(){return !this.data.sampledAt||this.now-this.data.sampledAt>15000||!!this.error},
  end(){return this.data.sampledAt||this.now},start(){return this.end-this.minutes*60000},
  selected(){return this.containers.find(c=>c.id===this.selectedId)},
  detailLoading(){return this.data.selectedId!==this.selectedId},
  filtered(){const q=this.query.trim().toLowerCase();return this.containers.filter(c=>(c.name+' '+c.image).toLowerCase().includes(q)).sort((a,b)=>this.sort==='name'?a.name.localeCompare(b.name):(b.point[this.sort]??-1)-(a.point[this.sort]??-1)||a.name.localeCompare(b.name))},
  pageItems(){return this.filtered.slice((this.page-1)*20,this.page*20)},
  uptime(){const s=this.host.uptimeSeconds;if(s==null)return '—';return s>=86400?Math.floor(s/86400)+' д '+Math.floor(s%86400/3600)+' ч':Math.floor(s/3600)+' ч '+Math.floor(s%3600/60)+' мин'},
 },
 watch:{minutes(){this.load()},query(){this.page=1},sort(){this.page=1},'containers.length'(){this.page=Math.min(this.page,Math.max(1,Math.ceil(this.filtered.length/20)))}},
 mounted(){this.load();this.timer=setInterval(()=>{this.now=Date.now();if(!this.paused&&!document.hidden&&!this.loading)this.load()},5000);document.addEventListener('visibilitychange',this.visibility)},
 beforeDestroy(){this.destroyed=true;clearInterval(this.timer);this.controller?.abort();document.removeEventListener('visibilitychange',this.visibility)},
 methods:{
  format:formatMetric,number(v){return v==null?'—':v.toFixed(2)},time(v){return new Date(v).toLocaleTimeString('ru-RU')},
  state(v){return ({running:'Работает',paused:'Приостановлен',exited:'Остановлен',created:'Создан',restarting:'Перезапускается',dead:'Ошибка'})[v]||v},
  select(id){this.selectedId=id;this.load()},
  togglePause(){this.paused=!this.paused;if(this.paused)this.controller?.abort();else this.load()},
  visibility(){if(document.hidden)this.controller?.abort();else if(!this.paused)this.load()},
  async load(){
   if(this.destroyed)return;this.controller?.abort();const seq=++this.sequence;this.controller=new AbortController();this.loading=true;
   try{const {data}=await this.$http.get('/v2/monitor',{params:{minutes:this.minutes,container:this.selectedId},signal:this.controller.signal});if(seq!==this.sequence||this.destroyed)return;this.data=data;this.error='';this.now=Date.now();}
   catch(e){if(seq===this.sequence&&e.code!=='ERR_CANCELED'&&!this.destroyed)this.error=e.response?.data?.error||'Не удалось обновить мониторинг. Последние данные сохранены.'}
   finally{if(seq===this.sequence)this.loading=false}
  },
 },
};
</script>
<style scoped>
.monitor-page{max-width:1680px;margin:0 auto}.monitor-heading{display:flex;align-items:center;gap:12px;flex-wrap:wrap;margin-bottom:14px}.monitor-heading h1{font-size:20px;margin:0;font-weight:700}.monitor-status{font-size:12px;color:var(--gm-accent)}.monitor-controls{margin-left:auto;display:flex;gap:8px;align-items:center}.monitor-controls label{margin:0;color:var(--gm-muted);font-size:12px}.monitor-controls select{width:110px}.monitor-section-heading{display:flex;align-items:center;gap:12px;margin:14px 0 10px;flex-wrap:wrap}.monitor-section-heading h2{font-size:15px;font-weight:650;margin:0;overflow-wrap:anywhere}.monitor-section-heading>span{color:var(--gm-muted);font-size:12px}.monitor-kpis{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:10px;margin-bottom:10px}.monitor-kpis>div{background:var(--gm-panel);border:1px solid var(--gm-border);border-radius:var(--gm-radius);padding:10px 12px;display:flex;flex-direction:column;gap:3px}.monitor-kpis span,.monitor-kpis small{color:var(--gm-muted);font-size:12px}.monitor-kpis strong{font-size:23px;line-height:1.2;font-variant-numeric:tabular-nums;font-weight:650}.monitor-charts{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:10px}.monitor-container-heading input{max-width:220px;margin-left:auto}.monitor-container-heading select{width:130px}.monitor-table-wrap{overflow:auto}.monitor-table{margin:0;font-size:12px;white-space:nowrap}.monitor-table th{color:var(--gm-muted);font-weight:600}.monitor-table td,.monitor-table th{padding:8px 10px;vertical-align:middle}.monitor-table tr.is-selected{background:var(--gm-accent-soft)}.monitor-table td:first-child{min-width:180px;max-width:360px}.monitor-container-select{display:block;background:none;border:0;padding:0;max-width:100%;white-space:normal;overflow-wrap:anywhere;text-align:left;color:var(--gm-accent);font-weight:600;cursor:pointer;min-height:24px}.monitor-container-state{display:block;font-size:11px;color:var(--gm-muted)}.monitor-table-footer{display:flex;align-items:center;justify-content:space-between;gap:12px;margin-top:6px;color:var(--gm-muted);font-size:12px}.monitor-table-footer>div{display:flex;align-items:center;gap:8px}.monitor-detail{border-top:1px solid var(--gm-border);margin-top:16px}.monitor-detail .monitor-section-heading a{margin-left:auto;font-size:12px}.monitor-note{color:var(--gm-muted);font-size:12px;margin:12px 0;overflow-wrap:anywhere}.monitor-loading{padding:20px;color:var(--gm-muted)}
@media(min-width:1400px){.monitor-charts{grid-template-columns:repeat(4,minmax(0,1fr))}}
@media(max-width:700px){.monitor-heading{gap:8px}.monitor-controls{margin-left:0;flex-wrap:wrap;width:100%}.monitor-kpis{grid-template-columns:repeat(2,minmax(0,1fr))}.monitor-charts{grid-template-columns:1fr}.monitor-container-heading input{margin-left:0;max-width:none;flex:1;min-width:120px}.monitor-table-footer{align-items:flex-start;flex-direction:column}.monitor-detail .monitor-section-heading a{margin-left:0}.monitor-status{font-size:11px}}
</style>
