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
import { describe, expect, mock, test } from 'bun:test'

const calls: Array<{ url: string; body: unknown }> = []
mock.module('@/lib/api', () => ({
  api: {
    post: async (url: string, body: unknown) => {
      calls.push({ url, body })
      return {
        data: {
          success: true,
          data: { pool_id: 'edu', status: 'provisioning' },
        },
      }
    },
  },
}))
const { createPool } = await import('./api')

describe('createPool', () => {
  test('T3.7 sends explicit mode', async () => {
    calls.length = 0
    await createPool('Edu', 'codex', 'gopool')
    expect(calls[0].body).toEqual({
      label: 'Edu',
      provider: 'codex',
      mode: 'gopool',
    })
  })
  test('T3.7 defaults mode to cliproxy', async () => {
    calls.length = 0
    await createPool('Edu', 'claude')
    expect(calls[0].body).toEqual({
      label: 'Edu',
      provider: 'claude',
      mode: 'cliproxy',
    })
  })
})
