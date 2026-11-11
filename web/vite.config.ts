import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { devMockPlugin } from './devMockPlugin';
import { execSync } from 'child_process';

let commitHash = 'dev';
try {
  commitHash = execSync('git rev-parse --short HEAD', { encoding: 'utf-8' }).trim();
} catch {}

const buildTime = new Date().toISOString();

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react(), devMockPlugin()],
  define: {
    __APP_VERSION__: JSON.stringify('1.0.0'),
    __COMMIT_HASH__: JSON.stringify(commitHash || 'dev'),
    __BUILD_TIME__: JSON.stringify(buildTime),
  },
  server: {
    port: 3000,
    host: '0.0.0.0',
    proxy: process.env.VITE_PROXY_BACKEND === 'true' ? {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      }
    } : undefined,
  }
});
