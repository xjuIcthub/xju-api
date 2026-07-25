/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useQuery } from '@tanstack/react-query'
import { BarChart3 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { formatQuota } from '@/lib/format'

import { getUsersSummary } from '../api'
import { useUsers } from './users-provider'

export function UsersUsageSummary() {
  const { t } = useTranslation()
  const { refreshTrigger } = useUsers()
  const { data, isLoading } = useQuery({
    queryKey: ['users', 'summary', refreshTrigger],
    queryFn: async () => {
      const response = await getUsersSummary()
      if (!response.success || !response.data) {
        throw new Error(response.message || 'Failed to load user summary')
      }
      return response.data
    },
  })

  return (
    <Card data-card-hover='false' className='shrink-0 py-0'>
      <CardContent className='flex items-center gap-3 px-4 py-3 sm:px-5'>
        <div className='rounded-lg border bg-sky-500/8 p-2 text-sky-600 dark:text-sky-300'>
          <BarChart3 className='size-5' />
        </div>
        <div className='min-w-0 flex-1'>
          <div className='text-muted-foreground text-xs font-medium tracking-wide uppercase'>
            {t('All Users Total Usage')}
          </div>
          {isLoading ? (
            <Skeleton className='mt-1 h-7 w-32' />
          ) : (
            <div className='mt-0.5 font-mono text-xl font-bold tracking-tight tabular-nums'>
              {formatQuota(data?.total_used_quota ?? 0)}
            </div>
          )}
        </div>
        <p className='text-muted-foreground hidden max-w-sm text-right text-xs md:block'>
          {t('Cumulative usage across Default and private pools')}
        </p>
      </CardContent>
    </Card>
  )
}
