// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import './assets/main.css'

import { createApp } from 'vue'
import { createPinia } from 'pinia'

import App from './App.vue'
import router from './router'
import { initAuthGuard } from './services/authGuard'

const app = createApp(App)

app.use(createPinia())
app.use(router)

// After the router settles, not before: it reads window.location when its
// module is imported, and would put the re-auth param straight back.
router.isReady().then(initAuthGuard)

app.mount('#app')
