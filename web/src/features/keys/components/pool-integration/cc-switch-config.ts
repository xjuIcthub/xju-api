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
export const PUBLIC_API_ENDPOINT = 'https://api.selab.top'

export const XJU_CLAUDE_DEFAULT_MODELS = {
  model: 'claude-opus-5',
  haikuModel: 'claude-haiku-4-5-20251001',
  sonnetModel: 'claude-sonnet-5',
  opusModel: 'claude-opus-5',
} as const

export const XJU_CODEX_DEFAULT_MODELS = {
  model: 'gpt-5.6-sol',
  haikuModel: 'gpt-5.6-luna',
  sonnetModel: 'gpt-5.6-terra',
  opusModel: 'gpt-5.6-sol',
} as const

export const XJU_CODEX_CLAUDE_COMPACTION_ENV = {
  CLAUDE_CODE_MAX_CONTEXT_TOKENS: '372000',
  CLAUDE_CODE_AUTO_COMPACT_WINDOW: '200000',
  CLAUDE_AUTOCOMPACT_PCT_OVERRIDE: '70',
} as const

export type AppType = 'claude' | 'codex' | 'gemini'
export type ClaudeCodePoolProvider = 'codex' | 'claude'
export type Models = Record<string, string>

export function endpointForApp(app: AppType): string {
  return app === 'codex' ? `${PUBLIC_API_ENDPOINT}/v1` : PUBLIC_API_ENDPOINT
}

export function getClaudeCodeDefaultModels(provider?: ClaudeCodePoolProvider) {
  return provider === 'claude'
    ? XJU_CLAUDE_DEFAULT_MODELS
    : XJU_CODEX_DEFAULT_MODELS
}

function resolvedClaudeModels(
  models: Models,
  defaults: Models = XJU_CLAUDE_DEFAULT_MODELS
) {
  return {
    model: models.model || defaults.model,
    haikuModel: models.haikuModel || defaults.haikuModel,
    sonnetModel: models.sonnetModel || defaults.sonnetModel,
    opusModel: models.opusModel || defaults.opusModel,
  }
}

export function buildClaudeConfig(
  token: string,
  models: Models,
  endpoint = PUBLIC_API_ENDPOINT,
  defaults: Models = XJU_CLAUDE_DEFAULT_MODELS,
  provider?: ClaudeCodePoolProvider
) {
  const resolved = resolvedClaudeModels(models, defaults)
  return {
    env: {
      ANTHROPIC_BASE_URL: endpoint.replace(/\/+$/, ''),
      ANTHROPIC_AUTH_TOKEN: token,
      ANTHROPIC_MODEL: resolved.model,
      ANTHROPIC_DEFAULT_HAIKU_MODEL: resolved.haikuModel,
      ANTHROPIC_DEFAULT_SONNET_MODEL: resolved.sonnetModel,
      ANTHROPIC_DEFAULT_OPUS_MODEL: resolved.opusModel,
      ...(provider === 'codex' ? XJU_CODEX_CLAUDE_COMPACTION_ENV : {}),
    },
  }
}

function encodeBase64JSON(value: unknown): string {
  const bytes = new TextEncoder().encode(JSON.stringify(value))
  let binary = ''
  for (const byte of bytes) binary += String.fromCharCode(byte)
  return btoa(binary)
}

export function buildCCSwitchURL(
  app: AppType,
  name: string,
  models: Models,
  apiKey: string,
  claudeDefaults: Models = XJU_CLAUDE_DEFAULT_MODELS,
  provider?: ClaudeCodePoolProvider
): string {
  const params = new URLSearchParams()
  const resolved =
    app === 'claude' ? resolvedClaudeModels(models, claudeDefaults) : models
  params.set('resource', 'provider')
  params.set('app', app)
  params.set('name', name)
  params.set('endpoint', endpointForApp(app))
  params.set('apiKey', apiKey)
  for (const [key, value] of Object.entries(resolved)) {
    if (value) params.set(key, value)
  }
  if (app === 'claude') {
    params.set('configFormat', 'json')
    params.set(
      'config',
      encodeBase64JSON(
        buildClaudeConfig(
          apiKey,
          models,
          PUBLIC_API_ENDPOINT,
          claudeDefaults,
          provider
        )
      )
    )
  }
  params.set('homepage', PUBLIC_API_ENDPOINT)
  params.set('enabled', 'true')
  return `ccswitch://v1/import?${params.toString()}`
}
