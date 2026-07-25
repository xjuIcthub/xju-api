/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Calculator, Save, ShieldCheck, SlidersHorizontal } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
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
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'

import { getDefaultPoolPricing, updateDefaultPoolPricing } from './api'

export function DefaultPricing() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [value, setValue] = useState('')
  const pricingQuery = useQuery({
    queryKey: ['pool', 'default-pricing'],
    queryFn: getDefaultPoolPricing,
  })

  useEffect(() => {
    if (pricingQuery.data) {
      setValue(String(pricingQuery.data.multiplier))
    }
  }, [pricingQuery.data])

  const parsedMultiplier = Number(value)
  const valid =
    Number.isFinite(parsedMultiplier) &&
    parsedMultiplier >= 0.1 &&
    parsedMultiplier <= 100
  const preview = useMemo(
    () => (valid ? parsedMultiplier.toFixed(2) : '—'),
    [parsedMultiplier, valid]
  )

  const updateMutation = useMutation({
    mutationFn: () => updateDefaultPoolPricing(parsedMultiplier),
    onSuccess: (data) => {
      queryClient.setQueryData(['pool', 'default-pricing'], data)
      setValue(String(data.multiplier))
      toast.success(t('Default pool pricing updated'))
    },
    onError: (error: Error) => toast.error(error.message),
  })

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Default Pool Pricing')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='mx-auto w-full max-w-3xl space-y-4'>
          <Alert className='border-sky-500/25 bg-sky-500/[0.05]'>
            <ShieldCheck className='text-sky-600 dark:text-sky-300' />
            <AlertTitle>{t('Default shared pool only')}</AlertTitle>
            <AlertDescription>
              {t(
                'This multiplier changes billing for the Default shared pool. User-owned private pools remain quota-free and are not affected.'
              )}
            </AlertDescription>
          </Alert>

          <Card data-card-hover='false'>
            <CardHeader>
              <CardTitle className='flex items-center gap-2'>
                <SlidersHorizontal className='size-5' />
                {t('Usage price multiplier')}
              </CardTitle>
              <CardDescription>
                {t(
                  'The model price is multiplied by this value before Default-pool quota is deducted.'
                )}
              </CardDescription>
            </CardHeader>
            <CardContent>
              {pricingQuery.isPending && (
                <div className='space-y-5'>
                  <Skeleton className='h-10 w-full' />
                  <Skeleton className='h-20 w-full' />
                </div>
              )}
              {pricingQuery.isError && (
                <div className='flex min-h-32 flex-col items-center justify-center gap-3 text-center'>
                  <p className='text-destructive text-sm font-medium'>
                    {t('Failed to load')}
                  </p>
                  <Button
                    variant='outline'
                    onClick={() => void pricingQuery.refetch()}
                    disabled={pricingQuery.isFetching}
                  >
                    {t('Retry')}
                  </Button>
                </div>
              )}
              {pricingQuery.isSuccess && (
                <div className='space-y-5'>
                  <div className='space-y-2'>
                    <Label htmlFor='default-pool-multiplier'>
                      {t('Multiplier')}
                    </Label>
                    <Input
                      id='default-pool-multiplier'
                      type='number'
                      min='0.1'
                      max='100'
                      step='0.1'
                      value={value}
                      onChange={(event) => setValue(event.target.value)}
                      aria-invalid={!valid}
                    />
                    <p className='text-muted-foreground text-xs'>
                      {t('Allowed range: 0.1 to 100')}
                    </p>
                  </div>

                  <div className='bg-muted/30 flex items-center gap-3 rounded-xl border p-4'>
                    <div className='bg-background rounded-lg border p-2'>
                      <Calculator className='size-5' />
                    </div>
                    <div>
                      <div className='text-sm font-medium'>
                        {t('Pricing preview')}
                      </div>
                      <div className='text-muted-foreground mt-0.5 text-xs'>
                        {t(
                          '$1.00 of base model usage deducts ${{amount}} from balance',
                          {
                            amount: preview,
                          }
                        )}
                      </div>
                    </div>
                  </div>

                  <div className='flex justify-end'>
                    <Button
                      onClick={() => updateMutation.mutate()}
                      disabled={!valid || updateMutation.isPending}
                    >
                      <Save data-icon='inline-start' />
                      {updateMutation.isPending ? t('Saving...') : t('Save')}
                    </Button>
                  </div>
                </div>
              )}
            </CardContent>
          </Card>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
