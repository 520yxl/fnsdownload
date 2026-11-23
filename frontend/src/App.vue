<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'

type Engine = { name: string; ready: boolean; error?: string }
type Task = {
  id: string; engine: string; source: string; name: string; directory: string
  status: string; total_bytes: number; completed_bytes: number; speed_bytes: number; error?: string
}
const base = '/app/fnsdownload'
type ThemeMode = 'light' | 'dark' | 'auto'
const themeOptions: {id: ThemeMode; label: string; path: string}[] = [
  {id:'light',label:'浅色模式',path:'M12 3v2M12 19v2M3 12h2M19 12h2M5.6 5.6 7 7M17 17l1.4 1.4M5.6 18.4 7 17M17 7l1.4-1.4M16 12a4 4 0 1 1-8 0 4 4 0 0 1 8 0'},
  {id:'dark',label:'深色模式',path:'M20.5 13.5A8.5 8.5 0 0 1 10.5 3.5a8.5 8.5 0 1 0 10 10'},
  {id:'auto',label:'跟随系统',path:'M3 4h18v12H3zM8 21h8M12 16v5'},
]
function savedTheme(): ThemeMode {
  try { const value = localStorage.getItem('moxyi-theme'); if (value==='light'||value==='dark'||value==='auto') return value } catch {}
  return 'auto'
}
const themeMode = ref<ThemeMode>(savedTheme())
const systemTheme = window.matchMedia('(prefers-color-scheme: dark)')
const systemDark = ref(systemTheme.matches)
const themeMenu = ref<HTMLDetailsElement | null>(null)
const activeThemeOption = computed(() => themeOptions.find(option => option.id===themeMode.value)!)
watch([themeMode,systemDark], () => {
  const resolved = themeMode.value==='auto' ? (systemDark.value ? 'dark' : 'light') : themeMode.value
  document.documentElement.dataset.theme = resolved
  document.documentElement.style.colorScheme = resolved
  try { localStorage.setItem('moxyi-theme',themeMode.value) } catch {}
}, {immediate:true})
function selectTheme(mode: ThemeMode) {
  themeMode.value = mode
  if (themeMenu.value) themeMenu.value.open = false
}
function systemThemeChanged(event: MediaQueryListEvent) { systemDark.value = event.matches }
function dismissThemeMenu(event: PointerEvent) {
  if (themeMenu.value?.open && event.target instanceof Node && !themeMenu.value.contains(event.target)) themeMenu.value.open = false
}
function themeMenuKeydown(event: KeyboardEvent) {
  if (event.key==='Escape' && themeMenu.value?.open) { themeMenu.value.open=false; themeMenu.value.querySelector('summary')?.focus() }
}
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
const createDialog = ref<HTMLDialogElement | null>(null)
const detailsDialog = ref<HTMLDialogElement | null>(null)
const removeDialog = ref<HTMLDialogElement | null>(null)
const selectedTask = ref<Task | null>(null)
const removalTask = ref<Task | null>(null)
const createError = ref('')
const operationError = ref('')
const pendingTask = ref('')
const notification = ref('')
const detailTask = computed(() => tasks.value.find(t => t.id === selectedTask.value?.id) || selectedTask.value)
let notificationTimer: number | undefined
let timer: number | undefined

function notify(message: string) {
  notification.value = message
  window.clearTimeout(notificationTimer)
  notificationTimer = window.setTimeout(() => notification.value = '', 3500)
}
function openCreate() {
  createError.value = ''
  createDialog.value?.showModal()
}
function openDetails(task: Task) {
  selectedTask.value = task
  operationError.value = ''
  detailsDialog.value?.showModal()
}
function confirmRemove(task: Task) {
  removalTask.value = task
  operationError.value = ''
  removeDialog.value?.showModal()
}
function cancelWhileBusy(event: Event, busy: boolean) {
  if (busy) event.preventDefault()
}

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
  if (submitting.value || !url.value.trim()) return
  try {
    const parsed = new URL(url.value.trim())
    if (!['http:', 'https:'].includes(parsed.protocol)) throw new Error()
  } catch {
    createError.value = '请输入有效的 HTTP 或 HTTPS 下载链接。'
    return
  }
  submitting.value = true
  createError.value = ''
  try {
    await api('/api/tasks', { method: 'POST', body: JSON.stringify({ engine: engineName.value, url: url.value.trim(), directory: directory.value.trim() }) })
    url.value = ''
    filter.value = 'all'
    search.value = ''
    createDialog.value?.close()
    notify('下载任务已创建')
    await refresh()
  } catch (e) { createError.value = e instanceof Error ? e.message : '创建任务失败' }
  finally { submitting.value = false }
}
async function action(task: Task, actionName: string) {
  if (pendingTask.value) return
  pendingTask.value = task.id
  operationError.value = ''
  try {
    await api(`/api/tasks/${encodeURIComponent(task.id)}/${actionName}`, { method: 'POST' })
    if (actionName === 'delete') {
      removeDialog.value?.close()
      if (selectedTask.value?.id === task.id) detailsDialog.value?.close()
    }
    notify(actionName === 'delete' ? '任务已移除，下载文件已保留' : actionName === 'pause' ? '已发送暂停请求' : '已发送继续请求')
    await refresh()
  } catch (e) { operationError.value = e instanceof Error ? e.message : '操作失败' }
  finally { pendingTask.value = '' }
}
onMounted(() => { refresh(); timer = window.setInterval(refresh, 2500); systemTheme.addEventListener('change',systemThemeChanged); document.addEventListener('pointerdown',dismissThemeMenu); document.addEventListener('keydown',themeMenuKeydown) })
onUnmounted(() => { if (timer) window.clearInterval(timer); window.clearTimeout(notificationTimer); systemTheme.removeEventListener('change',systemThemeChanged); document.removeEventListener('pointerdown',dismissThemeMenu); document.removeEventListener('keydown',themeMenuKeydown) })
</script>

<template>
  <div class="app-layout">
    <aside class="sidebar" aria-label="侧边导航">
      <div class="sidebar-brand">
      <a class="brand" href="#" @click.prevent="filter='all'; search=''" aria-label="MoxYi Download 全部任务">
        <div class="brand-icon"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 4v11m-4-4 4 4 4-4M5 17v3h14v-3" /></svg></div>
        <div><strong>MoxYi<span>Download</span></strong><small>让下载井然有序</small></div>
      </a>
        <details ref="themeMenu" class="theme-menu"><summary :aria-label="'外观：'+activeThemeOption.label" title="切换外观"><svg viewBox="0 0 24 24" aria-hidden="true"><path :d="activeThemeOption.path" /></svg></summary><fieldset class="theme-options"><legend>外观模式</legend><label v-for="option in themeOptions" :key="option.id"><input type="radio" name="theme" :value="option.id" :checked="themeMode===option.id" @change="selectTheme(option.id)" /><svg viewBox="0 0 24 24" aria-hidden="true"><path :d="option.path" /></svg><span>{{ option.label }}</span><small v-if="themeMode===option.id">✓</small></label><p>自动模式随系统外观切换</p></fieldset></details>
      </div>
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
      <div class="top-actions"><span class="connection" :class="{offline:!connected}"><i></i>{{ connected ? '服务已连接' : loading ? '正在连接' : '服务未连接' }}</span>

        <button class="icon-button" @click="refresh" :disabled="loading" title="刷新任务" aria-label="刷新任务"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M20 7v5h-5M4 17v-5h5M6 7a7 7 0 0 1 12-1l2 3M4 15l2 3a7 7 0 0 0 12-1" /></svg></button></div>
    </header>

    <section class="welcome">
      <div><div class="eyebrow">YOUR DOWNLOADS, ORGANIZED</div><h1>{{ currentSection }}<span class="heading-count">{{ taskCount(filter) }}</span></h1><p>管理每一次传输，让文件各就其位。</p></div>
      <button class="new-task-link" @click="openCreate">＋ 新建下载</button>
    </section>

    <section class="stats">
      <article><span>正在下载</span><strong>{{ activeCount }}</strong><small>个活动任务</small></article>
      <article><span>当前下载速度</span><strong>{{ formatBytes(totalSpeed) }}<small>/s</small></strong><small>所有引擎合计</small></article>
      <article><span>任务总量</span><strong>{{ tasks.length }}</strong><small>累计任务记录</small></article>
      <article><span>已完成数据</span><strong>{{ formatBytes(doneBytes) }}</strong><small>当前列表已下载</small></article>
    </section>

    <section class="panel tasks-panel">
      <div class="panel-heading task-heading"><div><h2>任务列表 <span class="task-count">{{ filteredTasks.length }}</span></h2><p>每 2.5 秒自动更新</p></div><label class="search-field"><svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="10" cy="10" r="6"/><path d="m15 15 5 5"/></svg><input v-model="search" type="search" placeholder="搜索文件或链接" aria-label="搜索文件或链接" /></label></div>
      <div v-if="error || operationError" class="error-box" role="alert">{{ error || operationError }}</div>
      <div v-if="filteredTasks.length" class="list-columns" aria-hidden="true"><span>文件名称 / 下载进度</span><span>操作</span></div>
      <div v-if="filteredTasks.length" class="task-list">
        <article v-for="task in filteredTasks" :key="task.id" class="task-row">
          <div class="file-icon">↓</div>
          <div class="task-main">
            <div class="task-title"><button class="task-name" @click="openDetails(task)" :title="'查看任务详情：'+(task.name || task.source)">{{ task.name || task.source }}</button><span class="status" :class="task.status">{{ statusLabel(task.status) }}</span></div>
            <div class="task-meta"><span>{{ task.engine }}</span><span>{{ formatBytes(task.completed_bytes) }} / {{ task.total_bytes ? formatBytes(task.total_bytes) : '大小未知' }}</span><span v-if="task.status==='running'">{{ formatBytes(task.speed_bytes) }}/s</span></div>
            <div class="progress-line"><div class="progress" :class="task.status" role="progressbar" :aria-label="task.name + '下载进度'" :aria-valuenow="task.total_bytes>0 ? percent(task) : undefined" aria-valuemin="0" aria-valuemax="100"><i :style="{width: percent(task)+'%'}"></i></div><span>{{ task.total_bytes>0 ? percent(task)+'%' : '大小未知' }}</span></div>
            <small v-if="task.error" class="task-error">{{ task.error }}</small>
          </div>
          <div class="task-actions">
            <button v-if="task.status==='running'" @click="action(task,'pause')" :disabled="!!pendingTask" title="暂停" :aria-label="'暂停 '+task.name">Ⅱ</button>
            <button v-else-if="task.status==='paused' || task.status==='queued'" @click="action(task,'resume')" :disabled="!!pendingTask" title="继续" :aria-label="'继续 '+task.name">▶</button>
            <button @click="confirmRemove(task)" :disabled="!!pendingTask" title="移除任务" :aria-label="'移除任务 '+task.name">×</button>
          </div>
        </article>
      </div>
      <div v-else class="empty-state" aria-live="polite"><div><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 3v12m-4-4 4 4 4-4M4 16v5h16v-5" /></svg></div><strong>{{ !connected ? (loading ? '正在加载任务' : '暂时无法获取任务') : search.trim() ? '没有找到匹配的任务' : filter==='all' ? '从一个链接开始' : '这里还没有'+currentSection+'的任务' }}</strong><span>{{ !connected ? '连接下载服务后，任务会显示在这里' : search.trim() ? '试试其他文件名，或清空搜索条件' : filter==='all' ? '新建下载任务，将文件保存到你的 NAS' : '切换到全部任务，查看其他下载记录' }}</span><button v-if="connected && (filter!=='all' || search.trim())" class="text-button" @click="filter='all';search=''">查看全部任务 →</button><button v-else-if="connected" class="text-button" @click="openCreate">添加第一个下载 →</button></div>
    </section>
    <footer> MoxYi Download <span>·</span> fnOS 原生应用 <span>·</span> 多引擎下载管理</footer>
  </main>
  <dialog ref="createDialog" class="modal create-modal" aria-labelledby="create-title" @cancel="cancelWhileBusy($event, submitting)">
    <div class="modal-header"><div><span class="modal-eyebrow">NEW DOWNLOAD</span><h2 id="create-title">新建下载</h2><p>添加链接，其余交给下载器。</p></div><button class="close-button" @click="createDialog?.close()" :disabled="submitting" aria-label="关闭新建下载">×</button></div>
    <form @submit.prevent="addTask" :aria-busy="submitting">
      <div class="modal-body create-form">
        <label class="url-field"><span>下载链接</span><textarea id="download-url" v-model="url" placeholder="粘贴 HTTP / HTTPS 下载地址" required autofocus :disabled="submitting" rows="3"></textarea><small>目前支持单个 HTTP 或 HTTPS 链接。</small></label>
        <label><span>下载引擎</span><select v-model="engineName" :disabled="submitting"><option v-for="item in engines.filter(e => e.ready)" :key="item.name" :value="item.name">{{ item.name === 'http' ? 'HTTP / HTTPS' : item.name }}</option></select></label>
        <label><span>保存子目录 <small>可选</small></span><input v-model="directory" placeholder="例如：软件 / 资料" :disabled="submitting" /><small>留空则保存到默认下载目录。</small></label>
        <div v-if="createError" class="error-box form-message" role="alert">{{ createError }}</div>
        <p v-if="!engines.some(e => e.ready)" class="hint form-message">当前没有可用引擎，请稍后重试。</p>
      </div>
      <div class="modal-footer"><span>文件保存在你的 NAS</span><button class="secondary-button" type="button" @click="createDialog?.close()" :disabled="submitting">取消</button><button class="primary-button" type="submit" :disabled="submitting || !url.trim() || !engines.some(e => e.name === engineName && e.ready)">{{ submitting ? '正在创建…' : '开始下载' }}</button></div>
    </form>
  </dialog>
  <dialog ref="detailsDialog" class="modal detail-modal" aria-labelledby="details-title" @cancel="cancelWhileBusy($event, !!pendingTask)">
    <template v-if="detailTask">
      <div class="modal-header"><div><span class="modal-eyebrow">TASK DETAILS</span><h2 id="details-title">任务详情</h2></div><button class="close-button" @click="detailsDialog?.close()" :disabled="!!pendingTask" aria-label="关闭任务详情">×</button></div>
      <div class="modal-body"><div class="detail-file"><div class="file-icon">↓</div><div><h3>{{ detailTask.name }}</h3><span class="status" :class="detailTask.status">{{ statusLabel(detailTask.status) }}</span></div></div>
        <div class="detail-progress"><div class="progress" :class="detailTask.status"><i :style="{width:percent(detailTask)+'%'}"></i></div><span>{{ detailTask.total_bytes>0 ? percent(detailTask)+'%' : '大小未知' }}</span></div>
        <dl class="detail-grid"><div><dt>已下载 / 总大小</dt><dd>{{ formatBytes(detailTask.completed_bytes) }} / {{ detailTask.total_bytes>0 ? formatBytes(detailTask.total_bytes) : '大小未知' }}</dd></div><div><dt>下载速度</dt><dd>{{ formatBytes(detailTask.status==='running' ? detailTask.speed_bytes : 0) }}/s</dd></div><div><dt>下载引擎</dt><dd>{{ detailTask.engine }}</dd></div><div class="detail-wide"><dt>保存位置</dt><dd>{{ detailTask.directory || '默认下载目录' }}</dd></div><div class="detail-wide"><dt>源链接</dt><dd class="source-text">{{ detailTask.source }}</dd></div></dl>
        <div v-if="detailTask.error || operationError" class="error-box" role="alert">{{ operationError || detailTask.error }}</div>
      </div>
      <div class="modal-footer"><button class="danger-text" @click="confirmRemove(detailTask)" :disabled="!!pendingTask">移除任务</button><button class="secondary-button" @click="detailsDialog?.close()" :disabled="!!pendingTask">关闭</button><button v-if="detailTask.status==='running'" class="primary-button" @click="action(detailTask,'pause')" :disabled="!!pendingTask">{{ pendingTask ? '处理中…' : '暂停下载' }}</button><button v-else-if="detailTask.status==='paused' || detailTask.status==='queued'" class="primary-button" @click="action(detailTask,'resume')" :disabled="!!pendingTask">{{ pendingTask ? '处理中…' : '继续下载' }}</button></div>
    </template>
  </dialog>
  <dialog ref="removeDialog" class="modal remove-modal" aria-labelledby="remove-title" aria-describedby="remove-description" @cancel="cancelWhileBusy($event, !!pendingTask)">
    <div class="modal-header"><div><span class="modal-eyebrow">REMOVE TASK</span><h2 id="remove-title">移除这个下载任务？</h2></div><button class="close-button" @click="removeDialog?.close()" :disabled="!!pendingTask" aria-label="关闭移除确认">×</button></div>
    <div class="modal-body"><p class="remove-name">{{ removalTask?.name }}</p><p id="remove-description">仅移除任务记录，已下载的文件会保留。正在下载的任务会停止。</p><div v-if="operationError" class="error-box" role="alert">{{ operationError }}</div></div>
    <div class="modal-footer"><button class="secondary-button" @click="removeDialog?.close()" :disabled="!!pendingTask" autofocus>保留任务</button><button class="danger-button" @click="removalTask && action(removalTask,'delete')" :disabled="!!pendingTask">{{ pendingTask ? '正在移除…' : '确认移除' }}</button></div>
  </dialog>
  <div v-if="notification" class="toast" role="status"><span>✓</span>{{ notification }}</div>
  </div>
</template>
