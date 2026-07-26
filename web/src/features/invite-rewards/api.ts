/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { api } from '@/lib/api'

export async function getPersonalInviteCode(): Promise<string> {
  const response = await api.get('/api/user/aff')
  if (!response.data?.success) {
    throw new Error(response.data?.message || 'Failed to load invitation code')
  }
  return String(response.data?.data ?? '')
}
