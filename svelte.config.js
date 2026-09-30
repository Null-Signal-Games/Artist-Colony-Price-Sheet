import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
  preprocess: vitePreprocess(),
  kit: {
    adapter: adapter({ fallback: '404.html', strict: false }),
    paths: {
      base: '/Artist-Colony-Price-Sheet'
    },
    prerender: {
      entries: ['*', '/staff/inventory']
    }
  }
};

export default config;
