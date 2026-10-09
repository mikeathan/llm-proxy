import { createApp, ref, watchEffect } from 'vue'
import './style.css'
import App from './App.vue'
import { createAppRouter } from './router'
import { installStaleChunkReload } from './router/staleChunkReload'
import { useTheme } from './composables/ui/useTheme'
import { startRunNotifications } from './composables/assistant/useRunNotifications'
import { documentTitle } from './utils/documentTitle'

// App bootstrap: the one place app-level lifecycle is wired. theme-boot.js has
// already painted the stored theme; useTheme takes over (OS listener, persistence).
useTheme().start()

const router = createAppRouter()

// After a deploy, an open tab's page chunks are gone: reload once onto the new build instead of failing silently.
installStaleChunkReload(router)

// Run notifications ride the existing global run poll (plan D5): started once here.
const runNotifications = startRunNotifications(router)

// Page title + unread run count, e.g. "(2) Activity · <app>" (D20, D25).
const pageTitle = ref<string | undefined>()
router.afterEach((to) => {
  pageTitle.value = to.meta.title
})
watchEffect(() => {
  document.title = documentTitle(pageTitle.value, runNotifications.unreadCount.value)
})

createApp(App).use(router).mount('#app')
