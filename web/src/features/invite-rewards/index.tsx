/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useQuery } from '@tanstack/react-query'
import {
  ArrowRight,
  CheckCircle2,
  Copy,
  Gift,
  PartyPopper,
  Share2,
  Sparkles,
  Trophy,
  UserPlus,
  UsersRound,
} from 'lucide-react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import { StatusBadge } from '@/components/status-badge'
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
import { copyToClipboard } from '@/lib/copy-to-clipboard'
import { useAuthStore, type AuthUser } from '@/stores/auth-store'

import { getPersonalInviteCode } from './api'

const rewardMilestones = [
  { target: 3, reward: 'Extra $10' },
  { target: 5, reward: 'Extra $20' },
  { target: 10, reward: 'Extra $50' },
]

const floatingDecorations = [
  { left: '5%', top: '16%', size: 7, delay: 0.1, duration: 4.8 },
  { left: '13%', top: '76%', size: 5, delay: 0.8, duration: 5.6 },
  { left: '24%', top: '8%', size: 4, delay: 1.4, duration: 4.2 },
  { left: '35%', top: '83%', size: 8, delay: 0.4, duration: 6.2 },
  { left: '49%', top: '12%', size: 5, delay: 1.8, duration: 5.1 },
  { left: '58%', top: '72%', size: 6, delay: 0.2, duration: 4.5 },
  { left: '69%', top: '7%', size: 8, delay: 1.1, duration: 6.4 },
  { left: '78%', top: '84%', size: 4, delay: 0.6, duration: 4.9 },
  { left: '89%', top: '18%', size: 6, delay: 1.6, duration: 5.7 },
  { left: '95%', top: '62%', size: 5, delay: 0.3, duration: 4.4 },
]

const orbitRewards = [
  { label: '+$5', className: 'top-[3%] left-1/2 -translate-x-1/2' },
  { label: '+$10', className: 'top-[45%] -right-3' },
  { label: '+$50', className: 'bottom-[3%] left-1/2 -translate-x-1/2' },
  { label: '$', className: 'top-[45%] -left-3' },
]

function getInviteProgress(inviteCount: number) {
  const next = rewardMilestones.find(
    (milestone) => inviteCount < milestone.target
  )
  if (!next) {
    return { percent: 100, remaining: 0, nextTarget: 10 }
  }

  const previousTarget =
    [...rewardMilestones]
      .reverse()
      .find((milestone) => milestone.target <= inviteCount)?.target ?? 0
  const range = Math.max(1, next.target - previousTarget)
  const completed = Math.max(0, inviteCount - previousTarget)

  return {
    percent: Math.min(100, (completed / range) * 100),
    remaining: next.target - inviteCount,
    nextTarget: next.target,
  }
}

export function InviteRewards() {
  const { t } = useTranslation()
  const shouldReduceMotion = useReducedMotion()
  const [hasJoined, setHasJoined] = useState(false)
  const invitePanelRef = useRef<HTMLDivElement>(null)
  const authUser = useAuthStore((state) => state.auth.user)

  const userQuery = useQuery({
    queryKey: ['invite-rewards', 'self'],
    queryFn: async () => {
      const response = await getSelf()
      if (!response?.success || !response.data) {
        throw new Error(
          response?.message || t('Failed to load invitation data')
        )
      }
      return response.data as AuthUser
    },
    initialData: authUser ?? undefined,
  })
  const referralQuery = useQuery({
    queryKey: ['invite-rewards', 'invite-code'],
    queryFn: getPersonalInviteCode,
  })

  const referralCode =
    userQuery.data?.aff_code?.trim() || referralQuery.data?.trim() || ''
  const referralLink = useMemo(() => {
    if (!referralCode || typeof window === 'undefined') return ''
    return `${window.location.origin}/register?aff=${encodeURIComponent(referralCode)}`
  }, [referralCode])
  const inviteCount = userQuery.data?.aff_count ?? 0
  const progress = getInviteProgress(inviteCount)

  const copyReferralLink = async () => {
    if (!referralLink) return
    const copied = await copyToClipboard(referralLink)
    if (copied) {
      toast.success(t('Invitation link copied'))
    } else {
      toast.error(t('Failed to copy invitation link'))
    }
  }

  const joinActivity = () => {
    if (!hasJoined) {
      setHasJoined(true)
      toast.success(t('Your invitation entry is ready'))
    }
    window.requestAnimationFrame(() => {
      invitePanelRef.current?.scrollIntoView({
        behavior: shouldReduceMotion ? 'auto' : 'smooth',
        block: 'center',
      })
    })
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Invitation Gifts')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='mx-auto max-w-6xl space-y-5 pb-4'>
          <section
            className='relative isolate overflow-hidden rounded-[28px] border border-[#ffd98a]/45 text-white shadow-[0_24px_80px_rgba(128,7,22,0.2)]'
            style={{
              background:
                'radial-gradient(circle at 16% 12%, rgba(255, 224, 153, 0.34), transparent 27%), radial-gradient(circle at 84% 20%, rgba(255, 183, 77, 0.28), transparent 25%), linear-gradient(135deg, #760515 0%, #b80f21 38%, #d72b27 68%, #ef5a34 100%)',
            }}
          >
            <div
              aria-hidden='true'
              className='pointer-events-none absolute inset-0 [background-image:linear-gradient(rgba(255,255,255,0.08)_1px,transparent_1px),linear-gradient(90deg,rgba(255,255,255,0.08)_1px,transparent_1px)] [mask-image:linear-gradient(to_bottom,black,transparent_82%)] [background-size:42px_42px] opacity-25'
            />
            <div
              aria-hidden='true'
              className='pointer-events-none absolute -top-28 -left-20 size-72 rounded-full border border-[#ffe2a6]/25'
            />
            <div
              aria-hidden='true'
              className='pointer-events-none absolute -right-24 -bottom-36 size-96 rounded-full border border-[#ffe2a6]/20'
            />

            <div
              aria-hidden='true'
              className='pointer-events-none absolute inset-0'
            >
              {floatingDecorations.map((decoration, index) => (
                <motion.span
                  key={`${decoration.left}-${decoration.top}`}
                  className='absolute rounded-[2px] bg-[#ffe6a8] shadow-[0_0_14px_rgba(255,221,139,0.72)]'
                  style={{
                    left: decoration.left,
                    top: decoration.top,
                    width: decoration.size,
                    height: decoration.size * 1.9,
                  }}
                  animate={
                    shouldReduceMotion
                      ? undefined
                      : {
                          y: [0, -16 - (index % 3) * 4, 0],
                          rotate: [index * 8, index * 8 + 160, index * 8 + 360],
                          opacity: [0.35, 1, 0.35],
                        }
                  }
                  transition={{
                    duration: decoration.duration,
                    delay: decoration.delay,
                    repeat: Infinity,
                    ease: 'easeInOut',
                  }}
                />
              ))}
            </div>

            <div className='relative grid min-h-[430px] items-center gap-8 px-5 py-8 sm:px-9 sm:py-10 lg:grid-cols-[1.12fr_0.88fr] lg:px-12 lg:py-12'>
              <div className='relative z-10 max-w-2xl'>
                <div className='mb-5 inline-flex items-center gap-2 rounded-full border border-[#ffe5af]/45 bg-[#690411]/40 px-3.5 py-1.5 text-xs font-semibold tracking-[0.18em] text-[#ffecc3] uppercase backdrop-blur-sm'>
                  <PartyPopper className='size-4' />
                  {t('Invitation celebration · Event preview')}
                </div>

                <p className='mb-2 text-sm font-medium text-[#ffe8b7]/90 sm:text-base'>
                  {t('Invite friends, unlock good fortune')}
                </p>
                <h1 className='font-serif text-[2.55rem] leading-[1.04] font-bold tracking-[-0.035em] text-balance sm:text-6xl lg:text-[4.25rem]'>
                  {t('Join the invitation event')}
                  <span className='mt-2 block text-[#ffe29a] drop-shadow-[0_5px_24px_rgba(93,2,11,0.34)]'>
                    {t('Win up to $10,000 in credit')}
                  </span>
                </h1>
                <p className='mt-5 max-w-xl text-sm leading-7 text-[#fff3dc]/85 sm:text-base'>
                  {t(
                    'Join now to unlock your personal invitation link. Standard invitation rewards continue to be credited automatically after successful registration.'
                  )}
                </p>

                <div className='mt-7 flex flex-col items-start gap-3 sm:flex-row sm:items-center'>
                  <motion.div
                    whileHover={
                      shouldReduceMotion ? undefined : { scale: 1.025 }
                    }
                    whileTap={shouldReduceMotion ? undefined : { scale: 0.985 }}
                  >
                    <Button
                      size='lg'
                      onClick={joinActivity}
                      className='group relative h-14 overflow-hidden rounded-full border border-[#fff1c9] bg-[linear-gradient(135deg,#fff3c6_0%,#ffc85b_45%,#fff0b2_100%)] px-7 text-base font-bold text-[#8a1018] shadow-[0_12px_34px_rgba(78,2,10,0.3)] hover:bg-[linear-gradient(135deg,#fff8dd_0%,#ffd576_45%,#fff4c6_100%)]'
                    >
                      {!shouldReduceMotion && (
                        <motion.span
                          aria-hidden='true'
                          className='absolute inset-y-0 -left-16 w-10 skew-x-[-18deg] bg-white/65 blur-[1px]'
                          animate={{ x: [0, 340] }}
                          transition={{
                            duration: 2.7,
                            repeat: Infinity,
                            repeatDelay: 1.1,
                            ease: 'easeInOut',
                          }}
                        />
                      )}
                      {hasJoined ? (
                        <CheckCircle2 data-icon='inline-start' />
                      ) : (
                        <Gift data-icon='inline-start' />
                      )}
                      <span className='relative'>
                        {hasJoined
                          ? t('View my invitation entry')
                          : t('Join the event')}
                      </span>
                      <ArrowRight className='relative size-4 transition-transform group-hover:translate-x-0.5' />
                    </Button>
                  </motion.div>
                  <span className='text-xs leading-5 text-[#ffe7b0]/78'>
                    {t(
                      'Grand-prize assistance and settlement rules will be announced before the official launch.'
                    )}
                  </span>
                </div>

                <div className='mt-8 grid max-w-2xl grid-cols-3 gap-2.5'>
                  {[
                    ['$10,000', 'Maximum credit prize'],
                    ['$5 + $5', 'Reward for both users'],
                    ['$80', 'Milestone bonus total'],
                  ].map(([value, label]) => (
                    <div
                      key={label}
                      className='rounded-2xl border border-white/15 bg-[#6d0713]/28 px-3 py-3 backdrop-blur-sm sm:px-4'
                    >
                      <div className='font-serif text-lg font-bold text-[#ffe5a7] sm:text-2xl'>
                        {value}
                      </div>
                      <div className='mt-0.5 text-[10px] leading-4 text-white/68 sm:text-xs'>
                        {t(label)}
                      </div>
                    </div>
                  ))}
                </div>
              </div>

              <div className='relative mx-auto hidden aspect-square w-full max-w-[390px] lg:block'>
                <motion.div
                  aria-hidden='true'
                  className='absolute inset-[8%] rounded-full border border-dashed border-[#ffe3a0]/40'
                  animate={shouldReduceMotion ? undefined : { rotate: 360 }}
                  transition={{
                    duration: 24,
                    repeat: Infinity,
                    ease: 'linear',
                  }}
                />
                <motion.div
                  aria-hidden='true'
                  className='absolute inset-[20%] rounded-full border border-[#ffe3a0]/24'
                  animate={shouldReduceMotion ? undefined : { rotate: -360 }}
                  transition={{
                    duration: 18,
                    repeat: Infinity,
                    ease: 'linear',
                  }}
                />

                {orbitRewards.map((reward, index) => (
                  <motion.div
                    key={reward.label}
                    className={`absolute z-20 flex h-12 min-w-12 items-center justify-center rounded-full border border-[#fff1c8]/70 bg-[linear-gradient(145deg,#fff5cf,#ffc755)] px-2 font-serif text-sm font-bold text-[#9e1820] shadow-[0_9px_28px_rgba(79,3,12,0.28)] ${reward.className}`}
                    animate={
                      shouldReduceMotion
                        ? undefined
                        : { y: [0, -9 - index * 2, 0], rotate: [0, 5, -4, 0] }
                    }
                    transition={{
                      duration: 3.4 + index * 0.45,
                      delay: index * 0.28,
                      repeat: Infinity,
                      ease: 'easeInOut',
                    }}
                  >
                    {reward.label}
                  </motion.div>
                ))}

                <motion.div
                  className='absolute inset-[27%] z-10 flex items-center justify-center rounded-[36px] border border-[#fff0c3]/65 bg-[linear-gradient(145deg,#ffdf86_0%,#ffac35_48%,#ff7b29_100%)] shadow-[0_28px_80px_rgba(82,2,10,0.34),inset_0_1px_0_rgba(255,255,255,0.6)]'
                  animate={
                    shouldReduceMotion
                      ? undefined
                      : { y: [0, -10, 0], rotate: [-1.5, 1.5, -1.5] }
                  }
                  transition={{
                    duration: 4.2,
                    repeat: Infinity,
                    ease: 'easeInOut',
                  }}
                >
                  <motion.div
                    animate={
                      shouldReduceMotion ? undefined : { scale: [1, 1.08, 1] }
                    }
                    transition={{
                      duration: 2.2,
                      repeat: Infinity,
                      ease: 'easeInOut',
                    }}
                  >
                    <Gift
                      className='size-24 text-[#9b1420] drop-shadow-[0_7px_9px_rgba(117,9,20,0.2)]'
                      strokeWidth={1.7}
                    />
                  </motion.div>
                </motion.div>

                <motion.div
                  aria-hidden='true'
                  className='absolute top-[18%] right-[22%] text-[#fff0b9]'
                  animate={
                    shouldReduceMotion
                      ? undefined
                      : { scale: [0.8, 1.3, 0.8], rotate: [0, 30, 0] }
                  }
                  transition={{
                    duration: 2.8,
                    repeat: Infinity,
                    ease: 'easeInOut',
                  }}
                >
                  <Sparkles className='size-9' />
                </motion.div>
                <motion.div
                  aria-hidden='true'
                  className='absolute bottom-[21%] left-[20%] text-[#ffe39b]'
                  animate={
                    shouldReduceMotion
                      ? undefined
                      : { scale: [1.1, 0.7, 1.1], rotate: [0, -25, 0] }
                  }
                  transition={{
                    duration: 3.1,
                    repeat: Infinity,
                    ease: 'easeInOut',
                  }}
                >
                  <Sparkles className='size-6' />
                </motion.div>
              </div>
            </div>
          </section>

          <AnimatePresence initial={false}>
            {hasJoined && (
              <motion.div
                initial={{ opacity: 0, y: -12, scale: 0.985 }}
                animate={{ opacity: 1, y: 0, scale: 1 }}
                exit={{ opacity: 0, y: -8 }}
                transition={{ duration: shouldReduceMotion ? 0 : 0.3 }}
                role='status'
                aria-live='polite'
                className='flex flex-col gap-3 rounded-2xl border border-red-200 bg-[linear-gradient(100deg,#fff8ed,#fff1e7)] px-4 py-3.5 text-red-950 sm:flex-row sm:items-center sm:justify-between dark:border-red-900/60 dark:bg-[linear-gradient(100deg,rgba(93,16,25,0.42),rgba(110,36,20,0.3))] dark:text-red-50'
              >
                <div className='flex items-start gap-3'>
                  <div className='mt-0.5 rounded-full bg-red-600 p-1.5 text-white'>
                    <CheckCircle2 className='size-4' />
                  </div>
                  <div>
                    <div className='font-semibold'>
                      {t('Your invitation entry is ready')}
                    </div>
                    <div className='mt-0.5 text-xs text-red-800/70 dark:text-red-100/65'>
                      {t(
                        'Share your personal link below to start inviting friends.'
                      )}
                    </div>
                  </div>
                </div>
                <Button
                  size='sm'
                  variant='outline'
                  onClick={copyReferralLink}
                  disabled={!referralLink}
                  className='border-red-300 bg-white/65 text-red-800 hover:bg-white hover:text-red-900 dark:border-red-800 dark:bg-red-950/40 dark:text-red-100'
                >
                  <Copy data-icon='inline-start' />
                  {t('Copy invitation link')}
                </Button>
              </motion.div>
            )}
          </AnimatePresence>

          <div
            ref={invitePanelRef}
            className='grid gap-5 lg:grid-cols-[1.15fr_0.85fr]'
          >
            <Card data-card-hover='false' className='overflow-hidden'>
              <CardHeader className='border-b bg-[linear-gradient(135deg,rgba(220,38,38,0.07),rgba(245,158,11,0.08))]'>
                <CardTitle className='flex items-center gap-2'>
                  <UserPlus className='size-5 text-red-600' />
                  {t('My invitation entry')}
                </CardTitle>
                <CardDescription>
                  {t(
                    'Share your personal invitation link. Rewards are credited directly to the Default pool balance after registration succeeds.'
                  )}
                </CardDescription>
              </CardHeader>
              <CardContent className='space-y-5 pt-5'>
                <div className='grid gap-2 sm:grid-cols-[1fr_auto]'>
                  {referralQuery.isLoading && !referralLink ? (
                    <Skeleton className='h-9 w-full' />
                  ) : (
                    <Input
                      readOnly
                      value={referralLink}
                      placeholder={t('Invitation link is being prepared')}
                      className='font-mono text-xs'
                    />
                  )}
                  <Button
                    variant='outline'
                    onClick={copyReferralLink}
                    disabled={!referralLink}
                  >
                    <Copy data-icon='inline-start' />
                    {t('Copy link')}
                  </Button>
                </div>

                <div className='flex flex-wrap items-center gap-2 text-xs'>
                  <StatusBadge
                    label={`${t('Invitation code')}: ${referralCode || '—'}`}
                    variant='info'
                    copyText={referralCode}
                    copyable={Boolean(referralCode)}
                  />
                  <StatusBadge
                    label={`${t('Successful invites')}: ${inviteCount}`}
                    variant='neutral'
                    copyable={false}
                  />
                </div>

                <div className='bg-muted/25 rounded-2xl border p-4'>
                  <div className='flex items-center justify-between gap-4'>
                    <div>
                      <div className='text-sm font-semibold'>
                        {t('Milestone progress')}
                      </div>
                      <div className='text-muted-foreground mt-1 text-xs'>
                        {progress.remaining > 0
                          ? t(
                              '{{count}} more successful invite(s) to reach the next reward',
                              {
                                count: progress.remaining,
                              }
                            )
                          : t('All current milestone rewards unlocked')}
                      </div>
                    </div>
                    <div className='font-serif text-2xl font-bold text-red-600 tabular-nums'>
                      {inviteCount}/{progress.nextTarget}
                    </div>
                  </div>
                  <div className='mt-4 h-2.5 overflow-hidden rounded-full bg-red-100 dark:bg-red-950/60'>
                    <motion.div
                      className='h-full rounded-full bg-[linear-gradient(90deg,#dc2626,#f59e0b,#facc15)]'
                      initial={{ width: 0 }}
                      animate={{ width: `${progress.percent}%` }}
                      transition={{
                        duration: shouldReduceMotion ? 0 : 0.65,
                        ease: 'easeOut',
                      }}
                    />
                  </div>
                </div>
              </CardContent>
            </Card>

            <Card data-card-hover='false'>
              <CardHeader>
                <CardTitle className='flex items-center gap-2'>
                  <Trophy className='size-5 text-amber-500' />
                  {t('Current invitation rewards')}
                </CardTitle>
                <CardDescription>
                  {t(
                    'These standard rewards remain active during the event preview.'
                  )}
                </CardDescription>
              </CardHeader>
              <CardContent className='space-y-3'>
                <div className='flex items-center justify-between gap-3 rounded-xl border border-amber-200/75 bg-amber-50/65 p-3 dark:border-amber-900/50 dark:bg-amber-950/20'>
                  <div className='flex items-center gap-3'>
                    <div className='rounded-full bg-amber-500/15 p-2 text-amber-700 dark:text-amber-300'>
                      <UsersRound className='size-4' />
                    </div>
                    <div>
                      <div className='text-sm font-medium'>
                        {t('Every successful invite')}
                      </div>
                      <div className='text-muted-foreground text-xs'>
                        {t('Inviter and friend')}
                      </div>
                    </div>
                  </div>
                  <span className='font-serif font-bold text-amber-700 dark:text-amber-300'>
                    {t('Both users receive $5')}
                  </span>
                </div>

                {rewardMilestones.map((milestone) => {
                  const unlocked = inviteCount >= milestone.target
                  return (
                    <div
                      key={milestone.target}
                      className='flex items-center justify-between gap-3 rounded-xl border p-3'
                    >
                      <div className='flex items-center gap-3'>
                        <div
                          className={`flex size-9 items-center justify-center rounded-full text-xs font-bold ${
                            unlocked
                              ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-300'
                              : 'bg-muted text-muted-foreground'
                          }`}
                        >
                          {unlocked ? (
                            <CheckCircle2 className='size-4' />
                          ) : (
                            milestone.target
                          )}
                        </div>
                        <span className='text-sm'>
                          {t('{{count}} successful invites', {
                            count: milestone.target,
                          })}
                        </span>
                      </div>
                      <span className='font-serif font-bold'>
                        {t(milestone.reward)}
                      </span>
                    </div>
                  )
                })}
              </CardContent>
            </Card>
          </div>

          <div className='grid gap-3 md:grid-cols-3'>
            {[
              [
                Gift,
                'Join the event',
                'Unlock your personal invitation entry.',
              ],
              [
                Share2,
                'Share your link',
                'Send the link to friends who want to join.',
              ],
              [
                Sparkles,
                'Receive rewards',
                'Rewards arrive after a successful registration.',
              ],
            ].map(([Icon, title, description], index) => {
              const StepIcon = Icon as typeof Gift
              return (
                <div
                  key={title as string}
                  className='bg-card rounded-2xl border p-4'
                >
                  <div className='mb-4 flex items-center justify-between'>
                    <div className='rounded-xl bg-red-500/10 p-2 text-red-600 dark:text-red-300'>
                      <StepIcon className='size-5' />
                    </div>
                    <span className='text-muted-foreground font-mono text-xs'>
                      0{index + 1}
                    </span>
                  </div>
                  <div className='font-semibold'>{t(title as string)}</div>
                  <div className='text-muted-foreground mt-1 text-sm leading-6'>
                    {t(description as string)}
                  </div>
                </div>
              )
            })}
          </div>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
