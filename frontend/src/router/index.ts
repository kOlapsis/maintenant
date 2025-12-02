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
        { path: 'alerts', name: 'alerts', component: AlertsPage },
        { path: 'status-admin', name: 'status-admin', component: StatusAdminPage },
        { path: 'webhooks', name: 'webhooks', component: WebhooksPage },
        { path: 'updates', name: 'updates', component: UpdatesPage },
      ],
    },
    {
      path: '/status',
      component: PublicLayout,
      children: [
        { path: '', name: 'status-public', component: PublicStatusPage },
      ],
    },
  ],
})

export default router
