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
import { Anthropic } from '@lobehub/icons'
import type { Row } from '@tanstack/react-table'
import {
  Trash2,
  Edit,
  Power,
  PowerOff,
  ArrowRightLeft,
  Copy,
  Link,
  Loader2,
} from 'lucide-react'
import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { IconCodex } from '@/assets/custom/icon-codex'
import ccSwitchIcon from '@/assets/vendor/cc-switch/app-icon.png'
import { DataTableRowActionMenu } from '@/components/data-table/core/row-action-menu'
import { Button } from '@/components/ui/button'
import {
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
} from '@/components/ui/dropdown-menu'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { encodeChannelConnectionInfo } from '@/lib/channel-connection-info'
import { copyToClipboard } from '@/lib/copy-to-clipboard'

import { updateApiKeyStatus } from '../api'
import { API_KEY_STATUS, ERROR_MESSAGES, SUCCESS_MESSAGES } from '../constants'
import type { ApiKeyProvider } from '../lib'
import { getPublicServerAddress } from '../lib/server-address'
import { apiKeySchema } from '../types'
import { useApiKeys } from './api-keys-provider'

type DataTableRowActionsProps<TData> = {
  row: Row<TData>
  provider?: ApiKeyProvider
}

export function DataTableRowActions<TData>({
  row,
  provider,
}: DataTableRowActionsProps<TData>) {
  const { t } = useTranslation()
  const apiKey = apiKeySchema.parse(row.original)
  const {
    setOpen,
    setCurrentRow,
    setCurrentProvider,
    triggerRefresh,
    setResolvedKey,
    resolveRealKey,
    resolvedKeys,
    loadingKeys,
  } = useApiKeys()
  const isEnabled = apiKey.status === API_KEY_STATUS.ENABLED
  const [isTogglingStatus, setIsTogglingStatus] = useState(false)
  const resolvedRealKey = resolvedKeys[apiKey.id]
  const isRealKeyLoading = Boolean(loadingKeys[apiKey.id])
  const isCodexPool = provider === 'codex'
  const isClaudePool = provider === 'claude'
  const supportsClaudeCode = isCodexPool || isClaudePool

  const toggleLabel = isEnabled ? t('Disable') : t('Enable')

  const handleMenuOpenChange = useCallback(
    (open: boolean) => {
      if (open && !resolvedRealKey && !isRealKeyLoading) {
        void resolveRealKey(apiKey.id)
      }
    },
    [apiKey.id, isRealKeyLoading, resolvedRealKey, resolveRealKey]
  )

  const getCachedRealKey = useCallback(() => {
    if (resolvedRealKey) return resolvedRealKey
    void resolveRealKey(apiKey.id)
    toast.info(t('API key is loading, please try again in a moment'))
    return null
  }, [apiKey.id, resolvedRealKey, resolveRealKey, t])

  const handleToggleStatus = async (
    e?: React.MouseEvent<HTMLButtonElement>
  ) => {
    e?.stopPropagation()
    const newStatus = isEnabled
      ? API_KEY_STATUS.DISABLED
      : API_KEY_STATUS.ENABLED

    setIsTogglingStatus(true)
    try {
      const result = await updateApiKeyStatus(apiKey.id, newStatus)
      if (result.success) {
        const message = isEnabled
          ? t(SUCCESS_MESSAGES.API_KEY_DISABLED)
          : t(SUCCESS_MESSAGES.API_KEY_ENABLED)
        toast.success(message)
        triggerRefresh()
      } else {
        toast.error(result.message || t(ERROR_MESSAGES.STATUS_UPDATE_FAILED))
      }
    } catch {
      toast.error(t(ERROR_MESSAGES.UNEXPECTED))
    } finally {
      setIsTogglingStatus(false)
    }
  }

  let statusIcon = <Power className='size-4' />
  if (isTogglingStatus) {
    statusIcon = <Loader2 className='size-4 animate-spin' />
  } else if (isEnabled) {
    statusIcon = <PowerOff className='size-4' />
  }

  const openCodexConfig = async () => {
    if (!isCodexPool) return
    const realKey = await resolveRealKey(apiKey.id)
    if (!realKey) return
    setResolvedKey(realKey)
    setCurrentRow(apiKey)
    setOpen('codex-config')
  }

  const openCCSwitchConfig = async () => {
    const realKey = await resolveRealKey(apiKey.id)
    if (!realKey) return
    setResolvedKey(realKey)
    setCurrentRow(apiKey)
    setCurrentProvider(provider)
    setOpen('cc-switch')
  }

  const openClaudeConfig = async () => {
    if (!supportsClaudeCode) return
    const realKey = await resolveRealKey(apiKey.id)
    if (!realKey) return
    setResolvedKey(realKey)
    setCurrentRow(apiKey)
    setCurrentProvider(provider)
    setOpen('claude-config')
  }

  return (
    <div className='-ml-1.5 flex items-center gap-1'>
      {/* Codex config is the primary self-serve action for a day-card user, so
          it gets a direct button at the far left in addition to the menu item. */}
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant='ghost'
              size='icon-sm'
              onClick={openCodexConfig}
              disabled={isRealKeyLoading || !isCodexPool}
              aria-label={t('Codex Config')}
            />
          }
        >
          <IconCodex className='size-4' />
        </TooltipTrigger>
        <TooltipContent>{t('Codex Config')}</TooltipContent>
      </Tooltip>

      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant='ghost'
              size='icon-sm'
              onClick={openClaudeConfig}
              disabled={isRealKeyLoading || !supportsClaudeCode}
              aria-label={t('Claude Config')}
            />
          }
        >
          {isRealKeyLoading ? (
            <Loader2 className='size-4 animate-spin' />
          ) : (
            <Anthropic className='size-4' />
          )}
        </TooltipTrigger>
        <TooltipContent>{t('Claude Config')}</TooltipContent>
      </Tooltip>

      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant='ghost'
              size='icon-sm'
              onClick={openCCSwitchConfig}
              disabled={isRealKeyLoading}
              aria-label={t('CC Switch Configuration')}
            />
          }
        >
          {isRealKeyLoading ? (
            <Loader2 className='size-4 animate-spin' />
          ) : (
            <img src={ccSwitchIcon} alt='' className='size-4 rounded-sm' />
          )}
        </TooltipTrigger>
        <TooltipContent>{t('CC Switch Configuration')}</TooltipContent>
      </Tooltip>

      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant='ghost'
              size='icon-sm'
              onClick={handleToggleStatus}
              disabled={isTogglingStatus}
              aria-label={toggleLabel}
              className={
                isEnabled
                  ? 'text-destructive hover:text-destructive'
                  : 'text-emerald-600 hover:text-emerald-600 dark:text-emerald-400 dark:hover:text-emerald-400'
              }
            />
          }
        >
          {statusIcon}
        </TooltipTrigger>
        <TooltipContent>{toggleLabel}</TooltipContent>
      </Tooltip>

      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant='ghost'
              size='icon-sm'
              onClick={() => {
                setCurrentRow(apiKey)
                setOpen('update')
              }}
              aria-label={t('Edit')}
            />
          }
        >
          <Edit />
        </TooltipTrigger>
        <TooltipContent>{t('Edit')}</TooltipContent>
      </Tooltip>

      <DataTableRowActionMenu
        ariaLabel={t('Open menu')}
        contentClassName='w-[200px]'
        modal={false}
        onOpenChange={handleMenuOpenChange}
      >
        <DropdownMenuItem
          onClick={async () => {
            const realKey = getCachedRealKey()
            if (!realKey) return
            const ok = await copyToClipboard(realKey)
            if (ok) toast.success(t('Copied'))
          }}
        >
          {t('Copy Key')}
          <DropdownMenuShortcut>
            <Copy size={16} />
          </DropdownMenuShortcut>
        </DropdownMenuItem>
        <DropdownMenuItem
          onClick={async () => {
            const realKey = getCachedRealKey()
            if (!realKey) return
            const connStr = encodeChannelConnectionInfo(
              realKey,
              getPublicServerAddress()
            )
            const ok = await copyToClipboard(connStr)
            if (ok) toast.success(t('Copied'))
          }}
        >
          {t('Copy Connection Info')}
          <DropdownMenuShortcut>
            <Link size={16} />
          </DropdownMenuShortcut>
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem onClick={openCCSwitchConfig}>
          {t('CC Switch')}
          <DropdownMenuShortcut>
            <ArrowRightLeft size={16} />
          </DropdownMenuShortcut>
        </DropdownMenuItem>
        <DropdownMenuItem onClick={openCodexConfig} disabled={!isCodexPool}>
          {t('Codex Config')}
          <DropdownMenuShortcut>
            <IconCodex className='size-4' />
          </DropdownMenuShortcut>
        </DropdownMenuItem>
        <DropdownMenuItem
          onClick={openClaudeConfig}
          disabled={!supportsClaudeCode}
        >
          {t('Claude Config')}
          <DropdownMenuShortcut>
            <Anthropic className='size-4' />
          </DropdownMenuShortcut>
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem
          onClick={() => {
            setCurrentRow(apiKey)
            setOpen('delete')
          }}
          className='text-destructive focus:text-destructive'
        >
          {t('Delete')}
          <DropdownMenuShortcut>
            <Trash2 size={16} />
          </DropdownMenuShortcut>
        </DropdownMenuItem>
      </DataTableRowActionMenu>
    </div>
  )
}
