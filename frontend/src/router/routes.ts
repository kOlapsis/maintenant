// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import type { RouteRecordRaw } from 'vue-router'
import PublicLayout from '@/layouts/PublicLayout.vue'

const DefaultLayout = () => import('@/layouts/DefaultLayout.vue')
const DashboardPage = () => import('../pages/DashboardPage.vue')
const ContainersPage = () => import('../pages/ContainersPage.vue')
const EndpointsPage = () => import('../pages/EndpointsPage.vue')
const HeartbeatsPage = () => import('../pages/HeartbeatsPage.vue')
const OutboundHeartbeatsPage = () => import('../pages/OutboundHeartbeatsPage.vue')
const CertificatesPage = () => import('../pages/CertificatesPage.vue')
const AlertsPage = () => import('../pages/AlertsPage.vue')
const StatusAdminPage = () => import('../pages/StatusAdminPage.vue')
const WebhooksPage = () => import('../pages/WebhooksPage.vue')
const UpdatesPage = () => import('../pages/UpdatesPage.vue')
const PublicStatusPage = () => import('../pages/PublicStatusPage.vue')
const SecurityPosturePage = () => import('../pages/SecurityPosturePage.vue')
const EditionsPage = () => import('../pages/EditionsPage.vue')
const ServicesPage = () => import('../pages/ServicesPage.vue')
const TasksPage = () => import('../pages/TasksPage.vue')
const WorkloadsPage = () => import('../pages/WorkloadsPage.vue')
const PodsPage = () => import('../pages/PodsPage.vue')
const ClusterOverviewPage = () => import('../pages/ClusterOverviewPage.vue')
const NodesPage = () => import('../pages/NodesPage.vue')
const EscalationPage = () => import('../pages/EscalationPage.vue')
const ChannelsPage = () => import('../pages/ChannelsPage.vue')
const AgentsPage = () => import('../pages/AgentsPage.vue')

export function buildRoutes(isStatusSubdomain: boolean): RouteRecordRaw[] {
  return [
    ...(isStatusSubdomain
      ? [
          {
            path: '/',
            component: PublicLayout,
            children: [{ path: '', name: 'status-public', component: PublicStatusPage }],
          },
        ]
      : [
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
              { path: 'alerts/:tab(history|triggers|silence|channels)?', name: 'alerts', component: AlertsPage },
              { path: 'channels', name: 'channels', component: ChannelsPage },
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
              { path: 'escalation', name: 'escalation', component: EscalationPage },
              { path: 'agents', name: 'agents', component: AgentsPage },
              { path: 'outbound-heartbeats', name: 'outbound-heartbeats', component: OutboundHeartbeatsPage },
              { path: 'editions', name: 'editions', component: EditionsPage },
              // The old offer page: keep the address working, send it to the comparison.
              { path: 'pro-edition', redirect: { name: 'editions' } },
            ],
          },
        ]),
    {
      path: '/status',
      component: PublicLayout,
      children: [{ path: '', name: isStatusSubdomain ? 'status-public-admin' : 'status-public', component: PublicStatusPage }],
    },
  ]
}
