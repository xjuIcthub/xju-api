/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { Gift, Sparkles } from 'lucide-react'
import { motion, useReducedMotion } from 'motion/react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

type ChallengeRevealDialogProps = {
  goal: string
  onOpenChange: (open: boolean) => void
  open: boolean
  overallProgress: string
  target: number
  unit: string
}

export function ChallengeRevealDialog({
  goal,
  onOpenChange,
  open,
  overallProgress,
  target,
  unit,
}: ChallengeRevealDialogProps) {
  const { t } = useTranslation()
  const shouldReduceMotion = useReducedMotion()

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        showCloseButton={false}
        className='overflow-hidden border-0 bg-transparent p-0 text-white ring-0 sm:max-w-[470px]'
      >
        <div className='relative isolate overflow-hidden rounded-[26px] border border-[#ffe09a]/55 bg-[linear-gradient(150deg,#6f0617_0%,#b40e26_48%,#e24a2f_100%)] px-5 py-6 shadow-[0_28px_90px_rgba(65,0,12,0.44)] sm:px-7 sm:py-8'>
          <div
            aria-hidden='true'
            className='pointer-events-none absolute inset-0 [background-image:radial-gradient(rgba(255,235,177,0.72)_1px,transparent_1px)] [background-size:22px_22px] opacity-20'
          />
          <div
            aria-hidden='true'
            className='pointer-events-none absolute -top-24 -right-20 size-64 rounded-full border border-[#ffe6a9]/25'
          />
          <div
            aria-hidden='true'
            className='pointer-events-none absolute -bottom-32 -left-24 size-72 rounded-full border border-[#ffe6a9]/20'
          />

          <motion.div
            aria-hidden='true'
            className='pointer-events-none absolute top-5 right-7 text-[#ffe8a3]'
            animate={
              shouldReduceMotion
                ? undefined
                : { rotate: [0, 18, 0], scale: [0.9, 1.2, 0.9] }
            }
            transition={{ duration: 2.2, ease: 'easeInOut', repeat: Infinity }}
          >
            <Sparkles className='size-9' />
          </motion.div>

          <div className='relative z-10'>
            <DialogHeader className='items-center text-center'>
              <motion.div
                className='flex size-14 items-center justify-center rounded-2xl border border-[#fff1bd]/70 bg-[linear-gradient(145deg,#ffeaa0,#f6ae31)] text-[#9b0b1d] shadow-[0_10px_30px_rgba(70,0,10,0.3)]'
                initial={
                  shouldReduceMotion
                    ? undefined
                    : { opacity: 0, rotate: -12, scale: 0.55 }
                }
                animate={{ opacity: 1, rotate: 0, scale: 1 }}
                transition={{ duration: shouldReduceMotion ? 0 : 0.45 }}
              >
                <Gift className='size-7' />
              </motion.div>
              <div className='mt-2 rounded-full border border-[#ffe7a9]/45 bg-[#650310]/38 px-3 py-1 text-[11px] font-semibold tracking-[0.14em] text-[#ffe7ae] uppercase'>
                {t('Current goal completed')}
              </div>
              <div className='mt-1 font-serif text-4xl font-black tracking-[-0.04em] text-[#ffe39a] tabular-nums'>
                {overallProgress}
              </div>
              <div className='text-xs font-medium text-[#ffe9bb]/70'>
                {t('Progress upgraded')}
              </div>
              <DialogTitle className='mt-3 font-serif text-3xl font-bold text-white sm:text-4xl'>
                {t('New challenge unlocked')}
              </DialogTitle>
              <DialogDescription className='max-w-sm text-sm leading-6 text-[#fff0d2]/78'>
                {t(
                  'Keep inviting effective friends. Each effective friend still unlocks one spin for the current challenge.'
                )}
              </DialogDescription>
            </DialogHeader>

            <div className='mt-5 rounded-2xl border border-white/15 bg-[#640511]/42 p-4 backdrop-blur-sm'>
              <div className='text-xs font-semibold tracking-[0.15em] text-[#ffe2a0]/72 uppercase'>
                {t('Current target')}
              </div>
              <div className='mt-1.5 text-xl font-bold text-[#fff2d2]'>
                {t(goal)}
              </div>
              <div className='mt-3 flex items-center justify-between gap-3 text-xs text-[#ffe6b0]/72'>
                <span>{t('Challenge progress')}</span>
                <span className='font-mono tabular-nums'>
                  0 / {target} {t(unit)}
                </span>
              </div>
              <div className='mt-2 h-2 overflow-hidden rounded-full bg-[#350108]/58 ring-1 ring-white/10'>
                <div className='h-full w-[4%] rounded-full bg-[linear-gradient(90deg,#ffd166,#fff0a8)] shadow-[0_0_13px_rgba(255,209,102,0.45)]' />
              </div>
            </div>

            <Button
              autoFocus
              size='lg'
              onClick={() => onOpenChange(false)}
              className='mt-5 h-12 w-full rounded-full border border-[#fff1bd] bg-[linear-gradient(135deg,#fff1b2,#ffc44f,#ffe59c)] font-bold text-[#8d0b1b] shadow-[0_10px_28px_rgba(58,0,9,0.35)] hover:bg-[linear-gradient(135deg,#fff7cf,#ffd36c,#fff0ba)]'
            >
              <Sparkles data-icon='inline-start' />
              {t('Continue challenge')}
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}
