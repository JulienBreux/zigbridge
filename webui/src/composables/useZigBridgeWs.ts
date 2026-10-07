import { ref, onMounted, onUnmounted } from 'vue'

export type WsEventHandler = (type: string, payload: any) => void

export function useZigBridgeWs() {
  const isConnected = ref(false)
  let socket: WebSocket | null = null
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null
  let isUnmounted = false
  const listeners = new Set<WsEventHandler>()

  function connect() {
    if (isUnmounted) return

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const wsUrl = `${protocol}//${window.location.host}/api/events`

    try {
      socket = new WebSocket(wsUrl)

      socket.onopen = () => {
        isConnected.value = true
        dispatch('system', {
          title: 'System Online',
          meta: 'Connected to ZigBridge live activity stream.',
        })
      }

      socket.onmessage = (event) => {
        try {
          const msg = JSON.parse(event.data)
          const eventType = msg.type || 'event'
          const payload = msg.data || msg.payload || msg
          dispatch(eventType, payload)
        } catch (e) {
          console.error('Failed to parse WebSocket message:', e)
        }
      }

      socket.onclose = () => {
        isConnected.value = false
        if (!isUnmounted) {
          reconnectTimer = setTimeout(connect, 3000)
        }
      }

      socket.onerror = (err) => {
        console.warn('WebSocket error:', err)
        socket?.close()
      }
    } catch (err) {
      console.error('WebSocket connection error:', err)
      if (!isUnmounted) {
        reconnectTimer = setTimeout(connect, 3000)
      }
    }
  }

  function dispatch(type: string, payload: any) {
    for (const listener of listeners) {
      try {
        listener(type, payload)
      } catch (err) {
        console.error('Error in WS listener:', err)
      }
    }
  }

  function addEventListener(handler: WsEventHandler) {
    listeners.add(handler)
    return () => {
      listeners.delete(handler)
    }
  }

  onMounted(() => {
    isUnmounted = false
    connect()
  })

  onUnmounted(() => {
    isUnmounted = true
    if (reconnectTimer) clearTimeout(reconnectTimer)
    if (socket) {
      socket.close()
      socket = null
    }
    listeners.clear()
  })

  return {
    isConnected,
    addEventListener,
  }
}
