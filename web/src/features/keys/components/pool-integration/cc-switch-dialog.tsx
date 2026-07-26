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
import { useQuery } from '@tanstack/react-query'
import { ChevronDown, Copy, ExternalLink, Eye, EyeOff } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { ComboboxInput } from '@/components/ui/combobox-input'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { Textarea } from '@/components/ui/textarea'
import { getUserModels } from '@/lib/api'
import { copyToClipboard } from '@/lib/copy-to-clipboard'

import type { ApiKeyProvider } from '../../lib'
import {
  buildCCSwitchURL,
  buildClaudeConfig,
  endpointForApp,
  getCCSwitchDefaultModels,
  type AppType,
  type Models,
} from './cc-switch-config'

const APP_CONFIGS = {
  claude: {
    label: 'Claude',
    defaultName: 'XJU API - Claude',
  },
  codex: {
    label: 'Codex',
    defaultName: 'XJU API - Codex',
  },
  gemini: {
    label: 'Gemini',
    defaultName: 'XJU API - Gemini',
  },
} as const

const CLAUDE_ADVANCED_MODELS = [
  { key: 'haikuModel', labelKey: 'Haiku Model' },
  { key: 'sonnetModel', labelKey: 'Sonnet Model' },
  { key: 'opusModel', labelKey: 'Opus Model' },
] as const

type ClaudeAdvancedModelKey = (typeof CLAUDE_ADVANCED_MODELS)[number]['key']

function normalizedToken(tokenKey: string): string {
  if (!tokenKey) return ''
  return tokenKey.startsWith('sk-') ? tokenKey : `sk-${tokenKey}`
}

function maskedToken(token: string): string {
  if (!token) return ''
  if (token.length <= 12) return `${token.slice(0, 3)}••••••`
  return `${token.slice(0, 7)}••••••••${token.slice(-4)}`
}

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  tokenKey: string
  provider?: ApiKeyProvider
}

export function CCSwitchDialog(props: Props) {
  const { t } = useTranslation()
  const providerDefaults = getCCSwitchDefaultModels(props.provider)
  const [app, setApp] = useState<AppType>('claude')
  const [name, setName] = useState<string>(APP_CONFIGS.claude.defaultName)
  const [models, setModels] = useState<Models>({
    ...providerDefaults,
  })
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const [showToken, setShowToken] = useState(false)

  const { data: modelsData } = useQuery({
    queryKey: ['user-models-ccswitch'],
    queryFn: getUserModels,
    enabled: props.open,
    staleTime: 5 * 60 * 1000,
  })

  const modelOptions = useMemo(() => {
    const items = modelsData?.data ?? []
    return items.map((model) => ({ value: model, label: model }))
  }, [modelsData?.data])

  const token = normalizedToken(props.tokenKey)
  const endpoint = endpointForApp(app)
  const deepLink = useMemo(
    () => buildCCSwitchURL(app, name, models, token, providerDefaults),
    [app, models, name, providerDefaults, token]
  )
  const maskedConfigJSON = useMemo(
    () =>
      JSON.stringify(
        buildClaudeConfig(
          showToken ? token : maskedToken(token),
          models,
          undefined,
          providerDefaults
        ),
        null,
        2
      ),
    [models, providerDefaults, showToken, token]
  )
  const fullConfigJSON = useMemo(
    () =>
      JSON.stringify(
        buildClaudeConfig(token, models, undefined, providerDefaults),
        null,
        2
      ),
    [models, providerDefaults, token]
  )

  useEffect(() => {
    if (!props.open) return
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setApp('claude')
    setName(APP_CONFIGS.claude.defaultName)
    setModels({ ...providerDefaults })
    setAdvancedOpen(false)
    setShowToken(false)
  }, [props.open, providerDefaults])

  const handleAppChange = (value: string) => {
    const nextApp = value as AppType
    setApp(nextApp)
    setName(APP_CONFIGS[nextApp].defaultName)
    setModels(nextApp === 'claude' ? { ...providerDefaults } : {})
    setAdvancedOpen(false)
  }

  const handlePrimaryModelChange = (value: string) => {
    setModels((previous) => ({ ...previous, model: value }))
  }

  const handleAdvancedModelChange = (
    key: ClaudeAdvancedModelKey,
    value: string
  ) => {
    setModels((previous) => ({ ...previous, [key]: value }))
  }

  const ensureReady = () => {
    if (!models.model) {
      toast.warning(t('Please select a primary model'))
      return false
    }
    if (!token) {
      toast.error(t('API key is loading, please try again in a moment'))
      return false
    }
    return true
  }

  const copyValue = async (value: string, successMessage = 'Copied') => {
    const copied = await copyToClipboard(value)
    if (copied) toast.success(t(successMessage))
  }

  const handleOpenCCSwitch = () => {
    if (!ensureReady()) return
    window.location.href = deepLink
    toast.info(
      t('If CC Switch does not open, copy the Config JSON or Deep Link below')
    )
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('CC Switch Configuration')}
      contentClassName='sm:max-w-2xl'
      contentHeight='min(86vh, 780px)'
      bodyClassName='space-y-5 overflow-y-auto'
      footer={
        <>
          <Button variant='outline' onClick={() => props.onOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button onClick={handleOpenCCSwitch}>
            <ExternalLink className='size-4' />
            {t('Open CC Switch')}
          </Button>
        </>
      }
    >
      <div className='space-y-5'>
        <div className='space-y-2'>
          <Label>{t('Application')}</Label>
          <RadioGroup
            value={app}
            onValueChange={handleAppChange}
            className='flex flex-wrap gap-4'
          >
            {(
              Object.entries(APP_CONFIGS) as [
                AppType,
                (typeof APP_CONFIGS)[AppType],
              ][]
            ).map(([key, config]) => (
              <div key={key} className='flex items-center gap-2'>
                <RadioGroupItem value={key} id={`app-${key}`} />
                <Label htmlFor={`app-${key}`} className='cursor-pointer'>
                  {config.label}
                </Label>
              </div>
            ))}
          </RadioGroup>
        </div>

        <div className='grid gap-4 sm:grid-cols-2'>
          <div className='space-y-2'>
            <Label>{t('Name')}</Label>
            <ComboboxInput
              options={[]}
              value={name}
              onValueChange={setName}
              placeholder={APP_CONFIGS[app].defaultName}
              emptyText=''
              allowCustomValue
            />
          </div>
          <div className='space-y-2'>
            <Label>
              {t('Primary Model')}
              <span className='text-destructive ml-0.5'>*</span>
            </Label>
            <ComboboxInput
              options={modelOptions}
              value={models.model || ''}
              onValueChange={handlePrimaryModelChange}
              placeholder={t('Select or enter model name')}
              emptyText={t('No models found')}
            />
          </div>
        </div>

        <section className='bg-muted/35 space-y-3 rounded-lg border p-4'>
          <div className='space-y-2'>
            <div className='flex items-center justify-between gap-3'>
              <Label>{t('API Key')}</Label>
              <Button
                type='button'
                variant='ghost'
                size='sm'
                onClick={() => setShowToken((visible) => !visible)}
              >
                {showToken ? (
                  <EyeOff className='size-4' />
                ) : (
                  <Eye className='size-4' />
                )}
                {showToken ? t('Hide Token') : t('Show Token')}
              </Button>
            </div>
            <div className='flex gap-2'>
              <Input
                value={showToken ? token : maskedToken(token)}
                readOnly
                className='font-mono text-xs'
              />
              <Button
                type='button'
                variant='outline'
                size='icon'
                onClick={() => {
                  if (ensureReady()) {
                    void copyValue(token)
                  }
                }}
                aria-label={t('Copy API key')}
              >
                <Copy className='size-4' />
              </Button>
            </div>
          </div>
          <div className='space-y-2'>
            <div className='flex items-center justify-between gap-3'>
              <Label>{t('API Endpoint')}</Label>
              <span className='text-muted-foreground text-xs'>
                {t('Full URL')}: {t('No')}
              </span>
            </div>
            <div className='flex gap-2'>
              <Input value={endpoint} readOnly className='font-mono text-xs' />
              <Button
                type='button'
                variant='outline'
                size='icon'
                onClick={() => void copyValue(endpoint)}
                aria-label={t('Copy Endpoint')}
              >
                <Copy className='size-4' />
              </Button>
            </div>
          </div>
          {app === 'claude' && (
            <div className='text-sm'>
              <p className='text-amber-700 dark:text-amber-300'>
                {t('Claude Code automatically appends /v1/messages')}
                <span className='mx-1'>·</span>
                {t('Do not add /v1 to the Claude Endpoint')}
              </p>
            </div>
          )}
        </section>

        {app === 'claude' && (
          <section className='rounded-lg border'>
            <p className='text-muted-foreground border-b px-4 py-3 text-xs'>
              Opus → claude-opus-5 · Sonnet → claude-sonnet-5 · Haiku →
              claude-haiku-4-5
            </p>
            <Button
              type='button'
              variant='ghost'
              className='w-full justify-between rounded-lg px-4'
              onClick={() => setAdvancedOpen((open) => !open)}
            >
              <span>{t('Advanced model mapping')}</span>
              <ChevronDown
                className={`size-4 transition-transform ${advancedOpen ? 'rotate-180' : ''}`}
              />
            </Button>
            {advancedOpen && (
              <div className='grid gap-4 border-t p-4 sm:grid-cols-3'>
                {CLAUDE_ADVANCED_MODELS.map((field) => (
                  <div key={field.key} className='space-y-2'>
                    <Label>{t(field.labelKey)}</Label>
                    <ComboboxInput
                      options={modelOptions}
                      value={models[field.key] || models.model || ''}
                      onValueChange={(value) =>
                        handleAdvancedModelChange(field.key, value)
                      }
                      placeholder={t('Inherit primary model')}
                      emptyText={t('No models found')}
                    />
                  </div>
                ))}
              </div>
            )}
          </section>
        )}

        {app === 'claude' && (
          <section className='space-y-3 rounded-lg border p-4'>
            <div className='flex flex-wrap items-center justify-between gap-2'>
              <div>
                <Label>{t('Config JSON')}</Label>
                <p className='text-muted-foreground mt-1 text-xs'>
                  {t(
                    'The on-screen token is masked; copying includes the full token'
                  )}
                </p>
              </div>
              <div className='flex gap-2'>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  onClick={() => setShowToken((visible) => !visible)}
                >
                  {showToken ? (
                    <EyeOff className='size-4' />
                  ) : (
                    <Eye className='size-4' />
                  )}
                  {showToken ? t('Hide Token') : t('Show Token')}
                </Button>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  onClick={() => {
                    if (ensureReady()) {
                      void copyValue(fullConfigJSON, 'Config JSON copied')
                    }
                  }}
                >
                  <Copy className='size-4' />
                  {t('Copy Config JSON')}
                </Button>
              </div>
            </div>
            <Textarea
              value={maskedConfigJSON}
              readOnly
              spellCheck={false}
              className='min-h-64 resize-y font-mono text-xs'
            />
          </section>
        )}

        <section className='space-y-3 rounded-lg border p-4'>
          <div className='flex flex-wrap items-center justify-between gap-2'>
            <div>
              <Label>{t('CC Switch Deep Link')}</Label>
              <p className='text-muted-foreground mt-1 text-xs'>
                {t('The Deep Link contains your Token. Do not share it')}
              </p>
            </div>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={() => {
                if (ensureReady()) void copyValue(deepLink, 'Deep Link copied')
              }}
            >
              <Copy className='size-4' />
              {t('Copy Deep Link')}
            </Button>
          </div>
          <Input
            value={
              token ? deepLink.replace(token, maskedToken(token)) : deepLink
            }
            readOnly
            className='font-mono text-xs'
          />
          <p className='text-muted-foreground text-xs'>
            {t(
              'If CC Switch does not open, copy the Config JSON or Deep Link below'
            )}
          </p>
        </section>
      </div>
    </Dialog>
  )
}
