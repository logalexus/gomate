import Vue from 'vue';
import BootstrapVue from 'bootstrap-vue';
import {createHTTPClient} from './http.mjs';
import InfiniteLoading from 'vue-infinite-loading';
import App from './App.vue';
import router from './router';

import store from './store';
import 'bootstrap/dist/css/bootstrap.css';
import 'bootstrap-vue/dist/bootstrap-vue.css';
import '@fortawesome/fontawesome-free/css/all.css';
import './styles/workspace.css';
document.body.setAttribute('data-theme',store.state.theme||'dark');

Vue.config.productionTip = false;
Vue.use(BootstrapVue);
Vue.use(InfiniteLoading, {
	slots: {
		noResults: 'No results...',
		noMore: 'No more data',
		error: 'An error has occurred!',
		errorBtnText: 'Retry',
	},
	props: {
		spinner: 'waveDots',
	},
});

const axiosInstance = createHTTPClient();
// noinspection JSUnusedGlobalSymbols
Vue.prototype.$http = axiosInstance;

// noinspection JSUnusedGlobalSymbols
new Vue({
	router,
	store,
	render: h => h(App),
}).$mount('#app');
