/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { api } from '@/lib/api'

export interface RechargeInfo {
  enable_online_topup: boolean
  enable_stripe_topup: boolean
  enable_creem_topup?: boolean
  enable_waffo_topup?: boolean
  enable_waffo_pancake_topup?: boolean
  enable_redemption?: boolean
  topup_link?: string
  pay_methods?: Array<{ name?: string; type?: string }>
}

export interface RechargeRecord {
  id: number
  amount: number
  money: number
  trade_no: string
  payment_method: string
  create_time: number
  complete_time?: number
  status: string
}

interface PageData<T> {
  items?: T[]
  total?: number
  page?: number
  page_size?: number
}

export async function getRechargeInfo(): Promise<RechargeInfo> {
  const response = await api.get('/api/user/topup/info')
  if (!response.data?.success || !response.data?.data) {
    throw new Error(response.data?.message || 'Failed to load recharge info')
  }
  return response.data.data
}

export async function getPersonalInviteCode(): Promise<string> {
  const response = await api.get('/api/user/aff')
  if (!response.data?.success) {
    throw new Error(response.data?.message || 'Failed to load invitation code')
  }
  return String(response.data?.data ?? '')
}

export async function getRechargeHistory(): Promise<RechargeRecord[]> {
  const response = await api.get('/api/user/topup/self?p=1&page_size=8')
  if (!response.data?.success || !response.data?.data) {
    throw new Error(response.data?.message || 'Failed to load recharge history')
  }
  const page = response.data?.data as PageData<RechargeRecord> | undefined
  return Array.isArray(page?.items) ? page.items : []
}

export async function redeemRechargeCode(key: string): Promise<number> {
  const response = await api.post('/api/user/topup', { key })
  if (!response.data?.success && response.data?.message !== 'success') {
    throw new Error(response.data?.message || 'Redemption failed')
  }
  return Number(response.data?.data) || 0
}
