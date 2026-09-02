import { parse } from 'yaml'
import jaYaml from './ja.yml?raw'

const translations = parse(jaYaml) as Record<string, unknown>

export function t(key: string, values: Record<string, string | number> = {}): string {
  const value = key.split('.').reduce<unknown>((current, part) => {
    if (!current || typeof current !== 'object') return undefined
    return (current as Record<string, unknown>)[part]
  }, translations)

  if (typeof value !== 'string') return key
  return value.replace(/\{(\w+)\}/g, (_, name: string) => String(values[name] ?? `{${name}}`))
}
