import { mount } from 'svelte'
import App from './App.svelte'
import LogWindow from './LogWindow.svelte'
import { LOG_WINDOW_QUERY } from './lib/events'

// 独立日志窗口和主窗口共用同一份前端产物，靠入口参数分流。
// 不另开路由：打包后是 embed 进来的静态资源，多一条路由就要在资源目录里
// 伪造一份对应结构，而查询参数在开发服务器和生产包里都直接可用。
const target = document.getElementById('app')!
const isLogWindow = new URLSearchParams(location.search).has(LOG_WINDOW_QUERY)

if (isLogWindow) {
  mount(LogWindow, { target })
} else {
  mount(App, { target })
}
