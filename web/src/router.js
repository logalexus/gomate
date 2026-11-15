import Vue from 'vue';
import Router from 'vue-router';
import ContentStream from './views/ContentStream';
import Sidebar from './components/Sidebar';
import Containers from './views/Containers';
import Chat from './views/Chat';
import Anomalies from './views/Anomalies';

Vue.use(Router);

export default new Router({
 scrollBehavior() {return {x:0,y:0};},
	routes: [
 {path:"/monitoring",name:"monitoring",components:{content:()=>import('./views/Monitoring.vue')}},
 {path:"/containers",name:"containers",components:{content:Containers}},
 {path:"/chat",name:"chat",components:{content:Chat}},
 {path:"/anomalies",name:"anomalies",components:{content:Anomalies}},
		{
			path: '/:servicePort?/:streamId?',
			name: 'stream',
			props: true,
			components: {
				sidebar: Sidebar,
				content: ContentStream,
			},
		},
	],
	linkActiveClass: 'active',
});
