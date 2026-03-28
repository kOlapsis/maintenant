import { apiFetch } from './apiFetch'

const API_BASE = import.meta.env.VITE_API_BASE || '/api/v1'

export interface SwarmInfo {
  active: boolean
  cluster_id?: string
  is_manager?: boolean
  manager_count?: number
  worker_count?: number
  created_at?: string
}

export interface SwarmNetworkAttachment {
  network_id: string
  network_name: string
  scope: string
}

export interface SwarmPortConfig {
  protocol: string
  target_port: number
  published_port: number
  publish_mode: string
}

export interface SwarmServiceResponse {
  service_id: string
  name: string
  image: string
  mode: string
  desired_replicas: number
  running_replicas: number
  stack_name: string
  networks: SwarmNetworkAttachment[]
  ports: SwarmPortConfig[]
  labels: Record<string, string>
  created_at: string
  update_status?: {
    state: string
    started_at?: string
    completed_at?: string
    message?: string
  }
}

export interface SwarmTaskResponse {
  task_id: string
  slot: number
  state: string
  desired_state: string
  container_id: string
  node_id: string
  node_hostname: string
  error: string
  exit_code: number | null
  timestamp: string
}

export interface SwarmServiceDetailResponse extends SwarmServiceResponse {
  tasks: SwarmTaskResponse[]
}

export interface SwarmServiceListResponse {
  services: SwarmServiceResponse[]
  total: number
}

export function fetchSwarmInfo(): Promise<SwarmInfo> {
  return apiFetch<SwarmInfo>(`${API_BASE}/swarm/info`)
}

export function fetchSwarmServices(stack?: string): Promise<SwarmServiceListResponse> {
  const url = new URL(`${API_BASE}/swarm/services`, window.location.origin)
  if (stack) url.searchParams.set('stack', stack)
  return apiFetch<SwarmServiceListResponse>(url.toString())
}

export function fetchSwarmServiceDetail(serviceID: string): Promise<SwarmServiceDetailResponse> {
  return apiFetch<SwarmServiceDetailResponse>(`${API_BASE}/swarm/services/${serviceID}`)
}

export interface SwarmNodeResponse {
  id: number
  node_id: string
  hostname: string
  role: string
  status: string
  availability: string
  engine_version: string
  address: string
  task_count: number
  first_seen_at: string
  last_seen_at: string
  last_status_change_at: string
}

export interface SwarmNodeTaskResponse {
  task_id: string
  service_id: string
  service_name: string
  slot: number
  state: string
  image: string
  timestamp: string
}

export interface SwarmNodeDetailResponse extends SwarmNodeResponse {
  tasks: SwarmNodeTaskResponse[]
}

export interface SwarmNodeListResponse {
  nodes: SwarmNodeResponse[]
  total: number
  manager_count: number
  worker_count: number
}

export function fetchSwarmNodes(): Promise<SwarmNodeListResponse> {
  return apiFetch<SwarmNodeListResponse>(`${API_BASE}/swarm/nodes`)
}

export function fetchSwarmNodeDetail(nodeID: string): Promise<SwarmNodeDetailResponse> {
  return apiFetch<SwarmNodeDetailResponse>(`${API_BASE}/swarm/nodes/${nodeID}`)
}

export interface SwarmDashboardCluster {
  cluster_id: string
  manager_count: number
  worker_count: number
  service_count: number
  task_count: number
  healthy_task_count: number
}

export interface SwarmDashboardNode {
  node_id: string
  hostname: string
  role: string
  status: string
  availability: string
  task_count: number
}

export interface SwarmDashboardService {
  service_id: string
  name: string
  mode: string
  desired_replicas: number
  running_replicas: number
  update_state: string | null
  crash_loop: boolean
}

export interface SwarmDashboardResponse {
  cluster: SwarmDashboardCluster
  nodes: SwarmDashboardNode[]
  services: SwarmDashboardService[]
  recent_events: Array<{
    type: string
    service_name?: string
    node_hostname?: string
    message: string
    timestamp: string
  }>
}

export function fetchSwarmDashboard(): Promise<SwarmDashboardResponse> {
  return apiFetch<SwarmDashboardResponse>(`${API_BASE}/swarm/dashboard`)
}

export interface SwarmClusterResponse {
  cluster_id: string
  manager_count: number
  worker_count: number
  total_services: number
  running_tasks: number
  desired_tasks: number
  cluster_health: 'healthy' | 'degraded' | 'unhealthy'
  nodes: {
    ready: number
    down: number
    disconnected: number
  }
}

export function fetchSwarmCluster(): Promise<SwarmClusterResponse> {
  return apiFetch<SwarmClusterResponse>(`${API_BASE}/swarm/cluster`)
}

export interface SwarmTaskListResponse {
  tasks: (SwarmTaskResponse & { service_name: string })[]
  total: number
}

// --- Per-task resource usage (Enterprise) ---

export interface SwarmTaskResourceEntry {
  task_id: string
  slot: number
  node_hostname: string
  container_id: string
  cpu_percent: number | null
  mem_used: number | null
  mem_limit: number | null
  mem_percent: number | null
  net_rx_bytes: number | null
  net_tx_bytes: number | null
  timestamp: string | null
}

export interface SwarmServiceResourcesResponse {
  service_id: string
  service_name: string
  tasks: SwarmTaskResourceEntry[]
}

export function fetchSwarmServiceResources(serviceID: string): Promise<SwarmServiceResourcesResponse> {
  return apiFetch<SwarmServiceResourcesResponse>(`${API_BASE}/swarm/services/${serviceID}/resources`)
}

export function fetchSwarmTasks(params?: { service?: string; node?: string; state?: string }): Promise<SwarmTaskListResponse> {
  const url = new URL(`${API_BASE}/swarm/tasks`, window.location.origin)
  if (params?.service) url.searchParams.set('service', params.service)
  if (params?.node) url.searchParams.set('node', params.node)
  if (params?.state) url.searchParams.set('state', params.state)
  return apiFetch<SwarmTaskListResponse>(url.toString())
}
