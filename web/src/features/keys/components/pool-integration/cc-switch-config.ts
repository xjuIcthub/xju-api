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
  model: 'gpt-5.6-sol',
  haikuModel: 'gpt-5.6-luna',
  sonnetModel: 'gpt-5.6-terra',
  opusModel: 'gpt-5.6-sol',
} as const

export type AppType = 'claude' | 'codex' | 'gemini'
export type Models = Record<string, string>

export function endpointForApp(app: AppType): string {
  return app === 'codex' ? `${PUBLIC_API_ENDPOINT}/v1` : PUBLIC_API_ENDPOINT
}

function resolvedClaudeModels(models: Models) {
  return {
    model: models.model || XJU_CLAUDE_DEFAULT_MODELS.model,
    haikuModel: models.haikuModel || XJU_CLAUDE_DEFAULT_MODELS.haikuModel,
    sonnetModel: models.sonnetModel || XJU_CLAUDE_DEFAULT_MODELS.sonnetModel,
    opusModel: models.opusModel || XJU_CLAUDE_DEFAULT_MODELS.opusModel,
  }
}

export function buildClaudeConfig(
  token: string,
  models: Models,
  endpoint = PUBLIC_API_ENDPOINT
) {
  const resolved = resolvedClaudeModels(models)
  return {
    env: {
      ANTHROPIC_BASE_URL: endpoint.replace(/\/+$/, ''),
      ANTHROPIC_AUTH_TOKEN: token,
      ANTHROPIC_MODEL: resolved.model,
      ANTHROPIC_DEFAULT_HAIKU_MODEL: resolved.haikuModel,
      ANTHROPIC_DEFAULT_SONNET_MODEL: resolved.sonnetModel,
      ANTHROPIC_DEFAULT_OPUS_MODEL: resolved.opusModel,
    },
  }
}

export function buildCCSwitchURL(
  app: AppType,
  name: string,
  models: Models,
  apiKey: string
): string {
  const params = new URLSearchParams()
  const resolved = app === 'claude' ? resolvedClaudeModels(models) : models
  params.set('resource', 'provider')
  params.set('app', app)
  params.set('name', name)
  params.set('endpoint', endpointForApp(app))
  params.set('apiKey', apiKey)
  for (const [key, value] of Object.entries(resolved)) {
    if (value) params.set(key, value)
  }
  params.set('homepage', PUBLIC_API_ENDPOINT)
  params.set('enabled', 'true')
  return `ccswitch://v1/import?${params.toString()}`
}
