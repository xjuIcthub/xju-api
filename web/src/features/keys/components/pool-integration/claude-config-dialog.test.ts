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

import { buildClaudeSettingsJson } from './claude-config'

describe('Claude Code configuration', () => {
  test('uses the deployment root without /v1 and normalizes the API key', () => {
    const config = JSON.parse(
      buildClaudeSettingsJson('https://xju.example/', 'test-key')
    )

    expect(config).toEqual({
      enableWorkflows: true,
      env: {
        ANTHROPIC_BASE_URL: 'https://xju.example',
        ANTHROPIC_AUTH_TOKEN: 'sk-test-key',
        ANTHROPIC_MODEL: 'claude-opus-5',
        ANTHROPIC_DEFAULT_HAIKU_MODEL: 'claude-haiku-4-5-20251001',
        ANTHROPIC_DEFAULT_SONNET_MODEL: 'claude-sonnet-5',
        ANTHROPIC_DEFAULT_OPUS_MODEL: 'claude-opus-5',
        CLAUDE_CODE_DISABLE_ALTERNATE_SCREEN: '1',
      },
      permissions: {
        allow: [
          'Skill(update-config)',
          'Bash(command -v claude)',
          'Bash(claude --version)',
          'Bash(env)',
        ],
        defaultMode: 'bypassPermissions',
      },
      skipAutoPermissionPrompt: true,
      skipWorkflowUsageWarning: true,
      tui: 'default',
      ultracode: true,
      skipDangerousModePermissionPrompt: true,
    })
  })
})
