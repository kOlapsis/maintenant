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

import { createRouter, createWebHistory } from 'vue-router'
import DefaultLayout from '@/layouts/DefaultLayout.vue'
import PublicLayout from '@/layouts/PublicLayout.vue'
import DashboardPage from '../pages/DashboardPage.vue'

// Lazy-loaded routes for code splitting
const ContainersPage = () => import('../pages/ContainersPage.vue')
const EndpointsPage = () => import('../pages/EndpointsPage.vue')
const HeartbeatsPage = () => import('../pages/HeartbeatsPage.vue')
const CertificatesPage = () => import('../pages/CertificatesPage.vue')
const AlertsPage = () => import('../pages/AlertsPage.vue')
const StatusAdminPage = () => import('../pages/StatusAdminPage.vue')
const WebhooksPage = () => import('../pages/WebhooksPage.vue')
const UpdatesPage = () => import('../pages/UpdatesPage.vue')
const PublicStatusPage = () => import('../pages/PublicStatusPage.vue')
const SecurityPosturePage = () => import('../pages/SecurityPosturePage.vue')
const ProPage = () => import('../pages/ProPage.vue')
const ServicesPage = () => import('../pages/ServicesPage.vue')
const TasksPage = () => import('../pages/TasksPage.vue')
const WorkloadsPage = () => import('../pages/WorkloadsPage.vue')
const PodsPage = () => import('../pages/PodsPage.vue')
const ClusterOverviewPage = () => import('../pages/ClusterOverviewPage.vue')
const NodesPage = () => import('../pages/NodesPage.vue')

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/',
      component: DefaultLayout,
      children: [
        { path: '', redirect: '/dashboard' },
        { path: 'dashboard', name: 'dashboard', component: DashboardPage },
        { path: 'containers', name: 'containers', component: ContainersPage },
        { path: 'endpoints', name: 'endpoints', component: EndpointsPage },
        { path: 'heartbeats', name: 'heartbeats', component: HeartbeatsPage },
        { path: 'certificates', name: 'certificates', component: CertificatesPage },
        { path: 'alerts/:tab(history|channels|silence)?', name: 'alerts', component: AlertsPage },
        { path: 'status-admin', name: 'status-admin', component: StatusAdminPage },
        { path: 'webhooks', name: 'webhooks', component: WebhooksPage },
        { path: 'updates', name: 'updates', component: UpdatesPage },
        { path: 'security', name: 'security', component: SecurityPosturePage },
        { path: 'services', name: 'services', component: ServicesPage },
        { path: 'tasks', name: 'tasks', component: TasksPage },
        { path: 'workloads', name: 'workloads', component: WorkloadsPage },
        { path: 'pods', name: 'pods', component: PodsPage },
        { path: 'cluster', name: 'cluster', component: ClusterOverviewPage },
        { path: 'nodes', name: 'nodes', component: NodesPage },
        { path: 'pro-edition', name: 'pro-edition', component: ProPage },

      ],
    },
    {
      path: '/status',
      component: PublicLayout,
      children: [{ path: '', name: 'status-public', component: PublicStatusPage }],
    },
  ],
})

export default router
