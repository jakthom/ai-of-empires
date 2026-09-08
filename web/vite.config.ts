import { defineConfig } from 'vite';
import { resolve } from 'node:path';

export default defineConfig({
  server: { proxy: { '/api': 'http://127.0.0.1:9090' } },
  build: {
    target: 'es2022',
    chunkSizeWarningLimit: 700,
    rollupOptions: {
      input: {
        app: resolve(import.meta.dirname, 'index.html'),
        apiDocs: resolve(import.meta.dirname, 'docs.html'),
      },
    },
  },
});
