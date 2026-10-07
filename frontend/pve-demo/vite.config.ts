import { defineConfig, loadEnv } from 'vite';

export default defineConfig(({ mode }) => {
  const configured = loadEnv(mode, process.cwd(), 'PVE_BACKEND_URL').PVE_BACKEND_URL || 'http://127.0.0.1:18082';
  const address = new URL(configured);
  if (!['http:', 'https:'].includes(address.protocol) || !['127.0.0.1', 'localhost', '[::1]'].includes(address.hostname) || address.username || address.password || address.pathname !== '/' || address.search || address.hash) throw new Error('PVE_BACKEND_URL必须是本机HTTP服务地址');
  const backend = address.origin;
  return {
  server: {
    host: '127.0.0.1', port: 5174, strictPort: true,
    proxy: {
      '/api': { target: backend, changeOrigin: true },
      '/ws': {
        target: backend, ws: true, changeOrigin: true,
        configure(proxy) {
          proxy.on('proxyReqWs', (proxyRequest, request, socket) => {
            if (request.headers.origin !== 'http://127.0.0.1:5174') { socket.destroy(); return; }
            proxyRequest.setHeader('Origin', backend);
          });
        },
      },
    },
  },
  };
});
