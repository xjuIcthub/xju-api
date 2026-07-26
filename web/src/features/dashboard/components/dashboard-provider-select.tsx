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
// xju-api:new — shared Provider selector for the unified usage dashboard.
import { useTranslation } from 'react-i18next'

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import type { DashboardProvider } from '../types'

const DASHBOARD_PROVIDER_OPTIONS: DashboardProvider[] = [
  'all',
  'codex',
  'claude',
]

const DASHBOARD_PROVIDER_LABELS: Record<DashboardProvider, string> = {
  all: 'All',
  codex: 'Codex',
  claude: 'Claude',
}

interface DashboardProviderSelectProps {
  value: DashboardProvider
  onValueChange: (value: DashboardProvider) => void
}

export function DashboardProviderSelect({
  value,
  onValueChange,
}: DashboardProviderSelectProps) {
  const { t } = useTranslation()

  return (
    <Select
      value={value}
      onValueChange={(nextValue) =>
        onValueChange(nextValue as DashboardProvider)
      }
    >
      <SelectTrigger
        className='h-8 w-24 min-w-24'
        aria-label={t('Provider')}
        title={t('Provider')}
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent
        align='end'
        alignItemWithTrigger={false}
        className='w-24 min-w-24'
      >
        {DASHBOARD_PROVIDER_OPTIONS.map((provider) => (
          <SelectItem key={provider} value={provider}>
            {t(DASHBOARD_PROVIDER_LABELS[provider])}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}
