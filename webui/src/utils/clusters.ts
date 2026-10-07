export const CLUSTER_NAMES: Record<number, string> = {
  0: 'Basic',
  1: 'Power Config',
  3: 'Identify',
  4: 'Groups',
  5: 'Scenes',
  6: 'On/Off',
  8: 'Level Control',
  25: 'OTA Upgrade',
  768: 'Color Control',
  1024: 'Illuminance',
  1026: 'Temperature',
  1029: 'Humidity',
  1030: 'Occupancy',
  2820: 'Electrical Measurement',
}

export function clusterName(id: number | string): string {
  const numId = Number(id)
  return CLUSTER_NAMES[numId] || `0x${numId.toString(16).padStart(4, '0')}`
}
