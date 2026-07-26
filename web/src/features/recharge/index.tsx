/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  BadgeDollarSign,
  Clock3,
  Crown,
  Gift,
  History,
  KeyRound,
  ShieldCheck,
  Sparkles,
} from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import { StatusBadge } from '@/components/status-badge'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { getSelf } from '@/lib/api'
import { formatCurrencyFromUSD } from '@/lib/currency'
import {
  DEFAULT_POOL_REDEMPTION_RATE_LABEL,
  DEFAULT_POOL_USD_CREDIT_PER_CNY,
} from '@/lib/default-pool'
import { formatQuota, formatTimestamp } from '@/lib/format'
import { useAuthStore, type AuthUser } from '@/stores/auth-store'

import {
  getRechargeHistory,
  getRechargeInfo,
  redeemRechargeCode,
  type RechargeInfo,
} from './api'

const rewardMilestones = [
  { amount: '$50', label: 'Flowing gold username', icon: Sparkles },
  { amount: '$100', label: 'Silver crown', icon: Crown },
  { amount: '$1,000', label: 'Gold crown', icon: Crown },
]

const historySkeletons = ['history-1', 'history-2', 'history-3']

function paymentConfigured(info?: RechargeInfo) {
  if (!info) return false
  return Boolean(
    info.enable_online_topup ||
    info.enable_stripe_topup ||
    info.enable_creem_topup ||
    info.enable_waffo_topup ||
    info.enable_waffo_pancake_topup
  )
}

export function Recharge() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [code, setCode] = useState('')
  const authUser = useAuthStore((state) => state.auth.user)
  const setUser = useAuthStore((state) => state.auth.setUser)

  const userQuery = useQuery({
    queryKey: ['recharge', 'self'],
    queryFn: async () => {
      const response = await getSelf()
      if (!response?.success || !response.data) {
        throw new Error(response?.message || t('Failed to load balance'))
      }
      return response.data as AuthUser
    },
    initialData: authUser ?? undefined,
  })
  const infoQuery = useQuery({
    queryKey: ['recharge', 'info'],
    queryFn: getRechargeInfo,
  })
  const historyQuery = useQuery({
    queryKey: ['recharge', 'history'],
    queryFn: getRechargeHistory,
  })
  const info = infoQuery.data
  const providers = useMemo(
    () =>
      (info?.pay_methods ?? [])
        .map((method) => method.name?.trim())
        .filter((name): name is string => Boolean(name)),
    [info?.pay_methods]
  )
  const redeemMutation = useMutation({
    mutationFn: () => redeemRechargeCode(code.trim()),
    onSuccess: async (added) => {
      setCode('')
      const response = await getSelf()
      if (response?.success && response.data) {
        setUser(response.data as AuthUser)
        queryClient.setQueryData(['recharge', 'self'], response.data)
      }
      await queryClient.invalidateQueries({ queryKey: ['recharge', 'history'] })
      toast.success(
        t('Recharge successful. Added {{amount}}.', {
          amount: formatQuota(added),
        })
      )
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const historyContent = (() => {
    if (historyQuery.isLoading) {
      return (
        <div className='space-y-3'>
          {historySkeletons.map((key) => (
            <Skeleton key={key} className='h-14 w-full' />
          ))}
        </div>
      )
    }
    if (historyQuery.isError) {
      return (
        <div className='flex flex-col items-center gap-3 py-8 text-center'>
          <div className='text-muted-foreground text-sm'>
            {t('Failed to load')}
          </div>
          <Button variant='outline' onClick={() => historyQuery.refetch()}>
            {t('Retry')}
          </Button>
        </div>
      )
    }
    if ((historyQuery.data?.length ?? 0) === 0) {
      return (
        <div className='text-muted-foreground py-10 text-center text-sm'>
          {t('No recharge records yet')}
        </div>
      )
    }
    return (
      <div className='divide-y'>
        {historyQuery.data?.map((record) => (
          <div
            key={record.id}
            className='flex items-center justify-between gap-4 py-3 first:pt-0 last:pb-0'
          >
            <div className='min-w-0'>
              <div className='truncate font-mono text-xs'>
                {record.trade_no}
              </div>
              <div className='text-muted-foreground mt-1 text-xs'>
                {formatTimestamp(record.create_time)} ·{' '}
                {record.payment_method || t('Manual')}
              </div>
            </div>
            <div className='shrink-0 text-right'>
              <div className='font-medium tabular-nums'>
                {formatCurrencyFromUSD(record.amount)}
              </div>
              <div className='text-muted-foreground text-xs'>
                {t(record.status || 'pending')}
              </div>
            </div>
          </div>
        ))}
      </div>
    )
  })()

  const balanceContent = (() => {
    if (userQuery.isLoading) {
      return <Skeleton className='h-12 w-40' />
    }
    if (userQuery.isError) {
      return (
        <div className='flex items-center gap-3'>
          <span className='text-muted-foreground text-sm'>
            {t('Failed to load')}
          </span>
          <Button
            variant='outline'
            size='sm'
            onClick={() => userQuery.refetch()}
          >
            {t('Retry')}
          </Button>
        </div>
      )
    }
    return (
      <div className='font-mono text-4xl font-bold tracking-tight tabular-nums'>
        {formatQuota(userQuery.data?.quota ?? 0)}
      </div>
    )
  })()

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Balance Recharge')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='mx-auto max-w-5xl space-y-4'>
          <Alert className='border-amber-500/25 bg-amber-500/[0.06]'>
            <ShieldCheck className='text-amber-600 dark:text-amber-300' />
            <AlertTitle>{t('Default pool balance')}</AlertTitle>
            <AlertDescription>
              {t(
                'This balance is charged only when an API key routes to the Default shared pool. Your private pool never consumes it.'
              )}
            </AlertDescription>
          </Alert>

          <div className='grid gap-4 lg:grid-cols-[1.05fr_1.4fr]'>
            <Card data-card-hover='false'>
              <CardHeader>
                <CardTitle className='flex items-center gap-2'>
                  <BadgeDollarSign className='size-5 text-emerald-600' />
                  {t('Default Pool Balance')}
                </CardTitle>
                <CardDescription>
                  {t('Available credit for paid shared-pool requests')}
                </CardDescription>
              </CardHeader>
              <CardContent>
                {balanceContent}
                <div className='text-muted-foreground mt-5 space-y-2 text-sm'>
                  <div className='flex items-center gap-2'>
                    <KeyRound className='size-4' />
                    {t(
                      'Choose Default as the API key group to use this balance.'
                    )}
                  </div>
                  <div className='flex items-center gap-2'>
                    <Clock3 className='size-4' />
                    {t('Failed requests are not intended to consume balance.')}
                  </div>
                </div>
              </CardContent>
            </Card>

            <Card data-card-hover='false'>
              <CardHeader>
                <CardTitle>{t('Recharge privileges')}</CardTitle>
                <CardDescription>
                  {t(
                    'Privileges are based on your current USD-equivalent balance.'
                  )}
                </CardDescription>
              </CardHeader>
              <CardContent className='grid gap-3 sm:grid-cols-3'>
                {rewardMilestones.map((milestone) => (
                  <div
                    key={milestone.amount}
                    className='bg-muted/30 rounded-xl border p-3'
                  >
                    <milestone.icon className='mb-3 size-5 text-amber-500' />
                    <div className='text-lg font-semibold'>
                      {milestone.amount}
                    </div>
                    <div className='text-muted-foreground mt-1 text-xs'>
                      {t(milestone.label)}
                    </div>
                  </div>
                ))}
              </CardContent>
            </Card>
          </div>

          <div className='grid gap-4 lg:grid-cols-2'>
            <Card data-card-hover='false'>
              <CardHeader>
                <CardTitle>{t('Recharge balance')}</CardTitle>
                <CardDescription>
                  {paymentConfigured(info)
                    ? t('Configured payment channels are listed below.')
                    : t(
                        'Only redemption-code recharge is available. Current conversion: ¥1 = ${{amount}} Default balance.',
                        { amount: DEFAULT_POOL_USD_CREDIT_PER_CNY }
                      )}
                </CardDescription>
              </CardHeader>
              <CardContent className='space-y-4'>
                {providers.length > 0 && (
                  <div className='flex flex-wrap gap-2'>
                    {providers.map((provider) => (
                      <StatusBadge
                        key={provider}
                        label={provider}
                        variant='info'
                        copyable={false}
                      />
                    ))}
                  </div>
                )}

                {info?.topup_link && (
                  <Button
                    variant='outline'
                    onClick={() =>
                      window.open(
                        info.topup_link,
                        '_blank',
                        'noopener,noreferrer'
                      )
                    }
                  >
                    {t('Open recharge channel')}
                  </Button>
                )}

                <div className='border-t pt-4'>
                  <div className='mb-2 flex items-center gap-2 text-sm font-medium'>
                    <Gift className='size-4' />
                    {t('Redemption Code')}
                  </div>
                  <p className='text-muted-foreground mb-3 text-xs leading-5'>
                    {t('Contact an administrator for a code. Current rate:')}{' '}
                    <span className='text-foreground font-medium'>
                      {DEFAULT_POOL_REDEMPTION_RATE_LABEL}
                    </span>
                  </p>
                  <div className='flex gap-2'>
                    <Input
                      value={code}
                      onChange={(event) => setCode(event.target.value)}
                      placeholder={t('Enter redemption code')}
                      disabled={info?.enable_redemption === false}
                    />
                    <Button
                      onClick={() => redeemMutation.mutate()}
                      disabled={
                        !code.trim() ||
                        redeemMutation.isPending ||
                        info?.enable_redemption === false
                      }
                    >
                      {redeemMutation.isPending
                        ? t('Redeeming...')
                        : t('Redeem')}
                    </Button>
                  </div>
                </div>
              </CardContent>
            </Card>

            <Card data-card-hover='false'>
              <CardHeader>
                <CardTitle className='flex items-center gap-2'>
                  <History className='size-5' />
                  {t('Recharge History')}
                </CardTitle>
                <CardDescription>
                  {t('The most recent recharge orders for your account')}
                </CardDescription>
              </CardHeader>
              <CardContent>{historyContent}</CardContent>
            </Card>
          </div>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
