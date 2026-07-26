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
import { Check, Copy } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { copyToClipboard } from '@/lib/copy-to-clipboard'

import type { ApiKeyProvider } from '../../lib'
import { getPublicServerAddress } from '../../lib/server-address'
import { buildClaudeSettingsJson } from './claude-config'

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  tokenKey: string
  provider?: ApiKeyProvider
}

function CopyBlock(props: { title: string; hint: string; content: string }) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)

  const handleCopy = async () => {
    const ok = await copyToClipboard(props.content)
    if (!ok) return
    setCopied(true)
    window.setTimeout(() => setCopied(false), 2000)
  }

  return (
    <div className='min-w-0'>
      <div className='mb-2 flex items-center justify-between gap-2'>
        <div className='min-w-0'>
          <p className='text-sm font-medium'>{props.title}</p>
          <p className='text-muted-foreground text-xs'>{props.hint}</p>
        </div>
        <Button
          variant='outline'
          size='sm'
          onClick={handleCopy}
          className='shrink-0'
        >
          {copied ? <Check /> : <Copy />}
          {copied ? t('Copied') : t('Copy')}
        </Button>
      </div>
      <pre className='border-border bg-muted text-foreground max-h-80 overflow-auto rounded-md border p-3 font-mono text-xs leading-relaxed'>
        {props.content}
      </pre>
    </div>
  )
}

export function ClaudeConfigDialog(props: Props) {
  const { t } = useTranslation()
  const baseUrl = getPublicServerAddress()

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='sm:max-w-2xl'>
        <DialogHeader>
          <DialogTitle>{t('Claude Config')}</DialogTitle>
          <DialogDescription>
            {t(
              'Save this file for Claude Code. The endpoint does not include /v1 because Claude Code appends /v1/messages automatically.'
            )}
          </DialogDescription>
        </DialogHeader>

        <CopyBlock
          title={t('settings.json')}
          hint={t('Save as ~/.claude/settings.json')}
          content={buildClaudeSettingsJson(
            baseUrl,
            props.tokenKey,
            props.provider
          )}
        />

        <p className='text-muted-foreground text-xs'>
          {t(
            'The API key in this file is the full key. Treat settings.json as a secret and do not share it.'
          )}
        </p>
      </DialogContent>
    </Dialog>
  )
}
