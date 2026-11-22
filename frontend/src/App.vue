<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'

type Engine = { name: string; ready: boolean; error?: string }
type Task = {
  id: string; engine: string; source: string; name: string; directory: string
  status: string; total_bytes: number; completed_bytes: number; speed_bytes: number; error?: string
}
const base = '/app/fnsdownload'
const engines = ref<Engine[]>([])
const tasks = ref<Task[]>([])
const loading = ref(false)
const submitting = ref(false)
const error = ref('')
const url = ref('')
const engineName = ref('http')
const directory = ref('')
const filter = ref('all')
let timer: number | undefined

const filteredTasks = computed(() => filter.value === 'all' ? tasks.value : tasks.value.filter(t => t.status === filter.value))
const activeCount = computed(() => tasks.value.filter(t => t.status === 'running').length)
const totalSpeed = computed(() => tasks.value.reduce((n, t) => n + (t.speed_bytes || 0), 0))
const totalBytes = computed(() => tasks.value.reduce((n, t) => n + (t.total_bytes || 0), 0))
const doneBytes = computed(() => tasks.value.reduce((n, t) => n + (t.completed_bytes || 0), 0))

function formatBytes(value = 0) {
  if (!value) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let n = value, i = 0
  while (n >= 1024 && i < units.length - 1) { n /= 1024; i++ }
  return `${n.toFixed(i ? 1 : 0)} ${units[i]}`
}
function percent(task: Task) {
  return task.total_bytes > 0 ? Math.min(100, Math.round(task.completed_bytes / task.total_bytes * 100)) : 0
}
function statusLabel(status: string) {
  return ({ queued: '等待中', running: '下载中', paused: '已暂停', completed: '已完成', failed: '失败', removed: '已移除' } as Record<string,string>)[status] || status
}
async function api(path: string, init?: RequestInit) {
  const response = await fetch(base + path, { ...init, headers: { 'Content-Type': 'application/json', ...(init?.headers || {}) } })
  if (!response.ok) {
    let message = `请求失败（${response.status}）`
    try { message = (await response.json()).error || message } catch {}
    throw new Error(message)
  }
  if (response.status === 204) return null
  return response.json()
}
async function refresh() {
  loading.value = true
  try {
    const [engineData, taskData] = await Promise.all([api('/api/engines'), api('/api/tasks')])
    engines.value = engineData
    tasks.value = taskData
    if (!engines.value.some(e => e.name === engineName.value && e.ready)) {
      const available = engines.value.find(e => e.ready)
      if (available) engineName.value = available.name
    }
    error.value = ''
  } catch (e) { error.value = e instanceof Error ? e.message : '无法连接下载服务' }
  finally { loading.value = false }
}
async function addTask() {
  if (!url.value.trim()) return
  submitting.value = true
  try {
    await api('/api/tasks', { method: 'POST', body: JSON.stringify({ engine: engineName.value, url: url.value.trim(), directory: directory.value.trim() }) })
    url.value = ''
    await refresh()
  } catch (e) { error.value = e instanceof Error ? e.message : '创建任务失败' }
  finally { submitting.value = false }
}
async function action(task: Task, actionName: string) {
  try { await api(`/api/tasks/${encodeURIComponent(task.id)}/${actionName}`, { method: 'POST' }); await refresh() }
  catch (e) { error.value = e instanceof Error ? e.message : '操作失败' }
}
onMounted(() => { refresh(); timer = window.setInterval(refresh, 2500) })
onUnmounted(() => { if (timer) window.clearInterval(timer) })
</script>

<template>
  <main class="shell">
    <header class="topbar">
      <div class="brand"><div class="brand-icon">M</div><div><strong>MoxYi Download</strong><span>飞牛 NAS 下载中心</span></div></div>
      <div class="top-actions"><span class="connection"><i></i> 本地服务已连接</span><button class="icon-button" @click="refresh" :disabled="loading" title="刷新">↻</button></div>
    </header>

    <section class="welcome">
      <div><div class="eyebrow">DOWNLOAD CENTER</div><h1>下载管理</h1><p>统一管理多种下载引擎与下载任务</p></div>
      <div class="engine-pills"><span v-for="item in engines" :key="item.name" class="engine-pill" :class="{ready:item.ready}"><i></i>{{ item.name }} <small>{{ item.ready ? '可用' : '未连接' }}</small></span></div>
    </section>

    <section class="stats">
      <article><span>正在下载</span><strong>{{ activeCount }}</strong><small>个活动任务</small></article>
      <article><span>当前下载速度</span><strong>{{ formatBytes(totalSpeed) }}<small>/s</small></strong><small>所有引擎合计</small></article>
      <article><span>任务总量</span><strong>{{ tasks.length }}</strong><small>累计任务记录</small></article>
      <article><span>已完成数据</span><strong>{{ formatBytes(doneBytes) }}</strong><small>当前列表已下载</small></article>
    </section>

    <section class="panel create-panel">
      <div class="panel-heading"><div><h2>新建下载</h2><p>添加 HTTP、HTTPS 下载链接，选择可用引擎</p></div><span class="plus">＋</span></div>
      <form class="create-form" @submit.prevent="addTask">
        <label class="url-field"><span>下载链接</span><input v-model="url" type="url" placeholder="粘贴 HTTP / HTTPS 下载地址" required /></label>
        <label><span>下载引擎</span><select v-model="engineName"><option v-for="item in engines.filter(e => e.ready)" :key="item.name" :value="item.name">{{ item.name }}</option></select></label>
        <label><span>保存子目录</span><input v-model="directory" placeholder="默认下载目录" /></label>
        <button class="primary-button" type="submit" :disabled="submitting || !url.trim() || !engines.some(e => e.name === engineName && e.ready)">{{ submitting ? '正在添加…' : '开始下载' }}</button>
      </form>
      <p v-if="!engines.some(e => e.ready)" class="hint">当前没有可用下载引擎。请检查引擎配置。</p>
    </section>

    <section class="panel tasks-panel">
      <div class="panel-heading task-heading"><div><h2>下载任务</h2><p>实时更新任务状态与传输进度</p></div><span class="task-count">{{ filteredTasks.length }} 项任务</span></div>
      <nav class="filters">
        <button v-for="item in [{id:'all',label:'全部'},{id:'running',label:'下载中'},{id:'paused',label:'已暂停'},{id:'completed',label:'已完成'},{id:'failed',label:'失败'}]" :key="item.id" :class="{selected:filter===item.id}" @click="filter=item.id">{{ item.label }}</button>
      </nav>
      <div v-if="error" class="error-box">{{ error }}</div>
      <div v-if="filteredTasks.length" class="task-list">
        <article v-for="task in filteredTasks" :key="task.id" class="task-row">
          <div class="file-icon">↓</div>
          <div class="task-main">
            <div class="task-title"><strong>{{ task.name || task.source }}</strong><span class="status" :class="task.status">{{ statusLabel(task.status) }}</span></div>
            <div class="task-meta"><span>{{ task.engine }}</span><span>{{ formatBytes(task.completed_bytes) }} / {{ task.total_bytes ? formatBytes(task.total_bytes) : '大小未知' }}</span><span v-if="task.status==='running'">{{ formatBytes(task.speed_bytes) }}/s</span></div>
            <div class="progress"><i :style="{width: percent(task)+'%'}"></i></div>
            <small v-if="task.error" class="task-error">{{ task.error }}</small>
          </div>
          <div class="task-actions">
            <button v-if="task.status==='running'" @click="action(task,'pause')" title="暂停">Ⅱ</button>
            <button v-else-if="task.status==='paused' || task.status==='queued'" @click="action(task,'resume')" title="继续">▶</button>
            <button @click="action(task,'delete')" title="移除任务">×</button>
          </div>
        </article>
      </div>
      <div v-else class="empty-state"><div>⇩</div><strong>还没有下载任务</strong><span>在上方粘贴下载链接，开始你的第一个任务</span></div>
    </section>
    <footer> MoxYi Download <span>·</span> fnOS 原生应用 <span>·</span> 多引擎下载管理</footer>
  </main>
</template>
