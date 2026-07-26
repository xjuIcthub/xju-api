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
import { PUBLIC_API_ENDPOINT, buildClaudeConfig } from './cc-switch-config'

function normalizeToken(tokenKey: string): string {
  if (!tokenKey) return ''
  return tokenKey.startsWith('sk-') ? tokenKey : `sk-${tokenKey}`
}

export function buildClaudeSettingsJson(
  baseUrl: string,
  tokenKey: string
): string {
  const endpoint = (baseUrl || PUBLIC_API_ENDPOINT).replace(/\/+$/, '')
  const config = buildClaudeConfig(normalizeToken(tokenKey), {}, endpoint)
  return JSON.stringify(
    {
      enableWorkflows: true,
      env: {
        ANTHROPIC_AUTH_TOKEN: config.env.ANTHROPIC_AUTH_TOKEN,
        ANTHROPIC_BASE_URL: config.env.ANTHROPIC_BASE_URL,
        ANTHROPIC_DEFAULT_HAIKU_MODEL: config.env.ANTHROPIC_DEFAULT_HAIKU_MODEL,
        ANTHROPIC_DEFAULT_OPUS_MODEL: config.env.ANTHROPIC_DEFAULT_OPUS_MODEL,
        ANTHROPIC_DEFAULT_SONNET_MODEL:
          config.env.ANTHROPIC_DEFAULT_SONNET_MODEL,
        ANTHROPIC_MODEL: config.env.ANTHROPIC_MODEL,
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
    },
    null,
    2
  )
}
