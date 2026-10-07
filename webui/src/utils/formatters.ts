import type { Device, DeviceDefinition, DeviceExpose } from '@/types'

export function inferDeviceType(dev: Device): { name: string; icon: string } {
  const inClusters = dev.input_clusters || []
  const outClusters = dev.output_clusters || []
  const model = (dev.model || '').toLowerCase()

  if (inClusters.includes(1030) || model.includes('motion') || model.includes('occupancy')) {
    return { name: 'Motion Sensor', icon: '🚶' }
  }
  if (inClusters.includes(1026) || inClusters.includes(1029) || model.includes('temp') || model.includes('weather')) {
    return { name: 'Climate Sensor', icon: '🌡️' }
  }
  if (outClusters.includes(6) || model.includes('switch') || model.includes('remote') || model.includes('button')) {
    return { name: 'Remote / Switch', icon: '🖲️' }
  }
  if (inClusters.includes(6) && inClusters.includes(8)) {
    return { name: 'Dimmable Light', icon: '💡' }
  }
  if (inClusters.includes(6) && inClusters.includes(768)) {
    return { name: 'Color Light', icon: '🎨' }
  }
  if (inClusters.includes(6)) {
    return { name: 'Smart Light / Switch', icon: '💡' }
  }
  if (inClusters.includes(2820) || model.includes('plug') || model.includes('outlet')) {
    return { name: 'Smart Plug', icon: '🔌' }
  }
  return { name: 'Zigbee Device', icon: '📦' }
}

export function getDeviceDisplayName(dev: Device): string {
  if (!dev) return 'Unknown'
  return (dev.friendly_name || dev.ieee || 'Unknown').trim()
}

export function sortDevicesByFriendlyName(devices: Device[]): Device[] {
  if (!Array.isArray(devices)) return devices
  return [...devices].sort((a, b) => {
    const nameA = (a.friendly_name || a.ieee || '').trim()
    const nameB = (b.friendly_name || b.ieee || '').trim()
    const cmp = nameA.localeCompare(nameB, undefined, { numeric: true, sensitivity: 'base' })
    if (cmp !== 0) return cmp
    return (a.ieee || '').localeCompare(b.ieee || '')
  })
}

export function formatSignalQuality(lqi: number) {
  const percent = Math.min(100, Math.round((Math.max(0, lqi) / 255) * 100))
  if (lqi >= 180) {
    return { label: 'Excellent', colorClass: 'text-emerald-500 bg-emerald-500/15 border-emerald-500/30', percent }
  }
  if (lqi >= 110) {
    return { label: 'Good', colorClass: 'text-blue-500 bg-blue-500/15 border-blue-500/30', percent }
  }
  if (lqi >= 50) {
    return { label: 'Fair', colorClass: 'text-amber-500 bg-amber-500/15 border-amber-500/30', percent }
  }
  return { label: 'Poor', colorClass: 'text-rose-500 bg-rose-500/15 border-rose-500/30', percent }
}

export function formatUptime(sec: number): string {
  const s = Math.floor(sec || 0)
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const remSec = s % 60
  if (h > 0) return `${h}h ${m}m ${remSec}s`
  if (m > 0) return `${m}m ${remSec}s`
  return `${remSec}s`
}

export function formatHex(val: number, pad = 4): string {
  if (val == null || isNaN(val)) return '--'
  return `0x${val.toString(16).toUpperCase().padStart(pad, '0')}`
}

export function getExposeIcon(prop: string, type?: string): string {
  const p = (prop || '').toLowerCase()
  if (p === 'state') return '⭐'
  if (p.includes('power_outage')) return '💾'
  if (p.includes('indicator')) return '🔆'
  if (p.includes('lock')) return '🔒'
  if (p.includes('countdown') || p.includes('timer')) return '⏱️'
  if (p.includes('voltage') || p.includes('current') || p.includes('power') || p.includes('watt')) return '⚡'
  if (p.includes('energy') || p.includes('kwh')) return '🌱'
  if (p.includes('temp')) return '🌡️'
  if (p.includes('hum')) return '💧'
  if (p.includes('occupancy') || p.includes('motion')) return '🚶'
  if (p.includes('contact') || p.includes('door') || p.includes('window')) return '🚪'
  if (p.includes('water') || p.includes('leak')) return '💧'
  if (p.includes('smoke')) return '🔥'
  if (p.includes('linkquality') || p.includes('lqi')) return '📶'
  if (p.includes('battery')) return '🔋'
  if (p.includes('identify')) return '✋'
  if (type === 'binary') return '💡'
  if (type === 'action') return '▶️'
  return '⚙️'
}

export function getDeviceExposes(dev: Device, def?: DeviceDefinition | null): DeviceExpose[] {
  let exposes: DeviceExpose[] = []

  if (def?.device?.exposes && Array.isArray(def.device.exposes) && def.device.exposes.length > 0) {
    exposes = [...def.device.exposes]
  } else {
    // Fallback cluster heuristics
    const inClusters = dev.input_clusters || []
    const outClusters = dev.output_clusters || []

    if (inClusters.includes(6) || outClusters.includes(6)) {
      exposes.push({
        type: 'binary',
        name: 'State',
        property: 'state',
        description: 'On/off state of the switch',
        values: ['OFF', 'ON'],
        access: 3,
      })
    }
    if (inClusters.includes(8)) {
      exposes.push({
        type: 'numeric',
        name: 'Brightness',
        property: 'brightness',
        description: 'Brightness level of the light',
        min: 0,
        max: 254,
        unit: '',
        access: 3,
      })
    }
    if (inClusters.includes(1026)) {
      exposes.push({
        type: 'numeric',
        name: 'Temperature',
        property: 'temperature',
        description: 'Measured temperature',
        unit: '°C',
        access: 1,
      })
    }
    if (inClusters.includes(1029)) {
      exposes.push({
        type: 'numeric',
        name: 'Humidity',
        property: 'humidity',
        description: 'Measured relative humidity',
        unit: '%',
        access: 1,
      })
    }
    if (inClusters.includes(1030)) {
      exposes.push({
        type: 'binary',
        name: 'Occupancy',
        property: 'occupancy',
        description: 'Indicates whether the device detected occupancy',
        values: ['CLEAR', 'OCCUPIED'],
        access: 1,
      })
    }
    if (inClusters.includes(2820)) {
      exposes.push(
        {
          type: 'numeric',
          name: 'Power',
          property: 'power',
          description: 'Instantaneous electrical power',
          unit: 'W',
          access: 1,
        },
        {
          type: 'numeric',
          name: 'Voltage',
          property: 'voltage',
          description: 'Measured electrical potential value',
          unit: 'V',
          access: 1,
        },
        {
          type: 'numeric',
          name: 'Current',
          property: 'current',
          description: 'Instantaneous measured electrical current',
          unit: 'A',
          access: 1,
        }
      )
    }
  }

  // Add simulations actions if available
  if (def?.device?.simulations?.actions) {
    const existingProps = new Set(exposes.map((e) => e.property))
    for (const act of Object.keys(def.device.simulations.actions)) {
      if (!existingProps.has(act)) {
        exposes.push({
          type: 'action',
          name: act.charAt(0).toUpperCase() + act.slice(1),
          property: act,
          description: `Trigger ${act} action`,
          access: 2,
        })
      }
    }
  }

  // Ensure linkquality is included
  if (!exposes.some((e) => e.property === 'linkquality')) {
    exposes.push({
      type: 'numeric',
      name: 'Linkquality',
      property: 'linkquality',
      description: 'Link quality (signal strength)',
      unit: 'lqi',
      min: 0,
      max: 255,
      access: 1,
    })
  }

  return exposes
}
