export interface PasswordStrength {
  score: number
  label: string
  hint: string
}

export function evaluatePasswordStrength(password: string): PasswordStrength {
  let score = 0

  if (password.length >= 8) score += 1
  if (password.length >= 12) score += 1
  if (/[a-z]/.test(password) && /[A-Z]/.test(password)) score += 1
  if (/\d/.test(password)) score += 1
  if (/[^A-Za-z0-9]/.test(password)) score += 1

  if (score <= 1) {
    return { score: 1, label: 'Needs work', hint: 'Use more characters and mix letters, numbers, and symbols.' }
  }
  if (score === 2) {
    return { score: 2, label: 'Fair', hint: 'Add another word or symbol to make it harder to guess.' }
  }
  if (score === 3 || score === 4) {
    return { score: 3, label: 'Strong', hint: 'Good foundation. A longer passphrase would make it even better.' }
  }
  return { score: 4, label: 'Excellent', hint: 'Long, varied, and ready for production use.' }
}

