import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
    plugins: [react()],
    server: {
        port: 5174,
        host: true,
        proxy: {
            '/api': process.env.API_URL || 'http://localhost:4000'
        }
    }
});
