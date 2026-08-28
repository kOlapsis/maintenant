// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

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
