/// <reference types="vitest/config" />
import { writeFileSync } from 'node:fs'
import { fileURLToPath, URL } from 'node:url'

import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig, type Plugin } from 'vite'

// 后端地址：`task dev` 会在 7860 端口启动后端；也可用环境变量 YANYI_API 指向其他实例。
const api = process.env.YANYI_API ?? 'http://127.0.0.1:7860'

// 构建会清空 dist/，这里补回 .gitkeep：保证未构建前端时 Go 的 //go:embed 仍能编译（见 web/embed.go）。
function keepGitkeep(): Plugin {
  return {
    name: 'yanyi-keep-gitkeep',
    apply: 'build',
    closeBundle() {
      writeFileSync(fileURLToPath(new URL('./dist/.gitkeep', import.meta.url)), '')
    },
  }
}

export default defineConfig({
  plugins: [react(), tailwindcss(), keepGitkeep()],
  resolve: {
    alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
  },
  server: {
    port: 5173,
    // changeOrigin 保持 false：后端的 Host / Origin 校验要求二者一致（均为 localhost:5173）。
    proxy: { '/api': { target: api, changeOrigin: false } },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    // 发布包也带 sourcemap：玩家反馈前端报错时可以直接定位到源码（体积代价可接受）。
    sourcemap: true,
  },
  test: {
    environment: 'jsdom',
    include: ['src/**/*.test.{ts,tsx}'],
  },
})
