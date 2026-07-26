/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { motion } from 'motion/react'

import { cn } from '@/lib/utils'

const festiveParticles = [
  { id: 'spark-1', left: '5%', top: '13%', delay: 0.2, size: 6 },
  { id: 'spark-2', left: '12%', top: '76%', delay: 1.1, size: 4 },
  { id: 'spark-3', left: '24%', top: '7%', delay: 0.7, size: 7 },
  { id: 'spark-4', left: '45%', top: '10%', delay: 0.4, size: 5 },
  { id: 'spark-5', left: '64%', top: '82%', delay: 1.3, size: 7 },
  { id: 'spark-6', left: '83%', top: '9%', delay: 0.9, size: 4 },
  { id: 'spark-7', left: '94%', top: '75%', delay: 1.8, size: 7 },
] as const

const packetRain = [
  {
    id: 'packet-1',
    left: '2%',
    delay: 0,
    duration: 7.2,
    size: 23,
    sway: 19,
    mobile: true,
  },
  {
    id: 'packet-2',
    left: '11%',
    delay: 3.4,
    duration: 6.4,
    size: 28,
    sway: -23,
    mobile: false,
  },
  {
    id: 'packet-3',
    left: '21%',
    delay: 1.5,
    duration: 8.1,
    size: 20,
    sway: 16,
    mobile: true,
  },
  {
    id: 'packet-4',
    left: '36%',
    delay: 5.2,
    duration: 7.5,
    size: 25,
    sway: -18,
    mobile: false,
  },
  {
    id: 'packet-5',
    left: '54%',
    delay: 2.6,
    duration: 6.8,
    size: 22,
    sway: 22,
    mobile: true,
  },
  {
    id: 'packet-6',
    left: '69%',
    delay: 0.8,
    duration: 8.4,
    size: 29,
    sway: -24,
    mobile: false,
  },
  {
    id: 'packet-7',
    left: '83%',
    delay: 4.1,
    duration: 7.1,
    size: 21,
    sway: 17,
    mobile: true,
  },
  {
    id: 'packet-8',
    left: '94%',
    delay: 2,
    duration: 7.8,
    size: 26,
    sway: -19,
    mobile: true,
  },
] as const

const floatingCoins = [
  {
    id: 'coin-1',
    left: '4%',
    top: '44%',
    size: 27,
    delay: 0.2,
    mobile: true,
  },
  {
    id: 'coin-2',
    left: '29%',
    top: '88%',
    size: 20,
    delay: 1.3,
    mobile: false,
  },
  {
    id: 'coin-3',
    left: '76%',
    top: '14%',
    size: 24,
    delay: 0.8,
    mobile: false,
  },
  {
    id: 'coin-4',
    left: '91%',
    top: '55%',
    size: 31,
    delay: 1.8,
    mobile: true,
  },
] as const

const celebrationCoins = [
  { id: 'burst-1', angle: -102, distance: 142, size: 18, delay: 0 },
  { id: 'burst-2', angle: -73, distance: 128, size: 14, delay: 0.03 },
  { id: 'burst-3', angle: -43, distance: 150, size: 20, delay: 0.06 },
  { id: 'burst-4', angle: -12, distance: 136, size: 15, delay: 0.01 },
  { id: 'burst-5', angle: 18, distance: 145, size: 19, delay: 0.07 },
  { id: 'burst-6', angle: 49, distance: 126, size: 13, delay: 0.02 },
  { id: 'burst-7', angle: 82, distance: 153, size: 18, delay: 0.05 },
  { id: 'burst-8', angle: 116, distance: 132, size: 16, delay: 0.08 },
  { id: 'burst-9', angle: 151, distance: 146, size: 20, delay: 0.04 },
  { id: 'burst-10', angle: 185, distance: 128, size: 14, delay: 0.09 },
] as const

export type SpinCelebrationState = {
  key: number
  kind: 'result' | 'stage-complete' | 'all-complete'
  unit: string
  value: number
}

type FestiveBackgroundProps = {
  active: boolean
  reducedMotion: boolean
}

function RedPacket({ size }: { size: number }) {
  return (
    <span
      className='relative block rounded-[5px] border border-[#ffd56d]/75 bg-[linear-gradient(145deg,#ff7145,#df142f_58%,#a80020)] shadow-[0_7px_16px_rgba(70,0,12,0.32),0_0_13px_rgba(255,194,82,0.28)]'
      style={{ width: size, height: size * 1.26 }}
    >
      <span className='absolute inset-x-[10%] top-[8%] h-[34%] rounded-b-[50%] border-b border-[#ffd56d]/80 bg-[linear-gradient(180deg,#ff8b55,#ed2940)]' />
      <span
        className='absolute top-[38%] left-1/2 flex -translate-x-1/2 items-center justify-center rounded-full border border-[#fff0a8] bg-[linear-gradient(145deg,#fff1a9,#f7ae28)] font-serif font-black text-[#b20d24] shadow-[0_2px_6px_rgba(87,0,13,0.28)]'
        style={{
          width: size * 0.42,
          height: size * 0.42,
          fontSize: size * 0.25,
        }}
      >
        福
      </span>
    </span>
  )
}

function GoldCoin({ size }: { size: number }) {
  return (
    <span
      className='flex items-center justify-center rounded-full border-2 border-[#fff0a5] bg-[radial-gradient(circle_at_35%_28%,#fff9c7,#ffd45f_40%,#f39b19_78%,#bb5b09)] font-serif font-black text-[#a44508] shadow-[0_5px_16px_rgba(84,0,11,0.28),inset_0_0_0_2px_rgba(255,245,176,0.38)]'
      style={{ width: size, height: size, fontSize: size * 0.53 }}
    >
      $
    </span>
  )
}

export function FestiveBackground({
  active,
  reducedMotion,
}: FestiveBackgroundProps) {
  return (
    <div aria-hidden='true' className='pointer-events-none absolute inset-0'>
      {!reducedMotion && active && (
        <>
          <motion.span
            className='absolute -top-8 left-[57%] size-40 rounded-full [mask-image:radial-gradient(circle,transparent_0_17%,black_20%_62%,transparent_72%)] opacity-0 [background:repeating-conic-gradient(from_0deg,transparent_0deg_13deg,rgba(255,232,160,0.92)_14deg_16deg,transparent_17deg_30deg)]'
            animate={{
              opacity: [0, 0.64, 0],
              rotate: [0, 13, 23],
              scale: [0.3, 1, 1.2],
            }}
            transition={{
              duration: 2.2,
              ease: 'easeOut',
              repeat: Infinity,
              repeatDelay: 4.5,
            }}
          />
          <motion.span
            className='absolute top-[34%] -right-11 size-36 rounded-full [mask-image:radial-gradient(circle,transparent_0_15%,black_18%_60%,transparent_71%)] opacity-0 [background:repeating-conic-gradient(from_6deg,transparent_0deg_17deg,rgba(255,198,83,0.82)_18deg_20deg,transparent_21deg_36deg)]'
            animate={{
              opacity: [0, 0.52, 0],
              rotate: [0, -9, -18],
              scale: [0.34, 1, 1.18],
            }}
            transition={{
              delay: 2.1,
              duration: 2.1,
              ease: 'easeOut',
              repeat: Infinity,
              repeatDelay: 5.2,
            }}
          />
        </>
      )}

      {festiveParticles.map((particle, index) => (
        <motion.span
          key={particle.id}
          className='absolute rounded-[2px] bg-[#ffe6a3] shadow-[0_0_15px_rgba(255,226,150,0.75)]'
          style={{
            left: particle.left,
            top: particle.top,
            width: particle.size,
            height: particle.size * 1.8,
          }}
          animate={
            active && !reducedMotion
              ? {
                  y: [0, -18 - (index % 3) * 5, 0],
                  rotate: [0, 180, 360],
                  opacity: [0.22, 0.9, 0.22],
                }
              : undefined
          }
          transition={{
            delay: particle.delay,
            duration: 4.2 + (index % 4) * 0.65,
            ease: 'easeInOut',
            repeat: Infinity,
          }}
        />
      ))}

      {reducedMotion
        ? packetRain.slice(0, 3).map((packet, index) => (
            <span
              key={packet.id}
              className='absolute opacity-45'
              style={{ left: packet.left, top: `${19 + index * 28}%` }}
            >
              <RedPacket size={packet.size} />
            </span>
          ))
        : active &&
          packetRain.map((packet) => (
            <motion.span
              key={packet.id}
              className={cn(
                'absolute inset-y-0 w-10',
                !packet.mobile && 'hidden sm:block'
              )}
              style={{ left: packet.left }}
              initial={{ y: '-14%' }}
              animate={{ y: ['-14%', '114%'] }}
              transition={{
                delay: packet.delay,
                duration: packet.duration,
                ease: 'linear',
                repeat: Infinity,
                repeatDelay: 1.3,
              }}
            >
              <motion.span
                className='absolute top-0 left-1/2 block'
                style={{ marginLeft: packet.size * -0.5 }}
                animate={{
                  x: [0, packet.sway, packet.sway * -0.45, 0],
                  rotate: [-12, 9, -7, 13],
                }}
                transition={{
                  duration: packet.duration * 0.62,
                  ease: 'easeInOut',
                  repeat: Infinity,
                }}
              >
                <RedPacket size={packet.size} />
              </motion.span>
            </motion.span>
          ))}

      {floatingCoins.map((coin, index) => (
        <motion.span
          key={coin.id}
          className={cn(
            'absolute opacity-70',
            !coin.mobile && 'hidden sm:block'
          )}
          style={{ left: coin.left, top: coin.top }}
          animate={
            active && !reducedMotion
              ? {
                  y: [0, -12 - index * 3, 3, 0],
                  rotateY: [0, 180, 360],
                  rotateZ: [index % 2 ? -6 : 5, index % 2 ? 6 : -5],
                }
              : undefined
          }
          transition={{
            delay: coin.delay,
            duration: 4.8 + index * 0.7,
            ease: 'easeInOut',
            repeat: Infinity,
          }}
        >
          <GoldCoin size={coin.size} />
        </motion.span>
      ))}
    </div>
  )
}

type SpinCelebrationProps = {
  celebration: SpinCelebrationState | null
  onComplete: (key: number) => void
  reducedMotion: boolean
}

export function SpinCelebration({
  celebration,
  onComplete,
  reducedMotion,
}: SpinCelebrationProps) {
  if (!celebration) {
    return null
  }

  const isStageCelebration = celebration.kind !== 'result'
  const fireworks =
    celebration.kind === 'all-complete'
      ? [
          { id: 'firework-left', left: '8%', top: '17%', delay: 0.08 },
          { id: 'firework-center', left: '50%', top: '4%', delay: 0.22 },
          { id: 'firework-right', left: '78%', top: '21%', delay: 0.14 },
        ]
      : [
          { id: 'firework-left', left: '9%', top: '22%', delay: 0.08 },
          { id: 'firework-right', left: '76%', top: '19%', delay: 0.18 },
        ]

  return (
    <motion.div
      key={celebration.key}
      aria-hidden='true'
      className='pointer-events-none absolute inset-0 z-40 overflow-visible'
      initial={{ opacity: 0 }}
      animate={{ opacity: [0, 1, 1, 0] }}
      transition={{
        duration: reducedMotion ? 1.15 : 1.55,
        times: [0, 0.1, 0.72, 1],
      }}
      onAnimationComplete={() => onComplete(celebration.key)}
    >
      {!reducedMotion &&
        celebrationCoins.map((coin) => {
          const radians = (coin.angle * Math.PI) / 180
          const x = Math.cos(radians) * coin.distance
          const y = Math.sin(radians) * coin.distance
          return (
            <motion.span
              key={coin.id}
              className='absolute top-1/2 left-1/2'
              style={{
                marginLeft: coin.size * -0.5,
                marginTop: coin.size * -0.5,
              }}
              initial={{ opacity: 0, scale: 0.3, x: 0, y: 0 }}
              animate={{
                opacity: [0, 1, 1, 0],
                scale: [0.3, 1.12, 0.92],
                x: [0, x * 0.58, x],
                y: [0, y * 0.54, y + 22],
                rotate: [0, coin.angle * 1.8, coin.angle * 3.2],
              }}
              transition={{
                delay: coin.delay,
                duration: isStageCelebration ? 1.25 : 1.05,
                ease: [0.18, 0.78, 0.24, 1],
              }}
            >
              <GoldCoin size={coin.size} />
            </motion.span>
          )
        })}

      {isStageCelebration &&
        !reducedMotion &&
        fireworks.map((firework) => (
          <motion.span
            key={firework.id}
            className='absolute size-28 rounded-full [mask-image:radial-gradient(circle,transparent_0_14%,black_17%_64%,transparent_74%)] [background:repeating-conic-gradient(from_0deg,transparent_0deg_12deg,#fff3b0_13deg_15deg,transparent_16deg_27deg,#ffb62f_28deg_30deg,transparent_31deg_44deg)]'
            style={{
              left: firework.left,
              marginLeft: -56,
              marginTop: -56,
              top: firework.top,
            }}
            initial={{ opacity: 0, rotate: -10, scale: 0.15 }}
            animate={{ opacity: [0, 1, 0], rotate: 17, scale: [0.15, 1, 1.18] }}
            transition={{
              delay: firework.delay,
              duration: 1.05,
              ease: 'easeOut',
            }}
          />
        ))}

      <div className='absolute inset-0 flex items-center justify-center'>
        <motion.div
          className='min-w-28 rounded-2xl border border-[#fff1b5] bg-[linear-gradient(145deg,rgba(111,4,22,0.95),rgba(184,13,38,0.95))] px-4 py-3 text-center text-[#fff0b5] shadow-[0_13px_38px_rgba(67,0,11,0.46),0_0_32px_rgba(255,198,74,0.42)]'
          initial={
            reducedMotion ? { opacity: 0 } : { opacity: 0, scale: 0.65, y: 18 }
          }
          animate={
            reducedMotion
              ? { opacity: [0, 1, 1, 0] }
              : {
                  opacity: [0, 1, 1, 0],
                  scale: [0.65, 1.12, 1],
                  y: [18, -7, -24],
                }
          }
          transition={{
            duration: reducedMotion ? 1 : 1.35,
            ease: 'easeOut',
            times: [0, 0.16, 0.72, 1],
          }}
        >
          <div className='font-serif text-3xl font-black tabular-nums'>
            +{celebration.value}
          </div>
          <div className='mt-0.5 text-[10px] font-bold tracking-[0.16em] uppercase'>
            {celebration.unit}
          </div>
        </motion.div>
      </div>
    </motion.div>
  )
}
