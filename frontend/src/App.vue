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
const connected = ref(false)
const search = ref('')
const url = ref('')
const engineName = ref('http')
const directory = ref('')
const filter = ref('all')
let timer: number | undefined

const navigation = [
  { id: 'all', label: '全部任务', path: 'M4 4h6v6H4z M14 4h6v6h-6z M4 14h6v6H4z M14 14h6v6h-6z' },
  { id: 'running', label: '下载中', path: 'M12 3v12 M7 10l5 5 5-5 M4 16v4h16v-4' },
  { id: 'queued', label: '等待中', path: 'M12 8v5l3 2 M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0' },
  { id: 'paused', label: '已暂停', path: 'M8 5v14 M16 5v14' },
  { id: 'completed', label: '已完成', path: 'M8 12l3 3 5-6 M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0' },
  { id: 'failed', label: '下载失败', path: 'M12 8v5 M12 16v.01 M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0' },
]
const currentSection = computed(() => navigation.find(item => item.id === filter.value)?.label || '全部任务')
const filteredTasks = computed(() => tasks.value.filter(t =>
  (filter.value === 'all' || t.status === filter.value) &&
  `${t.name} ${t.source}`.toLowerCase().includes(search.value.trim().toLowerCase())
))
const taskCount = (status: string) => status === 'all' ? tasks.value.length : tasks.value.filter(t => t.status === status).length
const activeCount = computed(() => tasks.value.filter(t => t.status === 'running').length)
const totalSpeed = computed(() => tasks.value.reduce((n, t) => n + (t.speed_bytes || 0), 0))
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
  if (loading.value) return
  loading.value = true
  try {
    const [engineData, taskData] = await Promise.all([api('/api/engines'), api('/api/tasks')])
    engines.value = engineData
    tasks.value = taskData
    connected.value = true
    if (!engines.value.some(e => e.name === engineName.value && e.ready)) {
      const available = engines.value.find(e => e.ready)
      if (available) engineName.value = available.name
    }
    error.value = ''
  } catch (e) { connected.value = false; error.value = e instanceof Error ? e.message : '无法连接下载服务' }
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
  <div class="app-layout">
    <aside class="sidebar" aria-label="侧边导航">
      <a class="brand" href="#" @click.prevent="filter='all'; search=''" aria-label="MoxYi Download 全部任务">
        <div class="brand-icon"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 4v11m-4-4 4 4 4-4M5 17v3h14v-3" /></svg></div>
        <div><strong>MoxYi<span>Download</span></strong><small>让下载井然有序</small></div>
      </a>
      <div class="nav-caption">下载管理</div>
      <nav class="side-nav" aria-label="下载任务分类">
        <button v-for="item in navigation" :key="item.id" :class="{selected:filter===item.id}" :aria-current="filter===item.id ? 'page' : undefined" @click="filter=item.id">
          <svg viewBox="0 0 24 24" aria-hidden="true"><path :d="item.path" /></svg>
          <span>{{ item.label }}</span><small :class="{nonzero:taskCount(item.id)>0}">{{ taskCount(item.id) }}</small>
        </button>
      </nav>
      <div class="sidebar-bottom">
        <div class="nav-caption">下载引擎 <span>{{ engines.filter(e => e.ready).length }} 可用</span></div>
        <div v-for="item in engines" :key="item.name" class="engine-item" :title="item.error || '引擎可用'">
          <span class="engine-symbol">{{ item.name.slice(0,1).toUpperCase() }}</span>
          <span class="engine-name">{{ item.name === 'http' ? 'HTTP / HTTPS' : item.name }}</span>
          <span class="engine-dot" :class="{ready:item.ready}" :aria-label="item.ready ? '可用' : '未连接'"></span>
        </div>
        <p v-if="!engines.length" class="engine-placeholder">{{ loading ? '正在检测引擎…' : '暂无引擎信息' }}</p>
        <div class="sidebar-note"><span class="nas-symbol">▤</span><div><strong>fnOS 原生应用</strong><small>文件保存在你的 NAS</small></div></div>
      </div>
    </aside>
  <main class="shell">
    <header class="topbar">
      <div class="breadcrumb">下载管理 <span>/</span><strong>{{ currentSection }}</strong></div>
      <div class="top-actions"><span class="connection" :class="{offline:!connected}"><i></i>{{ connected ? '服务已连接' : loading ? '正在连接' : '服务未连接' }}</span><button class="icon-button" @click="refresh" :disabled="loading" title="刷新任务" aria-label="刷新任务"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M20 7v5h-5M4 17v-5h5M6 7a7 7 0 0 1 12-1l2 3M4 15l2 3a7 7 0 0 0 12-1" /></svg></button></div>
    </header>

    <section class="welcome">
      <div><div class="eyebrow">YOUR DOWNLOADS, ORGANIZED</div><h1>{{ currentSection }}<span class="heading-count">{{ taskCount(filter) }}</span></h1><p>管理每一次传输，让文件各就其位。</p></div>
      <a class="new-task-link" href="#download-url" @click="filter='all'">＋ 新建下载</a>
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
        <label class="url-field"><span>下载链接</span><input id="download-url" v-model="url" type="url" placeholder="粘贴 HTTP / HTTPS 下载地址" required /></label>
        <label><span>下载引擎</span><select v-model="engineName"><option v-for="item in engines.filter(e => e.ready)" :key="item.name" :value="item.name">{{ item.name }}</option></select></label>
        <label><span>保存子目录</span><input v-model="directory" placeholder="默认下载目录" /></label>
        <button class="primary-button" type="submit" :disabled="submitting || !url.trim() || !engines.some(e => e.name === engineName && e.ready)">{{ submitting ? '正在添加…' : '开始下载' }}</button>
      </form>
      <p v-if="!engines.some(e => e.ready)" class="hint">当前没有可用下载引擎。请检查引擎配置。</p>
    </section>

    <section class="panel tasks-panel">
      <div class="panel-heading task-heading"><div><h2>任务列表 <span class="task-count">{{ filteredTasks.length }}</span></h2><p>每 2.5 秒自动更新</p></div><label class="search-field"><svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="10" cy="10" r="6"/><path d="m15 15 5 5"/></svg><input v-model="search" type="search" placeholder="搜索文件或链接" aria-label="搜索文件或链接" /></label></div>
      <div v-if="error" class="error-box" role="alert">{{ error }}</div>
      <div v-if="filteredTasks.length" class="list-columns" aria-hidden="true"><span>文件名称 / 下载进度</span><span>操作</span></div>
      <div v-if="filteredTasks.length" class="task-list">
        <article v-for="task in filteredTasks" :key="task.id" class="task-row">
          <div class="file-icon">↓</div>
          <div class="task-main">
            <div class="task-title"><strong>{{ task.name || task.source }}</strong><span class="status" :class="task.status">{{ statusLabel(task.status) }}</span></div>
            <div class="task-meta"><span>{{ task.engine }}</span><span>{{ formatBytes(task.completed_bytes) }} / {{ task.total_bytes ? formatBytes(task.total_bytes) : '大小未知' }}</span><span v-if="task.status==='running'">{{ formatBytes(task.speed_bytes) }}/s</span></div>
            <div class="progress-line"><div class="progress" :class="task.status" role="progressbar" :aria-label="task.name + '下载进度'" :aria-valuenow="task.total_bytes>0 ? percent(task) : undefined" aria-valuemin="0" aria-valuemax="100"><i :style="{width: percent(task)+'%'}"></i></div><span>{{ task.total_bytes>0 ? percent(task)+'%' : '大小未知' }}</span></div>
            <small v-if="task.error" class="task-error">{{ task.error }}</small>
          </div>
          <div class="task-actions">
            <button v-if="task.status==='running'" @click="action(task,'pause')" title="暂停" :aria-label="'暂停 '+task.name">Ⅱ</button>
            <button v-else-if="task.status==='paused' || task.status==='queued'" @click="action(task,'resume')" title="继续" :aria-label="'继续 '+task.name">▶</button>
            <button @click="action(task,'delete')" title="移除任务" :aria-label="'移除任务 '+task.name">×</button>
          </div>
        </article>
      </div>
      <div v-else class="empty-state" aria-live="polite"><div><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 3v12m-4-4 4 4 4-4M4 16v5h16v-5" /></svg></div><strong>{{ !connected ? (loading ? '正在加载任务' : '暂时无法获取任务') : search.trim() ? '没有找到匹配的任务' : filter==='all' ? '从一个链接开始' : '这里还没有'+currentSection+'的任务' }}</strong><span>{{ !connected ? '连接下载服务后，任务会显示在这里' : search.trim() ? '试试其他文件名，或清空搜索条件' : filter==='all' ? '粘贴下载链接，将文件保存到你的 NAS' : '切换到全部任务，查看其他下载记录' }}</span><button v-if="connected && (filter!=='all' || search.trim())" class="text-button" @click="filter='all';search=''">查看全部任务 →</button><a v-else-if="connected" class="text-button" href="#download-url">添加第一个下载 →</a></div>
    </section>
    <footer> MoxYi Download <span>·</span> fnOS 原生应用 <span>·</span> 多引擎下载管理</footer>
  </main>
  </div>
</template>
