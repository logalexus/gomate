import {defineConfig} from 'vite';
import vue from '@vitejs/plugin-vue2';
import {fileURLToPath} from 'node:url';

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {'@': fileURLToPath(new URL('./src', import.meta.url))},
    extensions: ['.mjs', '.js', '.ts', '.jsx', '.tsx', '.json', '.vue'],
  },
  server: {proxy: {'/api': {target: 'http://127.0.0.1:65000', ws: true}}},
});
