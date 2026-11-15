<template>
 <button type="button" class="btn btn-outline-secondary gm-theme-button" aria-label="Toggle theme" :aria-pressed="String(theme==='dark')" :title="theme==='dark'?'Switch to light theme':'Switch to dark theme'" @click="toggle"><i :class="theme==='dark'?'far fa-sun':'far fa-moon'" aria-hidden="true"/></button>
</template>
<script>
export default {
 computed:{theme(){return this.$store.state.theme||'dark'}},
 watch:{theme:{immediate:true,handler(value){document.body.setAttribute('data-theme',value)}}},
 methods:{toggle(){this.$store.commit('setTheme',this.theme==='dark'?'light':'dark')}},
};
</script>
