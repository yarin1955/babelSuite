import type { ProfileRecord } from './api'
import { parseSimpleYaml } from './simpleYaml'

export const NEW_PROFILE_ID = '__new__'

export interface MergeRow {
  key: string
  baseValue: string
  overrideValue: string
  resultValue: string
  source: string
  conflicted: boolean
}

export function createDraftProfile(profiles: ProfileRecord[]): ProfileRecord {
  const nextIndex = profiles.filter((profile) => profile.launchable).length + 1

  return {
    id: NEW_PROFILE_ID,
    name: `New Profile ${nextIndex}`,
    fileName: `profile-${nextIndex}.yaml`,
    description: 'New suite-scoped execution context for an environment override.',
    scope: 'Local',
    default: false,
    extendsId: profiles.find((item) => !item.launchable)?.id ?? profiles[0]?.id ?? '',
    yaml: 'env:\n  LOG_LEVEL: info\n',
    secretRefs: [],
    launchable: true,
    updatedAt: new Date().toISOString(),
  }
}

export function toUpsertPayload(profile: ProfileRecord) {
  return {
    name: profile.name,
    fileName: profile.fileName,
    description: profile.description,
    scope: profile.scope,
    yaml: profile.yaml,
    secretRefs: profile.secretRefs,
    default: profile.default,
    extendsId: profile.extendsId ?? '',
  }
}

export function buildMergeRows(baseYaml: string, overrideYaml: string) {
  const baseParsed = parseSimpleYaml(baseYaml).values
  const overrideParsed = parseSimpleYaml(overrideYaml).values
  const keys = Array.from(new Set([...Object.keys(baseParsed), ...Object.keys(overrideParsed)])).sort()

  return keys.map((key): MergeRow => {
    const baseValue = baseParsed[key] ?? ''
    const overrideValue = overrideParsed[key] ?? ''
    const resultValue = overrideValue || baseValue || ''
    const conflicted = Boolean(baseValue && overrideValue && baseValue !== overrideValue)

    return {
      key,
      baseValue,
      overrideValue,
      resultValue,
      source: overrideValue ? 'Override' : 'Base',
      conflicted,
    }
  })
}
