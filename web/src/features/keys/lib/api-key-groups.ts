/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
export type ApiKeyGroupOption = {
  value: string
  label: string
  desc?: string
  ratio?: number | string
  provider?: 'codex' | 'claude'
  isPrivate?: boolean
}

type UserGroupMap = Record<
  string,
  {
    desc: string
    ratio: number | string
    provider?: 'codex' | 'claude'
  }
>

function getProviderLabel(provider?: 'codex' | 'claude'): string {
  if (provider === 'claude') return 'Claude'
  if (provider === 'codex') return 'Codex / OpenAI'
  return ''
}

export function buildApiKeyGroupOptions(
  groups: UserGroupMap,
  privateGroup: string,
  username?: string
): ApiKeyGroupOption[] {
  return Object.entries(groups)
    .map(([value, info]) => {
      const isPrivate = privateGroup !== '' && value === privateGroup
      const displayName = info.desc || value
      const providerLabel = getProviderLabel(info.provider)
      return {
        value,
        label: isPrivate && username ? `@${username}` : displayName,
        desc: providerLabel ? `${providerLabel} · ${value}` : displayName,
        ratio: info.ratio,
        provider: info.provider,
        isPrivate,
      }
    })
    .sort((left, right) => {
      const rank = (option: ApiKeyGroupOption) => {
        if (option.isPrivate) return 0
        if (option.value === 'default') return 1
        return 2
      }
      return rank(left) - rank(right) || left.label.localeCompare(right.label)
    })
}

export function getPreferredApiKeyGroup(
  options: ApiKeyGroupOption[],
  fallback = 'default'
): string {
  return (
    options.find((option) => option.isPrivate)?.value ||
    options.find((option) => option.value === fallback)?.value ||
    options.find((option) => option.value === 'default')?.value ||
    options[0]?.value ||
    fallback
  )
}
