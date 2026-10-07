import type {
  StatusResponse,
  Device,
  DeviceDetailResponse,
  Binding,
  CreateBindingPayload,
  Recommendation,
  TestStatusResponse,
  DefinitionSummary,
  VirtualDeviceSummary,
} from '@/types'

async function request<T>(url: string, options: RequestInit = {}): Promise<T> {
  const headers = new Headers(options.headers || {})
  if (!headers.has('Content-Type') && options.body && typeof options.body === 'string') {
    headers.set('Content-Type', 'application/json')
  }

  const res = await fetch(url, {
    ...options,
    headers,
  })

  if (!res.ok) {
    let errorMsg = `HTTP ${res.status}`
    try {
      const errData = await res.json()
      if (errData && typeof errData.error === 'string') {
        errorMsg = errData.error
      }
    } catch {
      // Ignore JSON parse error on non-JSON response
    }
    throw new Error(errorMsg)
  }

  return res.json() as Promise<T>
}

export function useZigBridgeApi() {
  async function getStatus(): Promise<StatusResponse> {
    return request<StatusResponse>('/api/status')
  }

  async function permitJoin(duration: number): Promise<{ success: boolean; duration: number }> {
    return request<{ success: boolean; duration: number }>('/api/network/permit-join', {
      method: 'POST',
      body: JSON.stringify({ time: duration }),
    })
  }

  async function getDevices(): Promise<Device[]> {
    return request<Device[]>('/api/devices')
  }

  async function getDeviceDetail(ieee: string): Promise<DeviceDetailResponse> {
    return request<DeviceDetailResponse>(`/api/devices/${encodeURIComponent(ieee)}`)
  }

  async function renameDevice(ieee: string, friendlyName: string): Promise<{ success: boolean }> {
    return request<{ success: boolean }>(`/api/devices/${encodeURIComponent(ieee)}/rename`, {
      method: 'POST',
      body: JSON.stringify({ friendly_name: friendlyName }),
    })
  }

  async function setDeviceState(
    ieee: string,
    updates: Record<string, any>
  ): Promise<{ success: boolean; device?: Device }> {
    return request<{ success: boolean; device?: Device }>(`/api/devices/${encodeURIComponent(ieee)}/set`, {
      method: 'POST',
      body: JSON.stringify(updates),
    })
  }

  async function triggerDeviceAction(ieee: string, action: string): Promise<{ success: boolean; action: string }> {
    return request<{ success: boolean; action: string }>(`/api/devices/${encodeURIComponent(ieee)}/action`, {
      method: 'POST',
      body: JSON.stringify({ action }),
    })
  }

  async function getBindings(): Promise<Binding[]> {
    return request<Binding[]>('/api/bindings')
  }

  async function createBinding(payload: CreateBindingPayload): Promise<{ success: boolean; binding: Binding; warnings?: string[]; warning?: string }> {
    return request<{ success: boolean; binding: Binding; warnings?: string[]; warning?: string }>('/api/bindings', {
      method: 'POST',
      body: JSON.stringify(payload),
    })
  }

  async function deleteBinding(payload: CreateBindingPayload): Promise<{ success: boolean }> {
    return request<{ success: boolean }>('/api/bindings', {
      method: 'DELETE',
      body: JSON.stringify(payload),
    })
  }

  async function getRecommendations(): Promise<Recommendation[]> {
    return request<Recommendation[]>('/api/ai/recommendations')
  }

  async function applyRecommendation(id: string): Promise<{ success: boolean }> {
    return request<{ success: boolean }>(`/api/ai/recommendations/${encodeURIComponent(id)}/apply`, {
      method: 'POST',
    })
  }

  // Simulation Lab
  async function getTestStatus(): Promise<TestStatusResponse> {
    return request<TestStatusResponse>('/api/test/status')
  }

  async function getTestDefinitions(): Promise<DefinitionSummary[]> {
    return request<DefinitionSummary[]>('/api/test/definitions')
  }

  async function getTestDevices(): Promise<VirtualDeviceSummary[]> {
    return request<VirtualDeviceSummary[]>('/api/test/devices')
  }

  async function spawnTestDevice(payload: {
    model: string
    ieee?: string
    nwk?: number
  }): Promise<{ success: boolean }> {
    return request<{ success: boolean }>('/api/test/devices', {
      method: 'POST',
      body: JSON.stringify(payload),
    })
  }

  async function triggerTestDeviceAction(ieee: string, action: string): Promise<{ success: boolean }> {
    return request<{ success: boolean }>(`/api/test/devices/${encodeURIComponent(ieee)}/action`, {
      method: 'POST',
      body: JSON.stringify({ action }),
    })
  }

  async function sendTestDeviceTelemetry(
    ieee: string,
    telemetry: {
      battery?: number
      voltage?: number
      temperature?: number
      humidity?: number
    }
  ): Promise<{ success: boolean }> {
    return request<{ success: boolean }>(`/api/test/devices/${encodeURIComponent(ieee)}/telemetry`, {
      method: 'POST',
      body: JSON.stringify(telemetry),
    })
  }

  return {
    getStatus,
    permitJoin,
    setPermitJoin: permitJoin,
    getDevices,
    getDeviceDetail,
    getDevice: getDeviceDetail,
    renameDevice,
    setDeviceState,
    setDeviceProperty: setDeviceState,
    triggerDeviceAction,
    getBindings,
    createBinding,
    deleteBinding,
    getRecommendations,
    applyRecommendation,
    getTestStatus,
    getTestDefinitions,
    getTestDevices,
    getVirtualDevices: getTestDevices,
    spawnTestDevice,
    triggerTestDeviceAction,
    triggerVirtualDeviceAction: triggerTestDeviceAction,
    sendTestDeviceTelemetry,
    sendVirtualDeviceTelemetry: sendTestDeviceTelemetry,
  }
}
