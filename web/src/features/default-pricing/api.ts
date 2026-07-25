/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { api } from '@/lib/api'

export interface DefaultPoolPricing {
  multiplier: number
}

export async function getDefaultPoolPricing(): Promise<DefaultPoolPricing> {
  const response = await api.get('/api/pool/default-pricing')
  if (!response.data?.success || !response.data?.data) {
    throw new Error(response.data?.message || 'Failed to load Default pricing')
  }
  return response.data.data
}

export async function updateDefaultPoolPricing(
  multiplier: number
): Promise<DefaultPoolPricing> {
  const response = await api.put('/api/pool/default-pricing', { multiplier })
  if (!response.data?.success || !response.data?.data) {
    throw new Error(
      response.data?.message || 'Failed to update Default pricing'
    )
  }
  return response.data.data
}
