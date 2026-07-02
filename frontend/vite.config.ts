import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { VitePWA } from 'vite-plugin-pwa'
export default defineConfig({plugins:[react(),tailwindcss(),VitePWA({registerType:'autoUpdate',manifest:{name:'订阅管理',short_name:'订阅管理',display:'standalone',start_url:'/',theme_color:'#0f766e',background_color:'#f8fafc',icons:[{src:'/icons/time-beacon.svg',sizes:'512x512',type:'image/svg+xml',purpose:'any maskable'}]},workbox:{navigateFallback:'/',skipWaiting:true,clientsClaim:true}})],server:{proxy:{'/api':'http://127.0.0.1:8080'}}})
