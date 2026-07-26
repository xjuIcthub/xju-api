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
import { describe, expect, test } from 'bun:test'

import {
  buildApiKeyGroupOptions,
  getApiKeyGroupProvider,
} from './api-key-groups'

describe('API key group options', () => {
  test('shows public pool labels and provider identities instead of raw keys', () => {
    const options = buildApiKeyGroupOptions(
      {
        default: { desc: 'Default', ratio: 1, provider: 'codex' },
        'claude-team': {
          desc: 'Claude Team',
          ratio: 1,
          provider: 'claude',
        },
      },
      ''
    )

    expect(options).toEqual([
      {
        value: 'default',
        label: 'Default',
        desc: 'Codex / OpenAI · default',
        ratio: 1,
        provider: 'codex',
        isPrivate: false,
      },
      {
        value: 'claude-team',
        label: 'Claude Team',
        desc: 'Claude · claude-team',
        ratio: 1,
        provider: 'claude',
        isPrivate: false,
      },
    ])
  })

  test('resolves the provider used by API key integration actions', () => {
    const options = buildApiKeyGroupOptions(
      {
        default: { desc: 'Default', ratio: 1, provider: 'codex' },
        claude: { desc: 'Claude', ratio: 1, provider: 'claude' },
      },
      ''
    )

    expect(getApiKeyGroupProvider(options, 'default')).toBe('codex')
    expect(getApiKeyGroupProvider(options, 'claude')).toBe('claude')
    expect(getApiKeyGroupProvider(options, 'missing')).toBe(undefined)
  })
})
