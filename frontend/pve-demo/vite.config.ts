import { defineConfig } from 'vite';

const backend = 'http://127.0.0.1:18082';
export default defineConfig({
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
});
