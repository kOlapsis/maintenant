export type UpdateUrgency = 'critical' | 'recommended' | 'available'

export function updateUrgency(riskScore: number): UpdateUrgency {
  if (riskScore >= 81) return 'critical'
  if (riskScore >= 31) return 'recommended'
  return 'available'
}
