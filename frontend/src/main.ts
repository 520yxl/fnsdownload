import { createApp } from 'vue'
import App from './App.vue'
import './app.css'
import { TrimApp } from '@trimjs/web-app'

if (new URLSearchParams(location.search).has('directory-callback')) {
  const result = new TrimApp().parseAppAuthCallback(location.href)
  if (window.opener && !window.opener.closed) {
    window.opener.postMessage({ type: 'fnsdownload:directory', result }, location.origin)
    window.close()
  }
  document.getElementById('app')!.textContent = '目录授权已结束，请返回下载器。'
} else {
  createApp(App).mount('#app')
}
