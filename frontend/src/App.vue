<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { TrimApp, type AppAuthResult } from '@trimjs/web-app'

type Engine = { name: string; ready: boolean; error?: string }
type Task = {
  auto_engine?: boolean; headers_required?: boolean; tried_engines?: string[]; engine_note?: string; elapsed_ms?: number; id: string; engine: string; source: string; name: string; directory: string
  status: string; total_bytes: number; completed_bytes: number; speed_bytes: number; error?: string; retry_count?: number; retry_at?: string; retryable?: boolean
}
function formatDuration(seconds: number) {
  const total = Math.max(0, Math.floor(seconds))
  const hours = Math.floor(total / 3600)
  const minutes = Math.floor(total % 3600 / 60)
  const rest = total % 60
  return hours ? `${hours} 小时 ${minutes} 分` : minutes ? `${minutes} 分 ${rest} 秒` : `${rest} 秒`
}
function remainingTime(task: Task) {
  if (task.total_bytes <= 0 || task.speed_bytes <= 0 || task.completed_bytes >= task.total_bytes) return '计算中'
  return formatDuration(Math.ceil((task.total_bytes - task.completed_bytes) / task.speed_bytes))
}
const base = '/app/fnsdownload'
type ThemeMode = 'light' | 'dark' | 'auto'
const themeOptions: {id: ThemeMode; label: string; path: string}[] = [
  {id:'light',label:'浅色模式',path:'M12 3v2M12 19v2M3 12h2M19 12h2M5.6 5.6 7 7M17 17l1.4 1.4M5.6 18.4 7 17M17 7l1.4-1.4M16 12a4 4 0 1 1-8 0 4 4 0 0 1 8 0'},
  {id:'dark',label:'深色模式',path:'M20.5 13.5A8.5 8.5 0 0 1 10.5 3.5a8.5 8.5 0 1 0 10 10'},
  {id:'auto',label:'跟随系统',path:'M3 4h18v12H3zM8 21h8M12 16v5'},
]
const themeMode = ref<ThemeMode>('auto')
const appSettingsLoaded = ref(false)
const savingTheme = ref(false)
const systemTheme = window.matchMedia('(prefers-color-scheme: dark)')
const systemDark = ref(systemTheme.matches)
const activeThemeOption = computed(() => themeOptions.find(option => option.id===themeMode.value)!)
const nextThemeOption = computed(() => themeOptions[(themeOptions.findIndex(option => option.id===themeMode.value)+1)%themeOptions.length])
watch([themeMode,systemDark], () => {
  const resolved = themeMode.value==='auto' ? (systemDark.value ? 'dark' : 'light') : themeMode.value
  document.documentElement.dataset.theme = resolved
  document.documentElement.style.colorScheme = resolved
}, {immediate:true})
async function cycleTheme() {
  if (!appSettingsLoaded.value || savingTheme.value) return
  savingTheme.value = true
  try {
    const result = await api('/api/settings', {method:'PATCH',body:JSON.stringify({theme:nextThemeOption.value.id})})
    themeMode.value = result.theme
  } catch (e) { notify(e instanceof Error ? e.message : '无法保存主题模式') }
  finally { savingTheme.value = false }
}
function systemThemeChanged(event: MediaQueryListEvent) { systemDark.value = event.matches }
const engines = ref<Engine[]>([])
const tasks = ref<Task[]>([])
const loading = ref(false)
const submitting = ref(false)
const error = ref('')
const connected = ref(false)
const search = ref('')
const url = ref('')
const selectedEngine = ref('auto')
const torrentFile = ref<File | null>(null)
const torrentPath = ref('')
const pickingTorrent = ref(false)
const hasTorrent = computed(() => !!torrentFile.value || !!torrentPath.value)
let torrentAuthState = ''
const torrentName = computed(() => torrentFile.value?.name || torrentPath.value.split('/').pop() || '')
function clearTorrent() { torrentFile.value=null; torrentPath.value=''; if(torrentInput.value) torrentInput.value.value='' }
function localTorrentChanged(event: Event) { torrentPath.value=''; torrentFile.value=(event.target as HTMLInputElement).files?.[0] || null }
async function chooseNASTorrent() {
  if (pickingTorrent.value || submitting.value) return
  pickingTorrent.value=true; createError.value=''
  try {
    await fnos.ready()
    if (fnos.isStandaloneWeb) {
      if (['localhost','127.0.0.1'].includes(location.hostname)) throw new Error('请在飞牛桌面中选择 NAS 种子文件。')
      torrentAuthState=crypto.randomUUID()
      await fnos.openAppAuth('pickUserFile',{appName:'fnsdownload',directory:false,accept:['.torrent'],state:torrentAuthState,redirectUri:base+'/?directory-callback=1',sidebarGroup:['myFiles','otherShare','favorites']},{target:'_blank',features:'width=750,height=630'})
    } else {
      createDialog.value?.close()
      try {
        const result=await fnos.pickUserFile({directory:false,accept:['.torrent'],title:'选择 NAS 种子文件',okText:'使用此种子',sidebarGroup:['myFiles','otherShare','favorites']})
        if(result && result.code!==0) throw new Error(result.msg || '选择种子失败')
        if(result?.data?.length) {clearTorrent();torrentPath.value=result.data[0]}
      } finally {createDialog.value?.showModal()}
    }
  } catch(e) {createError.value=e instanceof Error?e.message:'无法打开飞牛文件选择器'}
  finally {pickingTorrent.value=false}
}
const torrentInput=ref<HTMLInputElement|null>(null)
const qbOptions=ref({download_kb:0,upload_kb:0,connections:200,dht:true,pex:true})
const qbSettingsLoaded=ref(false)
const qbSettingsError=ref('')
const engineChoices = [{name:'qbittorrent',label:'qBittorrent（BT / 磁力）'},{name:'xunlei',label:'迅雷'}, {name:'aria2',label:'aria2'}, {name:'http',label:'HTTP / HTTPS'}]
const canCreate=computed(()=>{
 const names=hasTorrent.value?['qbittorrent']:url.value.split(/\r?\n/).map(v=>v.trim()).filter(Boolean).map(v=>v.toLowerCase().startsWith('magnet:')?'qbittorrent':'http')
 return names.length>0&&names.every(kind=>{const choices=selectedEngine.value==='auto'?(kind==='qbittorrent'?['qbittorrent']:['xunlei','aria2','http']):[selectedEngine.value];return choices.some(name=>(kind==='qbittorrent')===(name==='qbittorrent')&&engines.value.some(e=>e.name===name&&e.ready))})
})
const supportsHeaders = computed(() => (selectedEngine.value === 'auto' || selectedEngine.value === 'xunlei') && engines.value.some(e => e.name === 'xunlei' && e.ready))
const directory = ref('')
type Preferences = { directory: string; interval: number; max_concurrent: number; auto_retry: number; disk_reserve_mb: number }
const preferences = ref<Preferences>({directory:'',interval:2500,max_concurrent:5,auto_retry:0,disk_reserve_mb:512})
const settingsDraft = ref<Preferences>({...preferences.value})
const settingsError = ref('')
const settingsPicking = ref(false)
type XunleiOptions = {concurrent:number; download_kb:number; upload_kb:number; upload:boolean; dynamic_acceleration:boolean}
const xunleiOptions = ref<XunleiOptions>({concurrent:10,download_kb:0,upload_kb:0,upload:false,dynamic_acceleration:true})
type Aria2Options = {split:number;connections:number;min_split_mb:number;download_kb:number;connect_timeout:number;timeout:number;max_tries:number;retry_wait:number}
const aria2Options=ref<Aria2Options>({split:8,connections:8,min_split_mb:10,download_kb:0,connect_timeout:30,timeout:60,max_tries:3,retry_wait:5})
const aria2SettingsLoaded=ref(false)
const aria2SettingsError=ref('')
const sdkSettingsLoaded = ref(false)
const savingSettings = ref(false)
const recovering = ref(false)
const requestHeaders = ref('')
const createAdvanced = ref(false)
const removeFiles = ref(false)
const sdkDetails = ref<{traffic:{origin:number;p2p:number;p2s:number;dcdn:number};created:number;completed:number} | null>(null)
const detailsError = ref('')
const pickingDirectory = ref(false)
const directoryError = ref('')
const fnos = new TrimApp()
let directoryAuthState = ''
function acceptDirectoryResult(result: AppAuthResult) {
  if(result && torrentAuthState && result.state===torrentAuthState && result.appName==='fnsdownload' && result.method==='pickUserFile') {
    torrentAuthState=''
    if(result.status==='success' && result.path?.length) {clearTorrent();torrentPath.value=result.path[0]}
    else if(result.status==='error') createError.value='种子文件授权失败，请重试。'
    return
  }
  if (!result || !directoryAuthState || result.state !== directoryAuthState || result.appName !== 'fnsdownload' || result.method !== 'pickUserFile') return
  directoryAuthState = ''
  if (result.status === 'success' && result.path?.length) { if (settingsPicking.value) settingsDraft.value.directory = result.path[0]; else directory.value = result.path[0] }
  else if (result.status === 'error') directoryError.value = '目录授权失败，请重试。'
}
function directoryMessage(event: MessageEvent) {
  if (event.origin === location.origin && event.data?.type === 'fnsdownload:directory') acceptDirectoryResult(event.data.result)
}
async function chooseDirectory(forSettings = false) {
  if (pickingDirectory.value) return
  settingsPicking.value = forSettings
  pickingDirectory.value = true
  directoryError.value = ''
  try {
    await fnos.ready()
    if (fnos.isStandaloneWeb) {
      if (['localhost','127.0.0.1'].includes(location.hostname)) throw new Error('请在飞牛桌面中打开应用，使用 NAS 的目录选择器。')
      directoryAuthState = crypto.randomUUID()
      await fnos.openAppAuth('pickUserFile', {
        appName:'fnsdownload', directory:true, state:directoryAuthState,
        redirectUri:base+'/?directory-callback=1', sidebarGroup:['myFiles','otherShare','favorites'],
      }, {target:'_blank',features:'width=750,height=630'})
    } else {
      // The host picker must appear above the native create dialog.
      const pickerParent = forSettings ? null : createDialog.value
      pickerParent?.close()
      try {
        const result = await fnos.pickUserFile({directory:true,title:'选择下载保存目录',okText:'保存到此目录',creatable:true,sidebarGroup:['myFiles','otherShare','favorites']})
        if (result && result.code !== 0) throw new Error(result.msg || '选择目录失败')
        if (result?.data?.length) { if (forSettings) settingsDraft.value.directory = result.data[0]; else directory.value = result.data[0] }
      } finally { pickerParent?.showModal() }
    }
  } catch (e) { directoryError.value = e instanceof Error ? e.message : '无法打开飞牛目录选择器' }
  finally { pickingDirectory.value = false }
}
const openingDirectory = ref(false)
async function openTaskDirectory(task: Task) {
  if (openingDirectory.value) return
  openingDirectory.value = true
  operationError.value = ''
  detailsError.value = ''
  const parent = detailsDialog.value?.open ? detailsDialog.value : null
  try {
    if (!task.directory || !task.directory.startsWith('/') || task.directory.includes(String.fromCharCode(0))) throw new Error('该任务没有可用的保存目录。')
    await fnos.ready()
    if (fnos.isStandaloneWeb) throw new Error('请在飞牛桌面中打开应用，再打开保存目录。')
    parent?.close()
    await fnos.openFileManager(task.directory)
  } catch (e) {
    const message = e instanceof Error ? e.message : '无法打开保存目录，请检查飞牛文件管理权限。'
    if (parent) { detailsError.value = message; if (!parent.open) parent.showModal() }
    else operationError.value = message
  } finally { openingDirectory.value = false }
}
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
let refreshing = false
let disposed = false

function notify(message: string) {
  notification.value = message
  window.clearTimeout(notificationTimer)
  notificationTimer = window.setTimeout(() => notification.value = '', 3500)
}
function openCreate() {
  createAdvanced.value = false
  selectedEngine.value = 'auto'
  clearTorrent()
  directoryError.value = ''
  createError.value = ''
  requestHeaders.value = ''
  directory.value = preferences.value.directory
  createDialog.value?.showModal()
}
async function loadAppSettings() {
  const result = await api('/api/settings')
  preferences.value = {directory:result.directory,interval:result.interval,max_concurrent:result.max_concurrent,auto_retry:result.auto_retry || 0,disk_reserve_mb:result.disk_reserve_mb ?? 512}
  themeMode.value = result.theme
  appSettingsLoaded.value = true
  scheduleRefresh()
}
async function openSettings() {
  settingsDraft.value = {...preferences.value}
  settingsError.value = ''
  directoryError.value = ''
  sdkSettingsLoaded.value = false
  qbSettingsLoaded.value=false
  qbSettingsError.value=''
  aria2SettingsLoaded.value = false
  aria2SettingsError.value = ''
  filter.value = 'settings'
  try {
    await loadAppSettings()
    settingsDraft.value = {...preferences.value}
    const result = await api('/api/xunlei/settings')
    xunleiOptions.value = result.options
    sdkSettingsLoaded.value = true
  } catch (e) { settingsError.value = e instanceof Error ? e.message : '无法加载迅雷设置' }
  try {
    const result=await api('/api/aria2/settings')
    aria2Options.value=result.options
    aria2SettingsLoaded.value=true
  } catch(e){aria2SettingsError.value=e instanceof Error ? e.message : '无法加载 aria2 设置'}
  try {qbOptions.value=(await api('/api/qbittorrent/settings')).options;qbSettingsLoaded.value=true} catch(e){qbSettingsError.value=e instanceof Error?e.message:'无法加载 qBittorrent 设置'}
}
async function saveSettings() {
  if (savingSettings.value) return
  savingSettings.value = true
  settingsError.value = ''
  try {
    if (sdkSettingsLoaded.value) await api('/api/xunlei/settings', {method:'PUT',body:JSON.stringify(xunleiOptions.value)})
    if(qbSettingsLoaded.value) await api('/api/qbittorrent/settings',{method:'PUT',body:JSON.stringify(qbOptions.value)})
    if (aria2SettingsLoaded.value) await api('/api/aria2/settings',{method:'PUT',body:JSON.stringify(aria2Options.value)})
    const patch: {interval:number;max_concurrent:number;auto_retry:number;disk_reserve_mb:number;directory?:string} = {interval:settingsDraft.value.interval,max_concurrent:settingsDraft.value.max_concurrent,auto_retry:settingsDraft.value.auto_retry,disk_reserve_mb:settingsDraft.value.disk_reserve_mb}
    if (settingsDraft.value.directory !== preferences.value.directory) patch.directory = settingsDraft.value.directory
    const result = await api('/api/settings',{method:'PATCH',body:JSON.stringify(patch)})
    preferences.value = {directory:result.directory,interval:result.interval,max_concurrent:result.max_concurrent,auto_retry:result.auto_retry || 0,disk_reserve_mb:result.disk_reserve_mb ?? 512}
    themeMode.value = result.theme
    scheduleRefresh()
    notify('下载器设置已保存')
  } catch (e) { settingsError.value = e instanceof Error ? e.message : '无法保存设置' }
  finally { savingSettings.value = false }
}
async function recoverSDKTasks() {
  if (recovering.value) return
  recovering.value = true
  settingsError.value = ''
  try { const result = await api('/api/xunlei/recover',{method:'POST'}); notify(`已恢复 ${result.recovered} 个任务`); await refresh(true) }
  catch (e) { settingsError.value = e instanceof Error ? e.message : '恢复任务失败' }
  finally { recovering.value = false }
}
async function openDetails(task: Task) {
  selectedTask.value = task
  operationError.value = ''
  sdkDetails.value = null
  detailsError.value = ''
  detailsDialog.value?.showModal()
  if (task.engine === 'xunlei') {
    try { const result = await api(`/api/xunlei/tasks/${encodeURIComponent(task.id)}/details`); if (selectedTask.value?.id === task.id) sdkDetails.value = result }
    catch (e) { if (selectedTask.value?.id === task.id) detailsError.value = e instanceof Error ? e.message : '无法读取迅雷详情' }
  }
}
function confirmRemove(task: Task) {
  removeFiles.value = false
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
    let taskCreated = false
    let duplicate = false
    try { const result = await response.json(); message = result.error || message; taskCreated = !!result.task_id; duplicate = !!result.duplicate } catch {}
    throw Object.assign(new Error(message), { taskCreated, duplicate })
  }
  if (response.status === 204) return null
  return response.json()
}
function needsSync(t: Task) {
 return t.status === 'running' || t.status === 'queued' || (!!t.retryable && t.status === 'failed' && ((t.retry_count || 0) < preferences.value.auto_retry || (t.auto_engine && !t.headers_required && engines.value.some(e => e.ready && !(t.tried_engines || [t.engine]).includes(e.name)))))
}
function scheduleRefresh() {
  window.clearTimeout(timer)
  if (disposed || document.hidden) return
  const active = tasks.value.some(needsSync)
  if (active) timer = window.setTimeout(() => refresh(true), preferences.value.interval)
}
function visibilityChanged() {
  window.clearTimeout(timer)
  if (!document.hidden && tasks.value.some(needsSync)) void refresh(true)
}
async function refresh(silent = false) {
  if (refreshing) return
  refreshing = true
  if (!silent) loading.value = true
  try {
    const [engineData, taskData] = await Promise.all([api('/api/engines'), api('/api/tasks')])
    if (JSON.stringify(engines.value) !== JSON.stringify(engineData)) engines.value = engineData
    if (JSON.stringify(tasks.value) !== JSON.stringify(taskData)) tasks.value = taskData
    connected.value = true
    error.value = ''
  } catch (e) { connected.value = false; error.value = e instanceof Error ? e.message : '无法连接下载服务' }
  finally { loading.value = false; refreshing = false; scheduleRefresh() }
}
const duplicateDialog = ref<HTMLDialogElement | null>(null)
const duplicateCount = ref(0)
function canonicalURL(value: string) {
 try { const parsed = new URL(value); if(parsed.protocol==='magnet:'){const xt=parsed.searchParams.getAll('xt');for(const key of xt){const h=key.match(/^urn:btih:([a-f0-9]{40})$/i);if(h)return 'magnet:'+h[1].toLowerCase()};for(const key of xt){const h=key.match(/^urn:btmh:1220([a-f0-9]{64})$/i);if(h)return 'magnet:'+h[1].slice(0,40).toLowerCase()}} parsed.hash=''; return parsed.href } catch { return value.trim() }
}
async function addTask(duplicates: 'ask' | 'skip' | 'allow' = 'ask') {
  if (submitting.value || (!url.value.trim() && !hasTorrent.value)) return
  if (selectedEngine.value!=='auto' && !engines.value.some(e => e.name === selectedEngine.value && e.ready)) { createError.value = '所选下载引擎当前不可用，请选择其他引擎。'; createAdvanced.value = true; return }
  if(hasTorrent.value && url.value.trim()){createError.value='请分别提交链接和种子文件。';return}
  let torrentData = ''
  if(torrentFile.value){
    if(url.value.trim()){createError.value='请分别提交链接和种子文件。';return}
    if(torrentFile.value.size>8*1024*1024){createError.value='种子文件最大 8 MB。';return}
    submitting.value=true
    try{const bytes=new Uint8Array(await torrentFile.value.arrayBuffer());let binary='';for(let i=0;i<bytes.length;i+=8192)binary+=String.fromCharCode(...bytes.subarray(i,i+8192));torrentData=btoa(binary)}catch{createError.value='无法读取种子文件';return}finally{submitting.value=false}
  }
  let links = url.value.split(/\r?\n/).map(v => v.trim()).filter(Boolean)
  let headers: Record<string,string> = {}
  try {
    for (const link of links) { const parsed = new URL(link); if (!['http:','https:','magnet:'].includes(parsed.protocol)) throw new Error('请输入有效的 HTTP、HTTPS 或磁力链接。') }
    if (links.length > 1000) throw new Error('一次最多添加 1000 个链接。')
    for (const line of (supportsHeaders.value ? requestHeaders.value : '').split(/\r?\n/).filter(v => v.trim())) {
      const index = line.indexOf(':'); if (index < 1) throw new Error('请求头格式应为 名称: 值')
      headers[line.slice(0,index).trim()] = line.slice(index+1).trim()
    }
    if (links.length > 1 && Object.keys(headers).length) throw new Error('自定义请求头请用于单文件任务。')
  } catch (e) { createError.value = e instanceof Error ? e.message : '链接格式无效'; return }
  submitting.value = true
  createError.value = ''
  try {
    // Refresh once at submission; the server repeats this check under its queue lock.
    const existing: Task[] = await api('/api/tasks')
    const seen = new Set(existing.map(t => canonicalURL(t.source)))
    const unique: string[] = []
    let count = 0
    for (const link of links) {
      const key = canonicalURL(link)
      if (seen.has(key)) count++
      else unique.push(link)
      seen.add(key)
    }
    if (count && duplicates === 'ask') {
      duplicateCount.value = count
      duplicateDialog.value?.showModal()
      submitting.value = false
      return
    }
    if (duplicates === 'skip') links = unique
    duplicateDialog.value?.close()
  } catch (e) {
    createError.value = e instanceof Error ? e.message : '无法检查重复任务'
    submitting.value = false
    return
  }
  const failed: string[] = []
  let created = 0
  let startFailed = 0
  let firstError = ''
  try {
    if(hasTorrent.value) links.push('')
    // Each URL goes through the ordinary task endpoint and owns its own task ID.
    // Submit in input order so creation timestamps also preserve batch ordering.
    for (const link of links) {
      try {
        await api('/api/tasks', { method: 'POST', body: JSON.stringify({ engine: selectedEngine.value, url: link, torrent:torrentData || undefined, torrent_path:torrentPath.value || undefined, directory: directory.value.trim(), headers, allow_duplicate: duplicates === 'allow' }) })
        created++
      } catch (e) {
        if (e instanceof Error && 'taskCreated' in e && e.taskCreated) { created++; startFailed++ }
        else failed.push(link)
        if (!firstError) firstError = e instanceof Error ? e.message : '创建任务失败'
      }
    }
    url.value = failed.join('\n')
    filter.value = 'all'
    search.value = ''
    if (created) notify(`已创建 ${created} 个独立下载任务`)
    else if (!failed.length) notify('重复链接已跳过，没有新增任务')
    if (failed.length) createError.value = `${failed.length} 个链接创建失败，已保留在输入框，可重新提交。${firstError}`
    else createDialog.value?.close()
    if (startFailed) notify(`已创建 ${created} 个任务，其中 ${startFailed} 个启动失败，请在任务列表查看。`)
    await refresh()
  } finally { submitting.value = false }
}
const selectedIDs = ref<string[]>([])
const bulkBusy = ref(false)
const bulkRemoveDialog = ref<HTMLDialogElement | null>(null)
const selectedVisible = computed(() => filteredTasks.value.filter(t => selectedIDs.value.includes(t.id)))
function selectVisible(event: Event) {
 const checked = (event.target as HTMLInputElement).checked
 selectedIDs.value = checked ? [...new Set([...selectedIDs.value, ...filteredTasks.value.map(t => t.id)])] : selectedIDs.value.filter(id => !filteredTasks.value.some(t => t.id === id))
}
async function bulkAction(name: 'pause' | 'resume' | 'retry' | 'delete') {
 if (bulkBusy.value || pendingTask.value) return
 const chosen = selectedVisible.value.filter(t => name === 'pause' ? ['running','queued'].includes(t.status) : name === 'resume' ? t.status === 'paused' : name === 'retry' ? t.status === 'failed' : true)
 if (!chosen.length) { notify('没有可执行此操作的任务'); return }
 bulkBusy.value = true
 operationError.value = ''
 const failed: string[] = []
 let done = 0
 try {
  for (const task of chosen) {
   try {
    await api(`/api/tasks/${encodeURIComponent(task.id)}/${name}`, {method:'POST'})
    done++
    selectedIDs.value = selectedIDs.value.filter(id => id !== task.id)
   } catch { failed.push(task.name || task.source) }
  }
  bulkRemoveDialog.value?.close()
  if (failed.length) operationError.value = `${failed.length} 个任务操作失败，已保留选择：${failed.slice(0,3).join('、')}`
  notify(`已处理 ${done} 个任务`)
  await refresh()
 } finally { bulkBusy.value = false }
}
async function action(task: Task, actionName: string) {
  if (pendingTask.value || bulkBusy.value) return
  pendingTask.value = task.id
  operationError.value = ''
  try {
    await api(`/api/tasks/${encodeURIComponent(task.id)}/${actionName}`, { method: 'POST' })
    if (actionName.startsWith('delete')) {
      removeDialog.value?.close()
      if (selectedTask.value?.id === task.id) detailsDialog.value?.close()
    }
    notify(actionName.startsWith('delete') ? (actionName==='delete-files' ? '任务和下载文件已删除' : '任务已移除，下载文件已保留') : actionName === 'pause' ? '已发送暂停请求' : actionName === 'cancel-retry' ? '已取消此任务的自动重试' : actionName === 'retry' ? '任务已加入重试队列' : '已发送继续请求')
    await refresh()
  } catch (e) { operationError.value = e instanceof Error ? e.message : '操作失败' }
  finally { pendingTask.value = '' }
}
onMounted(() => { void loadAppSettings().catch(e => notify(e instanceof Error ? e.message : '无法加载应用设置')); refresh(); document.addEventListener('visibilitychange', visibilityChanged); systemTheme.addEventListener('change',systemThemeChanged); window.addEventListener('message',directoryMessage) })
onUnmounted(() => { disposed = true; window.clearTimeout(timer); document.removeEventListener('visibilitychange', visibilityChanged); window.clearTimeout(notificationTimer); systemTheme.removeEventListener('change',systemThemeChanged); window.removeEventListener('message',directoryMessage) })
</script>

<template>
  <div class="app-layout">
    <aside class="sidebar" aria-label="侧边导航">
      <div class="sidebar-brand">
      <a class="brand" href="#" @click.prevent="filter='all'; search=''" aria-label="MoxYi Download 全部任务">
        <div class="brand-icon"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 4v11m-4-4 4 4 4-4M5 17v3h14v-3" /></svg></div>
        <div><strong>MoxYi<span>Download</span></strong><small>让下载井然有序</small></div>
      </a>
        <button class="theme-toggle" @click="cycleTheme" :disabled="!appSettingsLoaded || savingTheme" :aria-label="activeThemeOption.label+'，点击切换为'+nextThemeOption.label" :title="activeThemeOption.label+' · 点击切换为'+nextThemeOption.label"><svg viewBox="0 0 24 24" aria-hidden="true"><path :d="activeThemeOption.path" /></svg></button>
      </div>
      <div class="nav-caption">下载管理</div>
      <nav class="side-nav" aria-label="下载任务分类">
        <button v-for="item in navigation" :key="item.id" :class="{selected:filter===item.id}" :aria-current="filter===item.id ? 'page' : undefined" @click="filter=item.id">
          <svg viewBox="0 0 24 24" aria-hidden="true"><path :d="item.path" /></svg>
          <span>{{ item.label }}</span><small :class="{nonzero:taskCount(item.id)>0}">{{ taskCount(item.id) }}</small>
        </button>
        <button @click="openSettings" :class="{selected:filter==='settings'}" :aria-current="filter==='settings' ? 'page' : undefined"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 7h16M4 17h16M8 4v6M16 14v6" /></svg><span>设置</span></button>
      </nav>
    </aside>
  <main class="shell">
    <template v-if="filter!=='settings'">
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
      <div class="panel-heading task-heading"><div><h2>任务列表 <span class="task-count">{{ filteredTasks.length }}</span></h2><p>下载进度自动同步</p></div><label class="search-field"><svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="10" cy="10" r="6"/><path d="m15 15 5 5"/></svg><input v-model="search" type="search" placeholder="搜索文件或链接" aria-label="搜索文件或链接" /></label></div>
      <div v-if="error || operationError" class="error-box" role="alert">{{ error || operationError }}</div>
      <div v-if="filteredTasks.length" class="bulk-toolbar"><label><input type="checkbox" :checked="selectedVisible.length===filteredTasks.length" :indeterminate="selectedVisible.length>0 && selectedVisible.length<filteredTasks.length" @change="selectVisible" :disabled="bulkBusy" /> 全选当前列表</label><span>已选 {{ selectedVisible.length }} 项</span><button class="secondary-button" @click="bulkAction('pause')" :disabled="!selectedVisible.length || bulkBusy || !!pendingTask">暂停</button><button class="secondary-button" @click="bulkAction('resume')" :disabled="!selectedVisible.length || bulkBusy || !!pendingTask">继续</button><button class="secondary-button" @click="bulkAction('retry')" :disabled="!selectedVisible.some(t=>t.status==='failed') || bulkBusy || !!pendingTask">重试失败</button><button class="secondary-button" @click="bulkRemoveDialog?.showModal()" :disabled="!selectedVisible.length || bulkBusy || !!pendingTask">移除</button><button class="secondary-button" @click="selectedIDs=filteredTasks.filter(t => t.status==='completed').map(t=>t.id); bulkRemoveDialog?.showModal()" :disabled="!filteredTasks.some(t=>t.status==='completed') || bulkBusy || !!pendingTask">清理已完成</button></div>
      <div v-if="filteredTasks.length" class="list-columns" aria-hidden="true"><span>文件名称 / 下载进度</span><span>操作</span></div>
      <div v-if="filteredTasks.length" class="task-list">
        <article v-for="task in filteredTasks" :key="task.id" class="task-row">
          <input class="task-select" type="checkbox" v-model="selectedIDs" :value="task.id" :aria-label="'选择任务：'+(task.name || task.source)" :disabled="bulkBusy" />
          <div class="task-main">
            <div class="task-title"><button class="task-name" @click="openDetails(task)" :title="'查看任务详情：'+(task.name || task.source)">{{ task.name || task.source }}</button><span class="status" :class="task.status">{{ statusLabel(task.status) }}</span></div>
            <div class="task-meta"><span>{{ task.engine }}</span><span>{{ formatBytes(task.completed_bytes) }} / {{ task.total_bytes ? formatBytes(task.total_bytes) : '大小未知' }}</span><span v-if="task.status==='running'">{{ formatBytes(task.speed_bytes) }}/s</span><span>已下载耗时 {{ formatDuration((task.elapsed_ms || 0)/1000) }}</span><span v-if="task.status==='running'">预计剩余 {{ remainingTime(task) }}</span></div>
            <div class="progress-line"><div class="progress" :class="task.status" role="progressbar" :aria-label="task.name + '下载进度'" :aria-valuenow="task.total_bytes>0 ? percent(task) : undefined" aria-valuemin="0" aria-valuemax="100"><i :style="{width: percent(task)+'%'}"></i></div><span>{{ task.total_bytes>0 ? percent(task)+'%' : '大小未知' }}</span></div>
            <small v-if="task.retry_at" class="hint">计划于 {{ new Date(task.retry_at).toLocaleTimeString() }} 自动重试 · 已重试 {{ task.retry_count || 0 }}/{{ preferences.auto_retry }} 次 <button class="text-button" @click="action(task,'cancel-retry')" :disabled="!!pendingTask || bulkBusy">取消自动重试</button></small>
            <small v-if="task.engine_note" class="hint">{{ task.engine_note }}</small>
            <small v-if="task.error" class="task-error">{{ task.error }}</small>
          </div>
          <div class="task-actions">
            <button @click="openTaskDirectory(task)" :disabled="openingDirectory || !task.directory" title="打开保存目录" :aria-label="'打开保存目录：'+task.name"><svg width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" aria-hidden="true"><path d="M3 7V5h6l2 2h10v13H3z" /></svg></button>
            <button v-if="task.status==='failed'" @click="action(task,'retry')" :disabled="!!pendingTask || bulkBusy" title="重试" :aria-label="'重试 '+task.name">↻</button>
            <button v-else-if="task.status==='running'" @click="action(task,'pause')" :disabled="!!pendingTask || bulkBusy" title="暂停" :aria-label="'暂停 '+task.name">Ⅱ</button>
            <button v-else-if="task.status==='paused' || task.status==='queued'" @click="action(task,'resume')" :disabled="!!pendingTask || bulkBusy" title="继续" :aria-label="'继续 '+task.name">▶</button>
            <button @click="confirmRemove(task)" :disabled="!!pendingTask || bulkBusy" title="移除任务" :aria-label="'移除任务 '+task.name">×</button>
          </div>
        </article>
      </div>
      <div v-else class="empty-state" aria-live="polite"><div><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 3v12m-4-4 4 4 4-4M4 16v5h16v-5" /></svg></div><strong>{{ !connected ? (loading ? '正在加载任务' : '暂时无法获取任务') : search.trim() ? '没有找到匹配的任务' : filter==='all' ? '从一个链接开始' : '这里还没有'+currentSection+'的任务' }}</strong><span>{{ !connected ? '连接下载服务后，任务会显示在这里' : search.trim() ? '试试其他文件名，或清空搜索条件' : filter==='all' ? '新建下载任务，将文件保存到你的 NAS' : '切换到全部任务，查看其他下载记录' }}</span><button v-if="connected && (filter!=='all' || search.trim())" class="text-button" @click="filter='all';search=''">查看全部任务 →</button><button v-else-if="connected" class="text-button" @click="openCreate">添加第一个下载 →</button></div>
    </section>
    </template>
    <section v-else class="settings-page" aria-labelledby="settings-title">
      <div class="welcome"><div><div class="eyebrow">SETTINGS</div><h1 id="settings-title">设置</h1><p>管理下载偏好与下载引擎。</p></div></div>
      <div class="panel settings-panel">
    <form @submit.prevent="saveSettings" :aria-busy="savingSettings">
      <div class="settings-body create-form">
        <div class="directory-field"><span>默认保存目录</span><div class="directory-controls"><input aria-label="默认保存目录" :value="settingsDraft.directory || '应用默认下载目录'" readonly /><button type="button" class="secondary-button" @click="chooseDirectory(true)" :disabled="pickingDirectory">选择目录</button><button v-if="settingsDraft.directory" type="button" class="secondary-button" @click="settingsDraft.directory=''">重置</button></div><p v-if="directoryError" class="error-banner" role="alert">{{ directoryError }}</p></div>
        <label><span>最大同时下载数（所有引擎合计）</span><input v-model.number="settingsDraft.max_concurrent" type="number" min="1" max="100" required /><small>超过上限的任务按添加顺序等待；降低上限不会中断当前下载。应用重启后未完成任务保持暂停。</small></label>
        <label><span>下载进度同步间隔</span><select v-model.number="settingsDraft.interval"><option :value="2500">2.5 秒</option><option :value="5000">5 秒</option><option :value="10000">10 秒</option></select><small>仅有下载中、等待中或等待自动重试的任务时同步，页面隐藏时暂停。</small></label>
        <label class="disk-reserve-setting"><span>磁盘保留空间（MB）</span><input v-model.number="settingsDraft.disk_reserve_mb" type="number" min="64" max="102400" required /><small>启动前预估同卷任务所需空间，大小未知时先检查保留空间。下载中低于此值会暂停本应用任务，需释放空间后手动继续；排队任务在空间恢复后自动补位。</small></label>
        <label class="auto-retry-setting"><span>失败后自动重试</span><select v-model.number="settingsDraft.auto_retry"><option :value="0">关闭</option><option v-for="n in 5" :key="n" :value="n">最多 {{ n }} 次</option></select><small>仅对可确认的网络故障或临时服务器错误重试。间隔从 30 秒逐次加倍；授权、链接失效、磁盘和未知错误不自动重试。重启应用后需手动继续已有任务。</small></label>
        <fieldset v-if="qbSettingsLoaded" class="sdk-settings" :disabled="savingSettings">
          <legend>qBittorrent 引擎</legend>
          <label><span>总下载限速（KB/s，0 不限速）</span><input v-model.number="qbOptions.download_kb" type="number" min="0" max="1073741824" required /></label>
          <label><span>总上传限速（KB/s，0 不限速）</span><input v-model.number="qbOptions.upload_kb" type="number" min="0" max="1073741824" required /></label>
          <label><span>最大连接数</span><input v-model.number="qbOptions.connections" type="number" min="1" max="2000" required /></label>
          <label class="check-setting"><input v-model="qbOptions.dht" type="checkbox" /><span>启用 DHT 网络</span></label>
          <label class="check-setting"><input v-model="qbOptions.pex" type="checkbox" /><span>启用 Peer Exchange</span></label>
          <small>用于磁力和种子下载，下载期间会上传数据，完成后停止做种。配置与任务记录保存在本应用，设置不影响其他 qBittorrent。</small>
        </fieldset>
        <p v-else-if="qbSettingsError" class="error-box form-message" role="alert">{{ qbSettingsError }}</p>
        <fieldset v-if="aria2SettingsLoaded" class="sdk-settings" :disabled="savingSettings">
          <legend>aria2 引擎</legend>
          <label><span>每个任务的下载分段数</span><input v-model.number="aria2Options.split" type="number" min="1" max="16" required /></label>
          <label><span>单服务器最大连接数</span><input v-model.number="aria2Options.connections" type="number" min="1" max="16" required /></label>
          <label><span>最小分段大小（MB）</span><input v-model.number="aria2Options.min_split_mb" type="number" min="1" max="1024" required /></label>
          <label><span>aria2 总下载限速（KB/s，0 不限速）</span><input v-model.number="aria2Options.download_kb" type="number" min="0" max="1073741824" required /></label>
          <label><span>连接超时（秒）</span><input v-model.number="aria2Options.connect_timeout" type="number" min="1" max="600" required /></label>
          <label><span>无数据超时（秒）</span><input v-model.number="aria2Options.timeout" type="number" min="1" max="600" required /></label>
          <label><span>单次下载最多尝试次数</span><input v-model.number="aria2Options.max_tries" type="number" min="1" max="10" required /></label>
          <label><span>重试间隔（秒）</span><input v-model.number="aria2Options.retry_wait" type="number" min="0" max="600" required /></label>
          <small>多连接下载需要源站支持分段请求，不保证增加连接就能提速。分段与超时设置用于新任务及暂停后继续的任务；总限速立即生效。只影响本应用内置 aria2。</small>
        </fieldset>
        <p v-else-if="aria2SettingsError" class="error-box form-message" role="alert">{{ aria2SettingsError }}</p>
        <fieldset v-if="sdkSettingsLoaded" class="sdk-settings" :disabled="savingSettings || recovering">
          <legend>迅雷引擎</legend>
          <label><span>最大同时下载任务数</span><input v-model.number="xunleiOptions.concurrent" type="number" min="1" max="100" required /></label>
          <label><span>下载限速（KB/s，0 不限速）</span><input v-model.number="xunleiOptions.download_kb" type="number" min="0" max="4294967295" required /></label>
          <label><span>上传限速（KB/s，0 不限速）</span><input v-model.number="xunleiOptions.upload_kb" type="number" min="0" max="4294967295" required /></label>
          <label class="check-setting"><input v-model="xunleiOptions.upload" type="checkbox" /><span>允许 P2P 上传（消耗 NAS 上行带宽）</span></label>
          <label class="check-setting"><input v-model="xunleiOptions.dynamic_acceleration" type="checkbox" /><span>动态链接 P2SP 加速</span></label>
          <small>动态链接加速存在极低概率的文件不一致风险，重要文件请校验完整性。</small>
          <button type="button" class="secondary-button" @click="recoverSDKTasks" :disabled="recovering">{{ recovering ? '处理中…' : '重置' }}</button>
          <small>未完成任务恢复为暂停，不会自动开始下载。</small>
        </fieldset>
        <p class="hint">所有设置保存在应用中，更换浏览器或设备后仍然生效。</p>
        <div v-if="settingsError" class="error-box" role="alert">{{ settingsError }}</div>
      </div>
      <div class="settings-actions"><button type="button" class="secondary-button" @click="openSettings" :disabled="savingSettings || recovering || pickingDirectory">重置更改</button><button type="submit" class="primary-button" :disabled="pickingDirectory || savingSettings || recovering">{{ savingSettings ? '正在保存…' : '保存设置' }}</button></div>
    </form>
      </div>
    </section>
    <footer> MoxYi Download <span>·</span> fnOS 原生应用 <span>·</span> 多引擎下载管理</footer>
  </main>
  <dialog ref="createDialog" class="modal create-modal" aria-labelledby="create-title" @cancel="cancelWhileBusy($event, submitting)">
    <div class="modal-header"><div><span class="modal-eyebrow">NEW DOWNLOAD</span><h2 id="create-title">新建下载</h2><p>添加链接，其余交给下载器。</p></div><button class="close-button" @click="createDialog?.close()" :disabled="submitting" aria-label="关闭新建下载">×</button></div>
    <form @submit.prevent="addTask()" :aria-busy="submitting">
      <div class="modal-body create-form">
        <label class="url-field"><span>下载链接</span><textarea id="download-url" v-model="url" placeholder="粘贴 HTTP / HTTPS 或磁力链接" :required="!hasTorrent" autofocus :disabled="submitting" rows="3"></textarea><small>支持批量下载，每行一个链接。</small></label>
        <div class="torrent-source"><span>或导入种子文件</span><div class="torrent-actions"><button type="button" :disabled="submitting || pickingTorrent" @click="chooseNASTorrent">{{ pickingTorrent ? '选择中…' : 'NAS 文件' }}</button><button type="button" :disabled="submitting" @click="torrentInput?.click()">本地上传</button><button v-if="hasTorrent" type="button" :disabled="submitting" @click="clearTorrent">清除</button></div><input ref="torrentInput" hidden type="file" accept=".torrent,application/x-bittorrent" :disabled="submitting" @change="localTorrentChanged" /><small>{{ hasTorrent ? torrentName + (torrentPath ? ' · NAS 文件' : ' · 本地文件') : '支持 .torrent 文件，最大 8 MB' }}</small></div>
        <div class="directory-field"><span>保存目录</span><div class="directory-controls"><input aria-label="保存目录" :value="directory || '默认下载目录'" readonly /><button type="button" class="secondary-button" @click="chooseDirectory()" :disabled="submitting || pickingDirectory">{{ pickingDirectory ? '正在打开…' : '选择目录' }}</button><button v-if="directory" type="button" class="secondary-button" @click="directory=''" :disabled="submitting">重置</button></div><small>通过飞牛选择并授权 NAS 目录，未选择时使用默认目录。</small><p v-if="directoryError" role="alert" class="error-banner">{{ directoryError }}</p></div>
        <div class="create-advanced">
          <button type="button" class="advanced-toggle" :aria-expanded="createAdvanced" aria-controls="create-advanced-options" @click="createAdvanced=!createAdvanced"><span>高级设置</span><svg viewBox="0 0 24 24" :class="{expanded:createAdvanced}" aria-hidden="true"><path d="m8 10 4 4 4-4" /></svg></button>
          <div v-show="createAdvanced" id="create-advanced-options" class="advanced-options">
            <label><span>下载引擎</span><select v-model="selectedEngine" :disabled="submitting" aria-label="下载引擎"><option value="auto">自动智能选择（推荐）</option><option v-for="choice in engineChoices" :key="choice.name" :value="choice.name" :disabled="!engines.some(e => e.name===choice.name && e.ready)">{{ choice.label }}{{ engines.some(e => e.name===choice.name && e.ready) ? '' : '（不可用）' }}</option></select><small>{{ selectedEngine==='auto' ? '磁力和种子使用 qBittorrent；普通链接优先迅雷，其次 aria2、HTTP，并支持自动切换。' : '本次下载使用指定引擎，批量链接也使用此引擎。' }}</small></label>

        <template v-if="supportsHeaders">
          <label><span>自定义 HTTP 请求头（单文件任务）</span><textarea v-model="requestHeaders" rows="2" :disabled="submitting" placeholder="Referer: https://example.com/" /><small>每行一个 名称: 值。请求头不保存到应用数据库。</small></label>
        </template>
          </div>
        </div>
        <div v-if="createError" class="error-box form-message" role="alert">{{ createError }}</div>
        <p v-if="!engines.some(e => e.ready)" class="hint form-message">当前没有可用引擎，请稍后重试。</p>
      </div>
      <div class="modal-footer"><span>文件保存在你的 NAS</span><button class="secondary-button" type="button" @click="createDialog?.close()" :disabled="submitting">取消</button><button class="primary-button" type="submit" :disabled="submitting || !canCreate">{{ submitting ? '正在创建…' : '开始下载' }}</button></div>
    </form>
  </dialog>
  <dialog ref="duplicateDialog" class="modal remove-modal" aria-labelledby="duplicate-title" @cancel="cancelWhileBusy($event, submitting)">
    <div class="modal-header"><h2 id="duplicate-title">发现 {{ duplicateCount }} 个重复链接</h2></div>
    <div class="modal-body"><p>这些链接已在任务列表中，或在本次输入中重复出现。是否跳过？</p><p class="hint">再次下载会创建独立任务，文件保存到独立子目录。</p></div>
    <div class="modal-footer"><button class="secondary-button" @click="duplicateDialog?.close()" :disabled="submitting">返回编辑</button><button class="secondary-button" @click="addTask('allow')" :disabled="submitting">仍然下载全部</button><button class="primary-button" @click="addTask('skip')" :disabled="submitting">跳过重复链接</button></div>
  </dialog>
  <dialog ref="detailsDialog" class="modal detail-modal" aria-labelledby="details-title" @cancel="cancelWhileBusy($event, !!pendingTask)">
    <template v-if="detailTask">
      <div class="modal-header"><div><span class="modal-eyebrow">TASK DETAILS</span><h2 id="details-title">任务详情</h2></div><button class="close-button" @click="detailsDialog?.close()" :disabled="!!pendingTask || bulkBusy" aria-label="关闭任务详情">×</button></div>
      <div class="modal-body"><div class="detail-file"><div class="file-icon">↓</div><div><h3>{{ detailTask.name }}</h3><span class="status" :class="detailTask.status">{{ statusLabel(detailTask.status) }}</span></div></div>
        <div class="detail-progress"><div class="progress" :class="detailTask.status"><i :style="{width:percent(detailTask)+'%'}"></i></div><span>{{ detailTask.total_bytes>0 ? percent(detailTask)+'%' : '大小未知' }}</span></div>
        <dl class="detail-grid"><div><dt>已下载耗时</dt><dd>{{ formatDuration((detailTask.elapsed_ms || 0)/1000) }}</dd></div><div v-if="detailTask.status==='running'"><dt>预计剩余时间</dt><dd>{{ remainingTime(detailTask) }}</dd></div><div><dt>已下载 / 总大小</dt><dd>{{ formatBytes(detailTask.completed_bytes) }} / {{ detailTask.total_bytes>0 ? formatBytes(detailTask.total_bytes) : '大小未知' }}</dd></div><div><dt>下载速度</dt><dd>{{ formatBytes(detailTask.status==='running' ? detailTask.speed_bytes : 0) }}/s</dd></div><div><dt>下载引擎</dt><dd>{{ detailTask.engine }}</dd></div><div class="detail-wide"><dt>保存位置</dt><dd>{{ detailTask.directory || '默认下载目录' }}</dd></div><div class="detail-wide"><dt>源链接</dt><dd class="source-text">{{ detailTask.source }}</dd></div></dl>
        <dl v-if="sdkDetails" class="detail-grid"><div><dt>源站流量</dt><dd>{{ formatBytes(sdkDetails.traffic.origin) }}</dd></div><div><dt>P2P 流量</dt><dd>{{ formatBytes(sdkDetails.traffic.p2p) }}</dd></div><div><dt>P2S 流量</dt><dd>{{ formatBytes(sdkDetails.traffic.p2s) }}</dd></div><div v-if="sdkDetails.completed"><dt>完成时间</dt><dd>{{ new Date(sdkDetails.completed*1000).toLocaleString() }}</dd></div></dl>
        <p v-if="detailTask.engine_note" class="hint">{{ detailTask.engine_note }}</p>
        <p v-if="detailsError" class="error-box" role="alert">{{ detailsError }}</p>
        <div v-if="detailTask.error || operationError" class="error-box" role="alert">{{ operationError || detailTask.error }}</div>
      </div>
      <div class="modal-footer"><button class="secondary-button" @click="openTaskDirectory(detailTask)" :disabled="openingDirectory || !detailTask.directory">打开保存目录</button><button class="danger-text" @click="confirmRemove(detailTask)" :disabled="!!pendingTask || bulkBusy">移除任务</button><button class="secondary-button" @click="detailsDialog?.close()" :disabled="!!pendingTask || bulkBusy">关闭</button><button v-if="detailTask.status==='failed'" class="primary-button" @click="action(detailTask,'retry')" :disabled="!!pendingTask || bulkBusy">重试下载</button><button v-else-if="detailTask.status==='running'" class="primary-button" @click="action(detailTask,'pause')" :disabled="!!pendingTask || bulkBusy">{{ pendingTask ? '处理中…' : '暂停下载' }}</button><button v-else-if="detailTask.status==='paused' || detailTask.status==='queued'" class="primary-button" @click="action(detailTask,'resume')" :disabled="!!pendingTask || bulkBusy">{{ pendingTask ? '处理中…' : '继续下载' }}</button></div>
    </template>
  </dialog>
  <dialog ref="bulkRemoveDialog" class="modal remove-modal" @cancel="cancelWhileBusy($event, bulkBusy)">
    <div class="modal-header"><h2>移除选中的 {{ selectedVisible.length }} 个任务？</h2></div>
    <div class="modal-body"><p>正在下载的任务会停止，已下载的文件会保留。</p></div>
    <div class="modal-footer"><button class="secondary-button" @click="bulkRemoveDialog?.close()" :disabled="bulkBusy">取消</button><button class="danger-button" @click="bulkAction('delete')" :disabled="bulkBusy">{{ bulkBusy ? '正在移除…' : '确认移除' }}</button></div>
  </dialog>
  <dialog ref="removeDialog" class="modal remove-modal" aria-labelledby="remove-title" aria-describedby="remove-description" @cancel="cancelWhileBusy($event, !!pendingTask)">
    <div class="modal-header"><div><span class="modal-eyebrow">REMOVE TASK</span><h2 id="remove-title">移除这个下载任务？</h2></div><button class="close-button" @click="removeDialog?.close()" :disabled="!!pendingTask || bulkBusy" aria-label="关闭移除确认">×</button></div>
    <div class="modal-body"><p class="remove-name">{{ removalTask?.name }}</p><p id="remove-description">{{ removeFiles ? '任务及已下载的文件将被删除，无法撤销。' : '仅移除任务记录，已下载的文件会保留。正在下载的任务会停止。' }}</p><label class="check-setting"><input v-model="removeFiles" type="checkbox" :disabled="!!pendingTask || bulkBusy" /><span>同时删除下载文件（无法撤销）</span></label><div v-if="operationError" class="error-box" role="alert">{{ operationError }}</div></div>
    <div class="modal-footer"><button class="secondary-button" @click="removeDialog?.close()" :disabled="!!pendingTask || bulkBusy" autofocus>保留任务</button><button class="danger-button" @click="removalTask && action(removalTask,removeFiles ? 'delete-files' : 'delete')" :disabled="!!pendingTask || bulkBusy">{{ pendingTask ? '正在移除…' : '确认移除' }}</button></div>
  </dialog>
  <div v-if="notification" class="toast" role="status"><span>✓</span>{{ notification }}</div>
  </div>
</template>

