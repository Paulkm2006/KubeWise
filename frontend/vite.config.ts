import { defineConfig, loadEnv } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '');
  const apiTarget = env.VITE_PROXY_TARGET || 'http://localhost:3000';
  const apiUser = env.VITE_API_USER || '';
  const apiPass = env.VITE_API_PASS || '';

  return {
    plugins: [react()],
    server: {
      proxy: {
        '/api': {
          target: apiTarget,
          changeOrigin: true,
          ...(apiUser && apiPass ? { auth: `${apiUser}:${apiPass}` } : {}),
        },
      },
    },
  };
});
