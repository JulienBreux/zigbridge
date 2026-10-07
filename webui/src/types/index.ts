export interface CoordinatorInfo {
  type?: string
  version?: string
  ieee?: string
  channel?: number
  pan_id?: number
  ext_pan_id?: string
  status?: string
  online?: boolean
}

export interface StatusResponse {
  connected: boolean
  transport_status: string
  mqtt_connected: boolean
  device_count: number
  binding_count: number
  permit_join_remaining: number
  uptime_seconds: number
  coordinator?: CoordinatorInfo
  ai_enabled?: boolean
  version?: string
  commit?: string
}

export type BridgeStatus = StatusResponse

export interface Device {
  ieee: string
  nwk: number
  friendly_name?: string
  model?: string
  manufacturer?: string
  endpoints?: number[]
  input_clusters?: number[]
  output_clusters?: number[]
  lqi: number
  battery?: number
  voltage?: number
  power_source?: string
  last_seen?: string
  state?: Record<string, any>
}

export interface DeviceExpose {
  type: 'binary' | 'numeric' | 'enum' | 'action' | string
  name: string
  property: string
  description?: string
  access?: number // Bit 0: read (1), Bit 1: write (2) -> 3 = read/write
  unit?: string
  min?: number
  max?: number
  values?: (string | number | boolean)[]
}

export interface DeviceEndpointDefinition {
  endpoint: number
  input_clusters?: number[]
  output_clusters?: number[]
}

export interface DeviceDefinition {
  device?: {
    model?: string
    vendor?: string
    description?: string
    zigbee_models?: string[]
    endpoints?: DeviceEndpointDefinition[]
    exposes?: DeviceExpose[]
    simulations?: {
      actions?: Record<string, any>
      telemetry?: Record<string, any>
    }
  }
}

export interface DeviceDetailResponse {
  device: Device
  definition?: DeviceDefinition
}

export interface Binding {
  src_ieee: string
  src_endpoint: number
  cluster_id: number
  dst_ieee: string
  dst_endpoint: number
}

export interface CreateBindingPayload {
  source_ieee: string
  source_ep: number
  target_ieee: string
  target_ep: number
  cluster_id: number
}

export interface Recommendation {
  id: string
  type: string
  source_ieee: string
  source_ep?: number
  target_ieee: string
  target_ep?: number
  cluster_id: number
  confidence: number
  reasoning: string
  status?: 'pending' | 'applied' | string
}

export interface ActivityItem {
  id: string
  iconType: 'light' | 'motion' | 'switch' | 'join' | 'system'
  title: string
  meta: string
  iconClass: string
  time: string
}

export interface RawLogItem {
  id: string
  time: string
  type: string
  message: string
  typeClass: 'join' | 'frame' | 'binding' | 'warn' | 'error'
}

export interface DefinitionSummary {
  model: string
  vendor: string
  description: string
  zigbee_models: string[]
  actions: string[]
  has_battery: boolean
  has_temperature: boolean
  has_humidity: boolean
}

export interface VirtualDeviceSummary {
  ieee: string
  nwk: number
  model: string
  vendor: string
  description: string
  state: Record<string, any>
  actions: string[]
  has_battery: boolean
  has_temperature: boolean
  has_humidity: boolean
}

export interface TestStatusResponse {
  supported: boolean
  adapter: string
  devices: number
  active_virtual_devices: number
  definitions: number
}

export type TabId = 'devices' | 'bindings' | 'ai' | 'events' | 'diagnostics' | 'simulation'
export type UserMode = 'simple' | 'advanced'
