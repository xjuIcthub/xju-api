/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { Crown } from 'lucide-react'

import type { PremiumTier } from '@/lib/premium'
import { cn } from '@/lib/utils'

interface PremiumUsernameProps {
  name: string
  tier?: PremiumTier
  className?: string
  textClassName?: string
}

export function PremiumUsername({
  name,
  tier = 'none',
  className,
  textClassName,
}: PremiumUsernameProps) {
  const hasGoldName = tier !== 'none'
  const crown =
    tier === 'silver_crown' || tier === 'gold_crown' ? tier : undefined

  return (
    <span
      className={cn(
        'relative inline-flex min-w-0 max-w-full items-center',
        className
      )}
      title={name}
    >
      <span
        className={cn(
          'min-w-0 truncate',
          hasGoldName && 'premium-username-gold',
          textClassName
        )}
      >
        {name}
      </span>
      {crown && (
        <Crown
          aria-label={crown === 'gold_crown' ? 'Gold crown' : 'Silver crown'}
          className={cn(
            'premium-username-crown absolute -top-2.5 -right-3 size-3.5',
            crown === 'gold_crown'
              ? 'premium-username-crown-gold'
              : 'premium-username-crown-silver'
          )}
          strokeWidth={2.2}
        />
      )}
    </span>
  )
}
