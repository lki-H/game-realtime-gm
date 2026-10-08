import { defineConfig, loadEnv } from 'vite';

export default defineConfig(({ mode }) => {
  const configured = loadEnv(mode, '.', 'PVE_BACKEND_URL').PVE_BACKEND_URL || 'http://127.0.0.1:8080';
  const address = new URL(configured);
  if (!['http:', 'https:'].includes(address.protocol) || !['127.0.0.1', 'localhost', '[::1]'].includes(address.hostname) || address.username || address.password || address.pathname !== '/' || address.search || address.hash) throw new Error('PVE_BACKEND_URL必须是本机HTTP服务地址');
  return { server: { host: '127.0.0.1', port: 5173, strictPort: true, proxy: { '/api': { target: address.origin, changeOrigin: true } } } };
});
