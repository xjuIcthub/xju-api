/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import {
  ArrowLeft,
  CheckCircle2,
  Copy,
  Gift,
  History,
  Info,
  PartyPopper,
  RefreshCcw,
  RotateCw,
  Share2,
  ShieldCheck,
  Sparkles,
  Target,
  Ticket,
  Trophy,
  UserPlus,
  UsersRound,
} from 'lucide-react'
import { motion, useInView, useReducedMotion } from 'motion/react'
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
import { ChallengeRevealDialog } from '@/features/invite-activity/components/challenge-reveal-dialog'
import {
  FestiveBackground,
  SpinCelebration,
  type SpinCelebrationState,
} from '@/features/invite-activity/components/festive-effects'
import { getPersonalInviteCode } from '@/features/invite-rewards/api'
import { getSelf } from '@/lib/api'
import { copyToClipboard } from '@/lib/copy-to-clipboard'
import { cn } from '@/lib/utils'
import { useAuthStore, type AuthUser } from '@/stores/auth-store'

const wheelValues = [68, 21, 8, 5, 3, 2, 1, 10] as const
const demoOutcomeOrder = [0, 1, 2, 5] as const

const stageDefinitions = [
  {
    id: 'points',
    goal: 'Reach 99 points',
    unit: 'points',
    target: 99,
  },
  {
    id: 'fragments',
    goal: 'Collect lucky fragments',
    unit: 'fragments',
    target: 99,
  },
  {
    id: 'stardust',
    goal: 'Collect stardust',
    unit: 'stardust',
    target: 99,
  },
] as const

type StageIndex = 0 | 1 | 2 | 3
type ChallengeStageIndex = 1 | 2

type SpinHistoryItem = {
  id: number
  stageIndex: 0 | 1 | 2
  value: number
  time: string
}

type DemoState = {
  availableSpins: number
  effectiveInvites: number
  history: SpinHistoryItem[]
  progress: number
  spinSequence: number
  stageIndex: StageIndex
}

const initialDemoState: DemoState = {
  availableSpins: 0,
  effectiveInvites: 0,
  history: [],
  progress: 0,
  spinSequence: 0,
  stageIndex: 0,
}

function formatOverallProgress(stageIndex: StageIndex, progress: number) {
  if (stageIndex === 3) {
    return '100.0000'
  }
  if (stageIndex === 0) {
    return String(progress)
  }
  if (stageIndex === 1) {
    return `99.${String(progress).padStart(2, '0')}`
  }
  return `99.99${String(progress).padStart(2, '0')}`
}

export function InviteActivity() {
  const { t } = useTranslation()
  const shouldReduceMotion = useReducedMotion()
  const reducedMotion = Boolean(shouldReduceMotion)
  const authUser = useAuthStore((state) => state.auth.user)
  const [demoState, setDemoState] = useState(initialDemoState)
  const [isSpinning, setIsSpinning] = useState(false)
  const [wheelRotation, setWheelRotation] = useState(0)
  const [celebration, setCelebration] = useState<SpinCelebrationState | null>(
    null
  )
  const [pendingChallengeIndex, setPendingChallengeIndex] =
    useState<ChallengeStageIndex | null>(null)
  const [challengeDialogOpen, setChallengeDialogOpen] = useState(false)
  const heroRef = useRef<HTMLElement>(null)
  const celebrationSequenceRef = useRef(0)
  const pendingSpinRef = useRef<{
    progress: number
    stageIndex: 0 | 1 | 2
    value: number
  } | null>(null)
  const spinningRef = useRef(false)
  const heroIsVisible = useInView(heroRef, { amount: 0.12 })

  const userQuery = useQuery({
    queryKey: ['invite-activity', 'self'],
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
    queryKey: ['invite-activity', 'invite-code'],
    queryFn: getPersonalInviteCode,
  })

  const referralCode =
    userQuery.data?.aff_code?.trim() || referralQuery.data?.trim() || ''
  const referralLink = useMemo(() => {
    if (!referralCode || typeof window === 'undefined') {
      return ''
    }
    return `${window.location.origin}/register?aff=${encodeURIComponent(referralCode)}`
  }, [referralCode])

  const currentStage =
    demoState.stageIndex === 3 ? null : stageDefinitions[demoState.stageIndex]
  const overallProgress = formatOverallProgress(
    demoState.stageIndex,
    demoState.progress
  )
  const currentProgressPercent = currentStage
    ? Math.min(100, (demoState.progress / currentStage.target) * 100)
    : 100
  const overallProgressPercent = Math.min(
    100,
    Number.parseFloat(overallProgress)
  )
  const pendingChallenge =
    pendingChallengeIndex === null
      ? null
      : stageDefinitions[pendingChallengeIndex]
  let spinButtonLabel = t('Invite a friend to unlock a spin')
  if (demoState.availableSpins > 0) {
    spinButtonLabel = t('Spin now')
  }
  if (demoState.stageIndex === 3) {
    spinButtonLabel = t('Completed')
  }
  if (isSpinning) {
    spinButtonLabel = t('Spinning...')
  }

  const copyReferralLink = async () => {
    if (!referralLink) {
      return
    }
    const copied = await copyToClipboard(referralLink)
    if (copied) {
      toast.success(t('Invitation link copied'))
    } else {
      toast.error(t('Failed to copy invitation link'))
    }
  }

  const shareReferralLink = async () => {
    if (!referralLink) {
      return
    }
    if (typeof navigator !== 'undefined' && navigator.share) {
      try {
        await navigator.share({
          title: t('Invitation Wheel'),
          text: t('Join me and help unlock the invitation reward.'),
          url: referralLink,
        })
        return
      } catch (error) {
        if (error instanceof DOMException && error.name === 'AbortError') {
          return
        }
      }
    }
    await copyReferralLink()
  }

  const simulateEffectiveInvite = () => {
    if (
      spinningRef.current ||
      pendingChallengeIndex !== null ||
      demoState.stageIndex === 3
    ) {
      return
    }
    setDemoState((previous) => ({
      ...previous,
      availableSpins: previous.availableSpins + 1,
      effectiveInvites: previous.effectiveInvites + 1,
    }))
    toast.success(t('Demo friend added. One spin unlocked.'))
  }

  const spinWheel = () => {
    if (
      spinningRef.current ||
      pendingChallengeIndex !== null ||
      demoState.availableSpins < 1 ||
      demoState.stageIndex === 3
    ) {
      return
    }

    setCelebration(null)
    const outcomeIndex =
      demoOutcomeOrder[demoState.spinSequence % demoOutcomeOrder.length]
    const value = wheelValues[outcomeIndex]
    pendingSpinRef.current = {
      progress: demoState.progress,
      stageIndex: demoState.stageIndex,
      value,
    }
    spinningRef.current = true
    setIsSpinning(true)
    setDemoState((previous) => ({
      ...previous,
      availableSpins: previous.availableSpins - 1,
      spinSequence: previous.spinSequence + 1,
    }))

    const normalizedRotation = ((wheelRotation % 360) + 360) % 360
    const desiredRotation = (360 - outcomeIndex * 45) % 360
    let alignmentDelta = desiredRotation - normalizedRotation
    if (alignmentDelta <= 0) {
      alignmentDelta += 360
    }
    setWheelRotation(
      (previous) =>
        previous +
        (shouldReduceMotion ? alignmentDelta : 5 * 360 + alignmentDelta)
    )
  }

  const settleSpin = () => {
    const result = pendingSpinRef.current
    if (!result) {
      return
    }
    pendingSpinRef.current = null

    const definition = stageDefinitions[result.stageIndex]
    const nextProgress = Math.min(
      definition.target,
      result.progress + result.value
    )
    const completedStage = nextProgress >= definition.target
    const nextStageIndex = completedStage
      ? ((result.stageIndex + 1) as StageIndex)
      : result.stageIndex
    const completedAllStages = nextStageIndex === 3

    setDemoState((previous) => {
      if (previous.stageIndex !== result.stageIndex) {
        return previous
      }

      return {
        ...previous,
        history: [
          {
            id: Date.now(),
            stageIndex: result.stageIndex,
            value: result.value,
            time: new Date().toLocaleTimeString([], {
              hour: '2-digit',
              minute: '2-digit',
            }),
          },
          ...previous.history,
        ].slice(0, 8),
        progress: completedStage ? 0 : nextProgress,
        stageIndex: nextStageIndex,
      }
    })

    celebrationSequenceRef.current += 1
    let celebrationKind: SpinCelebrationState['kind'] = 'result'
    if (completedAllStages) {
      celebrationKind = 'all-complete'
    } else if (completedStage) {
      celebrationKind = 'stage-complete'
    }
    setCelebration({
      key: celebrationSequenceRef.current,
      kind: celebrationKind,
      unit: t(definition.unit),
      value: result.value,
    })
    if (completedStage && !completedAllStages) {
      setPendingChallengeIndex(nextStageIndex as ChallengeStageIndex)
    }

    spinningRef.current = false
    setIsSpinning(false)
    if (completedAllStages) {
      toast.success(t('All preview stages completed'))
    } else if (completedStage) {
      toast.success(t('New challenge unlocked'))
    } else {
      toast.success(
        t('Demo result: +{{value}} {{unit}}', {
          value: result.value,
          unit: t(stageDefinitions[result.stageIndex].unit),
        })
      )
    }
  }

  const previewNextStage = () => {
    if (spinningRef.current || pendingChallengeIndex !== null) {
      return
    }
    const nextStageIndex = ((demoState.stageIndex + 1) % 4) as StageIndex
    setDemoState((previous) => ({
      ...previous,
      availableSpins: 0,
      progress: 0,
      stageIndex: nextStageIndex,
    }))
    if (nextStageIndex === 1 || nextStageIndex === 2) {
      setPendingChallengeIndex(nextStageIndex)
      setChallengeDialogOpen(true)
    }
  }

  const resetPreview = () => {
    pendingSpinRef.current = null
    spinningRef.current = false
    setIsSpinning(false)
    setWheelRotation(0)
    setCelebration(null)
    setPendingChallengeIndex(null)
    setChallengeDialogOpen(false)
    setDemoState(initialDemoState)
  }

  const completeCelebration = (key: number) => {
    if (celebration?.key !== key) {
      return
    }
    const shouldRevealChallenge =
      celebration.kind === 'stage-complete' && pendingChallengeIndex !== null
    setCelebration(null)
    if (shouldRevealChallenge) {
      setChallengeDialogOpen(true)
    }
  }

  const changeChallengeDialogOpen = (open: boolean) => {
    setChallengeDialogOpen(open)
    if (!open) {
      setPendingChallengeIndex(null)
    }
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Invitation Wheel')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button variant='outline' render={<Link to='/invite-rewards' />}>
          <ArrowLeft data-icon='inline-start' />
          {t('Back to Invitation Gifts')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <>
          <div className='mx-auto max-w-7xl space-y-5 pb-5'>
            <section
              ref={heroRef}
              className='relative isolate overflow-hidden rounded-[30px] border border-[#ffd47a]/45 text-white shadow-[0_28px_90px_rgba(112,5,20,0.24)]'
              style={{
                background:
                  'radial-gradient(circle at 8% 0%, rgba(255,230,165,0.32), transparent 27%), radial-gradient(circle at 92% 14%, rgba(255,187,85,0.28), transparent 26%), linear-gradient(145deg,#680513 0%,#a4081d 33%,#d62828 68%,#f06b36 100%)',
              }}
            >
              <div
                aria-hidden='true'
                className='pointer-events-none absolute inset-0 [background-image:radial-gradient(rgba(255,235,188,0.75)_1px,transparent_1px)] [mask-image:linear-gradient(to_bottom,black,transparent_88%)] [background-size:24px_24px] opacity-25'
              />
              <div
                aria-hidden='true'
                className='pointer-events-none absolute -top-40 -left-36 size-[430px] rounded-full border border-[#ffe7ae]/20'
              />
              <div
                aria-hidden='true'
                className='pointer-events-none absolute -right-48 -bottom-56 size-[540px] rounded-full border border-[#ffe7ae]/20'
              />

              <FestiveBackground
                active={heroIsVisible}
                reducedMotion={reducedMotion}
              />

              <div className='relative grid items-center gap-8 px-5 py-7 sm:px-8 sm:py-9 lg:grid-cols-[1.04fr_0.96fr] lg:px-11 lg:py-11'>
                <div className='relative z-10'>
                  <div className='inline-flex items-center gap-2 rounded-full border border-[#ffe8b6]/45 bg-[#5f0310]/45 px-3 py-1.5 text-xs font-semibold text-[#ffedc3] backdrop-blur-sm'>
                    <Info className='size-3.5' />
                    {t('Activity prototype · No real rewards')}
                  </div>

                  <h1 className='mt-5 max-w-3xl font-serif text-[2.4rem] leading-[1.05] font-bold tracking-[-0.035em] text-balance sm:text-5xl lg:text-[3.75rem]'>
                    {t('Invite 1 effective friend, earn 1 spin')}
                  </h1>
                  <p className='mt-4 max-w-2xl text-sm leading-7 text-[#fff1d7]/84 sm:text-base'>
                    {t(
                      'Keep spinning to move your lucky progress toward up to $10,000 in Default credit.'
                    )}
                  </p>

                  <div className='mt-7 rounded-3xl border border-white/15 bg-[#620612]/38 p-4 backdrop-blur-sm sm:p-5'>
                    <div className='sr-only' role='status' aria-live='polite'>
                      {t(
                        'Current activity status: {{progress}} of 100, {{spins}} spins available.',
                        {
                          progress: overallProgress,
                          spins: demoState.availableSpins,
                        }
                      )}
                    </div>
                    <div className='flex flex-wrap items-end justify-between gap-4'>
                      <div>
                        <div className='text-xs font-semibold tracking-[0.16em] text-[#ffe7ae]/72 uppercase'>
                          {t('Current progress')}
                        </div>
                        <div className='mt-1 flex items-end gap-2'>
                          <motion.span
                            key={`${demoState.stageIndex}-${demoState.progress}`}
                            className='font-serif text-5xl font-bold tracking-[-0.05em] text-[#ffe19a] tabular-nums sm:text-6xl'
                            initial={
                              reducedMotion
                                ? undefined
                                : { opacity: 0.72, scale: 1.12, y: -4 }
                            }
                            animate={{ opacity: 1, scale: 1, y: 0 }}
                            transition={{ duration: reducedMotion ? 0 : 0.42 }}
                          >
                            {overallProgress}
                          </motion.span>
                          <span className='mb-1.5 text-lg font-semibold text-white/58'>
                            / 100
                          </span>
                        </div>
                      </div>
                      <div className='text-right'>
                        <div className='text-xs text-[#ffe7ae]/72'>
                          {t('Current target')}
                        </div>
                        <div className='mt-1 font-semibold text-[#fff2d5]'>
                          {currentStage ? t(currentStage.goal) : t('Completed')}
                        </div>
                      </div>
                    </div>
                    <div
                      className='mt-4 h-3 overflow-hidden rounded-full bg-[#3b0209]/55 ring-1 ring-white/10'
                      role='progressbar'
                      aria-label={t('Current progress')}
                      aria-valuemin={0}
                      aria-valuemax={100}
                      aria-valuenow={overallProgressPercent}
                    >
                      <motion.div
                        className='h-full rounded-full bg-[linear-gradient(90deg,#ffd166,#fff0a8,#ffb347)] shadow-[0_0_18px_rgba(255,209,102,0.62)]'
                        animate={{ width: `${overallProgressPercent}%` }}
                        transition={{
                          duration: shouldReduceMotion ? 0 : 0.55,
                          ease: 'easeOut',
                        }}
                      />
                    </div>
                    <div className='mt-3 flex items-center justify-between gap-3 text-xs text-[#ffe7ae]/72'>
                      <span>{t('Challenge progress')}</span>
                      <span className='font-mono tabular-nums'>
                        {currentStage
                          ? `${demoState.progress} / ${currentStage.target} ${t(currentStage.unit)}`
                          : t('Completed')}
                      </span>
                    </div>
                    <div
                      className='mt-2 h-1.5 overflow-hidden rounded-full bg-[#3b0209]/48 ring-1 ring-white/8'
                      role='progressbar'
                      aria-label={t('Challenge progress')}
                      aria-valuemin={0}
                      aria-valuemax={100}
                      aria-valuenow={Math.round(currentProgressPercent)}
                    >
                      <motion.div
                        className='h-full rounded-full bg-[linear-gradient(90deg,#ffb347,#ffe79d)]'
                        animate={{ width: `${currentProgressPercent}%` }}
                        transition={{
                          duration: shouldReduceMotion ? 0 : 0.45,
                          ease: 'easeOut',
                        }}
                      />
                    </div>
                    <div className='mt-4 grid grid-cols-2 gap-2.5'>
                      <div className='rounded-2xl border border-white/10 bg-white/7 px-3 py-3'>
                        <div className='flex items-center gap-2 text-xs text-white/62'>
                          <Ticket className='size-3.5 text-[#ffe09a]' />
                          {t('Available spins')}
                        </div>
                        <div className='mt-1 font-serif text-2xl font-bold text-[#ffe09a] tabular-nums'>
                          {demoState.availableSpins}
                        </div>
                      </div>
                      <div className='rounded-2xl border border-white/10 bg-white/7 px-3 py-3'>
                        <div className='flex items-center gap-2 text-xs text-white/62'>
                          <UsersRound className='size-3.5 text-[#ffe09a]' />
                          {t('Effective invites')}
                        </div>
                        <div className='mt-1 font-serif text-2xl font-bold text-[#ffe09a] tabular-nums'>
                          {demoState.effectiveInvites}
                        </div>
                      </div>
                    </div>
                  </div>

                  <p className='mt-4 flex max-w-2xl items-start gap-2 text-xs leading-5 text-[#ffe5ac]/72'>
                    <ShieldCheck className='mt-0.5 size-4 shrink-0' />
                    {t(
                      'Formal launch rules and probabilities will be published before rewards are enabled.'
                    )}
                  </p>
                </div>

                <div className='relative mx-auto flex w-full max-w-[470px] flex-col items-center'>
                  <div
                    aria-hidden='true'
                    className='relative aspect-square w-full max-w-[410px]'
                  >
                    <motion.div
                      aria-hidden='true'
                      className='absolute inset-[1%] rounded-full border border-dashed border-[#ffe6ad]/38'
                      animate={shouldReduceMotion ? undefined : { rotate: 360 }}
                      transition={{
                        duration: 32,
                        ease: 'linear',
                        repeat: Infinity,
                      }}
                    />
                    <div className='absolute top-0 left-1/2 z-30 -translate-x-1/2 drop-shadow-[0_5px_8px_rgba(70,0,10,0.38)]'>
                      <div className='h-0 w-0 border-x-[14px] border-t-[25px] border-x-transparent border-t-[#fff1b9]' />
                    </div>

                    <div className='absolute inset-[7%] rounded-full bg-[#7a0616] p-3 shadow-[0_24px_70px_rgba(63,0,10,0.44),inset_0_0_0_2px_rgba(255,239,194,0.46)]'>
                      <motion.div
                        className='relative size-full overflow-hidden rounded-full border-[5px] border-[#ffe8ac] shadow-[inset_0_0_32px_rgba(117,10,24,0.28)]'
                        style={{
                          background:
                            'conic-gradient(from -22.5deg,#fff0b7 0deg 45deg,#ff9b54 45deg 90deg,#ffe5a0 90deg 135deg,#f66b4d 135deg 180deg,#fff0b7 180deg 225deg,#ff9b54 225deg 270deg,#ffe5a0 270deg 315deg,#f66b4d 315deg 360deg)',
                        }}
                        animate={{ rotate: wheelRotation }}
                        transition={{
                          duration: shouldReduceMotion ? 0.05 : 1.55,
                          ease: [0.12, 0.68, 0.16, 1],
                        }}
                        onAnimationComplete={settleSpin}
                      >
                        {wheelValues.map((value, index) => {
                          const angle = index * 45
                          return (
                            <div
                              key={value}
                              className='absolute inset-0'
                              style={{ transform: `rotate(${angle}deg)` }}
                            >
                              <div
                                className='absolute top-[7%] left-1/2 flex -translate-x-1/2 flex-col items-center font-serif font-bold text-[#8b101b] drop-shadow-[0_1px_0_rgba(255,255,255,0.45)]'
                                style={{ transform: `rotate(${-angle}deg)` }}
                              >
                                <span className='text-xl tabular-nums sm:text-2xl'>
                                  +{value}
                                </span>
                              </div>
                            </div>
                          )
                        })}
                        <div className='absolute inset-[35%] rounded-full border-4 border-[#fff0bd] bg-[linear-gradient(145deg,#ffda72,#ff9a32)] shadow-[0_8px_30px_rgba(104,7,20,0.34),inset_0_2px_1px_rgba(255,255,255,0.65)]' />
                      </motion.div>

                      <div className='pointer-events-none absolute inset-[35%] z-20 flex flex-col items-center justify-center rounded-full text-center text-[#8b0d1a]'>
                        <Gift className='size-7 sm:size-9' strokeWidth={2.2} />
                        <span className='mt-1 text-[10px] font-bold sm:text-xs'>
                          {currentStage ? t(currentStage.unit) : t('Completed')}
                        </span>
                      </div>
                    </div>

                    <motion.div
                      aria-hidden='true'
                      className='absolute top-[11%] right-[5%] text-[#fff1b9]'
                      animate={
                        shouldReduceMotion
                          ? undefined
                          : { rotate: [0, 22, 0], scale: [0.9, 1.25, 0.9] }
                      }
                      transition={{
                        duration: 2.6,
                        ease: 'easeInOut',
                        repeat: Infinity,
                      }}
                    >
                      <Sparkles className='size-9' />
                    </motion.div>

                    <SpinCelebration
                      celebration={celebration}
                      reducedMotion={reducedMotion}
                      onComplete={completeCelebration}
                    />
                  </div>

                  <motion.div
                    className='relative z-30 -mt-3 w-full max-w-[330px]'
                    whileTap={shouldReduceMotion ? undefined : { scale: 0.985 }}
                  >
                    {demoState.availableSpins > 0 &&
                      !isSpinning &&
                      !reducedMotion && (
                        <motion.span
                          aria-hidden='true'
                          className='pointer-events-none absolute -inset-1 rounded-full border border-[#ffe99d]/85 shadow-[0_0_28px_rgba(255,223,115,0.52)]'
                          animate={{
                            opacity: [0.18, 0.72, 0],
                            scale: [1, 1.09, 1.13],
                          }}
                          transition={{
                            duration: 1.8,
                            ease: 'easeOut',
                            repeat: Infinity,
                          }}
                        />
                      )}
                    <Button
                      size='lg'
                      onClick={spinWheel}
                      disabled={
                        isSpinning ||
                        pendingChallengeIndex !== null ||
                        demoState.availableSpins < 1 ||
                        demoState.stageIndex === 3
                      }
                      className='relative h-14 w-full overflow-hidden rounded-full border border-[#fff2c5] bg-[linear-gradient(135deg,#fff2bd,#ffc34d,#ffe49a)] text-base font-bold text-[#8c0d19] shadow-[0_12px_34px_rgba(71,0,10,0.38)] hover:bg-[linear-gradient(135deg,#fff8d8,#ffd168,#fff0ba)] disabled:border-white/20 disabled:bg-white/12 disabled:text-white/55'
                    >
                      {demoState.availableSpins > 0 &&
                        !isSpinning &&
                        !reducedMotion && (
                          <motion.span
                            aria-hidden='true'
                            className='pointer-events-none absolute inset-y-[-30%] -left-[45%] w-[28%] rotate-12 bg-white/65 blur-[1px]'
                            animate={{ x: ['0%', '560%'] }}
                            transition={{
                              delay: 0.6,
                              duration: 0.95,
                              ease: 'easeInOut',
                              repeat: Infinity,
                              repeatDelay: 2.4,
                            }}
                          />
                        )}
                      <span className='relative z-10 flex items-center justify-center gap-2'>
                        <RotateCw
                          className={cn(
                            'size-4',
                            isSpinning && !shouldReduceMotion && 'animate-spin'
                          )}
                        />
                        {spinButtonLabel}
                      </span>
                    </Button>
                  </motion.div>
                </div>
              </div>
            </section>

            {import.meta.env.DEV && (
              <section className='rounded-2xl border border-dashed border-amber-400/70 bg-amber-50/65 p-4 dark:border-amber-800 dark:bg-amber-950/18'>
                <div className='flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between'>
                  <div className='flex items-start gap-3'>
                    <div className='rounded-xl bg-amber-500/15 p-2 text-amber-700 dark:text-amber-300'>
                      <PartyPopper className='size-5' />
                    </div>
                    <div>
                      <div className='flex flex-wrap items-center gap-2'>
                        <span className='font-semibold'>
                          {t('Demo controls')}
                        </span>
                        <StatusBadge
                          label={t('Local preview only')}
                          variant='warning'
                          copyable={false}
                        />
                      </div>
                      <p className='text-muted-foreground mt-1 text-xs leading-5'>
                        {t(
                          'Demo spins use fixed outcomes and do not represent official probabilities.'
                        )}
                      </p>
                    </div>
                  </div>
                  <div className='flex flex-wrap gap-2'>
                    <Button
                      onClick={simulateEffectiveInvite}
                      disabled={
                        isSpinning ||
                        pendingChallengeIndex !== null ||
                        demoState.stageIndex === 3
                      }
                      className='bg-red-600 text-white hover:bg-red-700'
                    >
                      <UserPlus data-icon='inline-start' />
                      {t('Simulate 1 effective friend')}
                    </Button>
                    <Button
                      variant='outline'
                      onClick={previewNextStage}
                      disabled={isSpinning || pendingChallengeIndex !== null}
                    >
                      <Target data-icon='inline-start' />
                      {t('Preview new challenge')}
                    </Button>
                    <Button
                      variant='ghost'
                      onClick={resetPreview}
                      disabled={isSpinning}
                    >
                      <RefreshCcw data-icon='inline-start' />
                      {t('Reset preview')}
                    </Button>
                  </div>
                </div>
              </section>
            )}

            <div className='grid gap-5 lg:grid-cols-[1.08fr_0.92fr]'>
              <Card data-card-hover='false'>
                <CardHeader>
                  <CardTitle className='flex items-center gap-2'>
                    <Share2 className='size-5 text-red-600' />
                    {t('Share my invitation')}
                  </CardTitle>
                  <CardDescription>
                    {t('Copy your personal link and invite a new user.')}
                  </CardDescription>
                </CardHeader>
                <CardContent className='space-y-4'>
                  <div className='grid gap-2 sm:grid-cols-[1fr_auto_auto]'>
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
                    <Button
                      onClick={shareReferralLink}
                      disabled={!referralLink}
                      className='bg-red-600 text-white hover:bg-red-700'
                    >
                      <Share2 data-icon='inline-start' />
                      {t('Share link')}
                    </Button>
                  </div>
                  <StatusBadge
                    label={`${t('Invitation code')}: ${referralCode || '—'}`}
                    variant='info'
                    copyText={referralCode}
                    copyable={Boolean(referralCode)}
                  />
                </CardContent>
              </Card>

              <Card data-card-hover='false'>
                <CardHeader>
                  <CardTitle className='flex items-center gap-2'>
                    <History className='size-5 text-amber-600' />
                    {t('Spin history')}
                  </CardTitle>
                  <CardDescription>
                    {t('Preview results are stored only in this page session.')}
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  {demoState.history.length === 0 ? (
                    <div className='text-muted-foreground flex min-h-24 flex-col items-center justify-center rounded-xl border border-dashed text-sm'>
                      <RotateCw className='mb-2 size-5 opacity-55' />
                      {t('No spins yet')}
                    </div>
                  ) : (
                    <div className='max-h-48 space-y-2 overflow-auto pr-1'>
                      {demoState.history.map((item) => (
                        <div
                          key={item.id}
                          className='flex items-center justify-between gap-3 rounded-xl border px-3 py-2.5'
                        >
                          <div className='flex items-center gap-2.5'>
                            <div className='rounded-full bg-amber-500/12 p-1.5 text-amber-700 dark:text-amber-300'>
                              <Sparkles className='size-3.5' />
                            </div>
                            <div>
                              <div className='text-sm font-medium'>
                                {t('Wheel reward')}
                              </div>
                              <div className='text-muted-foreground text-[11px]'>
                                {item.time}
                              </div>
                            </div>
                          </div>
                          <div className='font-serif font-bold text-amber-700 tabular-nums dark:text-amber-300'>
                            +{item.value}{' '}
                            {t(stageDefinitions[item.stageIndex].unit)}
                          </div>
                        </div>
                      ))}
                    </div>
                  )}
                </CardContent>
              </Card>
            </div>

            <Card data-card-hover='false' className='overflow-hidden'>
              <CardHeader className='border-b bg-[linear-gradient(135deg,rgba(220,38,38,0.06),rgba(245,158,11,0.07))]'>
                <CardTitle className='flex items-center gap-2'>
                  <Trophy className='size-5 text-amber-600' />
                  {t('How it works')}
                </CardTitle>
                <CardDescription>
                  {t(
                    'The formal event will use a server-side ledger. This page currently demonstrates only the interaction and challenge flow.'
                  )}
                </CardDescription>
              </CardHeader>
              <CardContent className='grid gap-3 pt-5 md:grid-cols-2'>
                {[
                  [UserPlus, 'One effective new user = one wheel spin.'],
                  [
                    RotateCw,
                    'Each spin result is settled by the server during the official event.',
                  ],
                  [
                    CheckCircle2,
                    'The current challenge is shown clearly. Any new challenge is revealed only after the current goal is completed.',
                  ],
                  [
                    ShieldCheck,
                    '$10,000 is Default-pool credit and cannot be withdrawn.',
                  ],
                ].map(([Icon, copy]) => {
                  const RuleIcon = Icon as typeof UserPlus
                  return (
                    <div
                      key={copy as string}
                      className='bg-muted/25 flex items-start gap-3 rounded-2xl border p-4'
                    >
                      <div className='rounded-xl bg-red-500/10 p-2 text-red-600 dark:text-red-300'>
                        <RuleIcon className='size-4' />
                      </div>
                      <p className='text-sm leading-6'>{t(copy as string)}</p>
                    </div>
                  )
                })}
              </CardContent>
            </Card>
          </div>
          {pendingChallenge && (
            <ChallengeRevealDialog
              open={challengeDialogOpen}
              onOpenChange={changeChallengeDialogOpen}
              goal={pendingChallenge.goal}
              overallProgress={overallProgress}
              target={pendingChallenge.target}
              unit={pendingChallenge.unit}
            />
          )}
        </>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
