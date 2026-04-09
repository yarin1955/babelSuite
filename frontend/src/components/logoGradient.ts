export const LOGO_GRADIENTS = [
  'linear-gradient(135deg, #173b5b 0%, #1f7ea8 100%)',
  'linear-gradient(135deg, #1a3a5c 0%, #0DADEA 100%)',
  'linear-gradient(135deg, #2c1654 0%, #7e3fb3 100%)',
  'linear-gradient(135deg, #0f3d2b 0%, #18BE94 100%)',
  'linear-gradient(135deg, #3d1f0a 0%, #f77530 100%)',
  'linear-gradient(135deg, #1a0f3d 0%, #5b4ee8 100%)',
  'linear-gradient(135deg, #3d0f1a 0%, #e84e6e 100%)',
  'linear-gradient(135deg, #1a3d0f 0%, #5cb84e 100%)',
]

export function logoGradient(seed: string): string {
  let h = 0
  for (let i = 0; i < seed.length; i++) h = (Math.imul(31, h) + seed.charCodeAt(i)) | 0
  return LOGO_GRADIENTS[Math.abs(h) % LOGO_GRADIENTS.length]
}
