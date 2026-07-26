/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { api } from '@/lib/api'

/**
 * xju-api:new — account-pool auth files, proxied by new-api's root-only /api/pool endpoints
 * to the CLIProxyAPI management API. Adding an account here is the same as
 * dropping an auth JSON into the pool — the pool hot-reloads, no restart.
 */

/** One 10-minute bucket of the account's recent request outcomes. */
type RecentRequestBucket = {
  time: string
  success: number
  failed: number
}

export type PoolAuthFile = {
  name: string
  email?: string
  account?: string
  account_type?: string
  disabled?: boolean
  unavailable?: boolean
  failed?: number
  success?: number
  status_message?: string
  last_refresh?: string
  updated_at?: string
  next_retry_after?: string
  // auth_index is the stable runtime id the management api-call pins a probe to.
  auth_index?: string
  recent_requests?: RecentRequestBucket[]
  // Codex accounts carry their ChatGPT subscription window + plan in the id_token.
  id_token?: {
    chatgpt_subscription_active_until?: string | number
    plan_type?: string
    chatgpt_account_id?: string
  }
  // The management API returns yet more metadata; keep it open.
  [key: string]: unknown
}

type ApiEnvelope<T> = {
  success: boolean
  message?: string
  data?: T
}

/**
 * The management API's list payload isn't a bare array — normalize the couple
 * of shapes it can take (array, or {files:[...]}/{items:[...]}) into a list.
 */
function normalizeList(data: unknown): PoolAuthFile[] {
  if (Array.isArray(data)) return data as PoolAuthFile[]
  if (data && typeof data === 'object') {
    const obj = data as Record<string, unknown>
    for (const key of ['files', 'items', 'auth_files', 'data']) {
      if (Array.isArray(obj[key])) return obj[key] as PoolAuthFile[]
    }
  }
  return []
}

export type PoolInfo = {
  id: string
  label: string
  provider?: 'codex' | 'claude'
  build_mode?: 'cliproxy' | 'gopool'
  owner_user_id?: number
  owner_username?: string
  kind?: 'system' | 'admin' | 'private'
  group_key?: string
  created_at?: number
}

export type ImportResult = {
  imported: number
  skipped: { name: string; reason: string }[]
  failed: { name: string; error: string }[]
}

export type PoolLoginProvider = 'codex' | 'claude'

export type PoolLoginSession = {
  session_id: string
  status: 'starting' | 'waiting_callback' | 'exchanging'
  url: string
  expires_in: number
  expires_at: number
}

export type PoolLoginStatus = {
  status: 'waiting_callback' | 'exchanging' | 'ok' | 'error'
  error?: string
}

/**
 * xju-api runs isolated pools (default + k12). Every management call carries the
 * target pool as `?pool=`; the backend routes it to that pool's CLIProxyAPI
 * management API. An empty pool means the primary (default) pool.
 */
function poolQuery(pool: string): string {
  return pool ? `?pool=${encodeURIComponent(pool)}` : ''
}

export async function listPools(): Promise<PoolInfo[]> {
  const res = await api.get<ApiEnvelope<PoolInfo[]>>('/api/pool/pools')
  if (!res.data.success) {
    throw new Error(res.data.message || 'Failed to load pools')
  }
  return Array.isArray(res.data.data) ? res.data.data : []
}

export async function listPoolAuthFiles(pool: string): Promise<PoolAuthFile[]> {
  const res = await api.get<ApiEnvelope<unknown>>(
    `/api/pool/auth-files${poolQuery(pool)}`
  )
  if (!res.data.success) {
    throw new Error(res.data.message || 'Failed to load pool auth files')
  }
  return normalizeList(res.data.data)
}

// A pasted/typed blob can carry one account or many (an exporter bundle or a JSON
// array), so the backend returns the same {imported, skipped, failed} report as
// the zip import.
export async function addPoolAuthFile(
  pool: string,
  args: { name: string; content: string }
): Promise<ImportResult> {
  const res = await api.post<ApiEnvelope<ImportResult>>(
    `/api/pool/auth-files${poolQuery(pool)}`,
    args
  )
  if (!res.data.success || !res.data.data) {
    throw new Error(res.data.message || 'Failed to add pool auth file')
  }
  return res.data.data
}

export async function importPoolAuthFiles(
  pool: string,
  file: File
): Promise<ImportResult> {
  const form = new FormData()
  form.append('file', file)
  const res = await api.post<ApiEnvelope<ImportResult>>(
    `/api/pool/auth-files/import${poolQuery(pool)}`,
    form
  )
  if (!res.data.success || !res.data.data) {
    throw new Error(res.data.message || 'Failed to import accounts')
  }
  return res.data.data
}

export async function startPoolLogin(
  pool: string,
  provider: PoolLoginProvider
): Promise<PoolLoginSession> {
  const res = await api.post<ApiEnvelope<PoolLoginSession>>(
    `/api/pool/oauth/${provider}/start${poolQuery(pool)}`
  )
  if (!res.data.success || !res.data.data) {
    throw new Error(res.data.message || `Failed to start ${provider} login`)
  }
  return res.data.data
}

export async function submitPoolLoginCallback(
  provider: PoolLoginProvider,
  sessionId: string,
  redirectUrl: string
): Promise<PoolLoginStatus> {
  const res = await api.post<ApiEnvelope<PoolLoginStatus>>(
    `/api/pool/oauth/${provider}/callback`,
    { session_id: sessionId, redirect_url: redirectUrl }
  )
  if (!res.data.success || !res.data.data) {
    throw new Error(res.data.message || 'Failed to submit OAuth callback')
  }
  return res.data.data
}

export async function getPoolLoginStatus(
  provider: PoolLoginProvider,
  sessionId: string
): Promise<PoolLoginStatus> {
  const res = await api.get<ApiEnvelope<PoolLoginStatus>>(
    `/api/pool/oauth/${provider}/status`,
    { params: { session_id: sessionId } }
  )
  if (!res.data.success || !res.data.data) {
    throw new Error(
      res.data.message || `Failed to check ${provider} login status`
    )
  }
  return res.data.data
}

export async function cancelPoolLogin(
  provider: PoolLoginProvider,
  sessionId: string
): Promise<void> {
  const res = await api.delete<ApiEnvelope<unknown>>(
    `/api/pool/oauth/${provider}/session`,
    { params: { session_id: sessionId } }
  )
  if (!res.data.success) {
    throw new Error(res.data.message || `Failed to cancel ${provider} login`)
  }
}

export async function deletePoolAuthFile(
  pool: string,
  name: string
): Promise<void> {
  const res = await api.delete<ApiEnvelope<unknown>>('/api/pool/auth-files', {
    params: { name, pool },
  })
  if (!res.data.success) {
    throw new Error(res.data.message || 'Failed to delete pool auth file')
  }
}

export async function setPoolAuthFileDisabled(
  pool: string,
  name: string,
  disabled: boolean
): Promise<void> {
  const res = await api.patch<ApiEnvelope<unknown>>(
    `/api/pool/auth-files/status${poolQuery(pool)}`,
    { name, disabled }
  )
  if (!res.data.success) {
    throw new Error(res.data.message || 'Failed to update account status')
  }
}

export async function cleanPoolAuthFilesNow(pool: string): Promise<number> {
  const res = await api.post<ApiEnvelope<{ disabled?: number }>>(
    `/api/pool/auth-files/clean${poolQuery(pool)}`
  )
  if (!res.data.success) {
    throw new Error(res.data.message || 'Failed to clean the pool')
  }
  return res.data.data?.disabled ?? 0
}

// xju-api:new — active verification (号池验活 Part A). Verdicts are the
// classified health of an account, produced by probing it through the pool.
export type ProbeVerdict =
  | 'online'
  | 'credential_dead'
  | 'rate_limited'
  | 'quota_exhausted'
  | 'subscription_expired'
  | 'unknown'

export type ProbeResult = {
  name: string
  auth_index: string
  verdict: ProbeVerdict
  http_code: number
  detail: string
  at: number
}

export type ProbeJobSnapshot = {
  running: boolean
  total: number
  done: number
  started_at: number
  finished_at: number
  heavy: boolean
  auto_disable: boolean
  disabled: number
  results: ProbeResult[]
  error: string
}

export async function verifyPoolAuthFile(
  pool: string,
  name: string,
  heavy: boolean
): Promise<ProbeResult> {
  const res = await api.post<ApiEnvelope<ProbeResult>>(
    `/api/pool/auth-files/verify${poolQuery(pool)}`,
    { name, heavy }
  )
  if (!res.data.success || !res.data.data) {
    throw new Error(res.data.message || 'Failed to verify account')
  }
  return res.data.data
}

export async function startVerifyAll(
  pool: string,
  opts: { heavy: boolean; autoDisable: boolean }
): Promise<ProbeJobSnapshot> {
  const res = await api.post<ApiEnvelope<ProbeJobSnapshot>>(
    `/api/pool/auth-files/verify-all${poolQuery(pool)}`,
    { heavy: opts.heavy, auto_disable: opts.autoDisable }
  )
  if (!res.data.success || !res.data.data) {
    throw new Error(res.data.message || 'Failed to start verification')
  }
  return res.data.data
}

export async function getVerifyProgress(
  pool: string
): Promise<ProbeJobSnapshot | null> {
  const res = await api.get<ApiEnvelope<ProbeJobSnapshot | null>>(
    `/api/pool/auth-files/verify-all/progress${poolQuery(pool)}`
  )
  if (!res.data.success) {
    throw new Error(res.data.message || 'Failed to load verification progress')
  }
  return res.data.data ?? null
}

// xju-api:new — per-account quota (号池额度). Snapshots are cached backend-side
// from ChatGPT's wham usage endpoint, fetched per account through cliproxy's
// pinned api-call. Reset consumes one of the account's reset credits.
export type PoolAccountUsage = {
  name: string
  auth_index: string
  plan?: string
  five_hour_used_percent?: number
  five_hour_reset_at?: number
  weekly_used_percent?: number
  weekly_reset_at?: number
  limit_reached: boolean
  reset_credits?: number
  fetched_at: number
  error?: string
}

export type PoolUsageJobSnapshot = {
  running: boolean
  total: number
  done: number
  started_at: number
  finished_at: number
  auto_reset: boolean
  // Targeted run: accounts still holding quota were skipped, not re-fetched.
  only_exhausted: boolean
  skipped: number
  resets: number
  errors: number
  error: string
}

export type PoolUsageData = {
  accounts: Record<string, PoolAccountUsage>
  job?: PoolUsageJobSnapshot | null
}

export async function getPoolUsage(pool: string): Promise<PoolUsageData> {
  const res = await api.get<ApiEnvelope<PoolUsageData>>(
    `/api/pool/auth-files/usage${poolQuery(pool)}`
  )
  if (!res.data.success || !res.data.data) {
    throw new Error(res.data.message || 'Failed to load pool quota')
  }
  return res.data.data
}

export async function refreshPoolAccountUsage(
  pool: string,
  name: string
): Promise<PoolAccountUsage> {
  const res = await api.post<ApiEnvelope<PoolAccountUsage>>(
    `/api/pool/auth-files/usage/refresh${poolQuery(pool)}`,
    { name }
  )
  if (!res.data.success || !res.data.data) {
    throw new Error(res.data.message || 'Failed to refresh quota')
  }
  return res.data.data
}

export async function startPoolUsageRefreshAll(
  pool: string
): Promise<PoolUsageJobSnapshot> {
  const res = await api.post<ApiEnvelope<PoolUsageJobSnapshot>>(
    `/api/pool/auth-files/usage/refresh${poolQuery(pool)}`,
    {}
  )
  if (!res.data.success || !res.data.data) {
    throw new Error(res.data.message || 'Failed to start quota refresh')
  }
  return res.data.data
}

export async function resetPoolAccountQuota(
  pool: string,
  name: string
): Promise<PoolAccountUsage> {
  const res = await api.post<ApiEnvelope<PoolAccountUsage>>(
    `/api/pool/auth-files/usage/reset${poolQuery(pool)}`,
    { name }
  )
  if (!res.data.success || !res.data.data) {
    throw new Error(res.data.message || 'Failed to reset quota')
  }
  return res.data.data
}

// xju-api:new — one-click pool creation (#4 Phase D). Reserved pools
// (default/k12) are env-managed; everything else is created here.
const RESERVED_POOL_IDS = new Set(['default', 'k12'])
export const isDynamicPool = (id: string) => !RESERVED_POOL_IDS.has(id)

export type PoolCreateStatus = {
  pool_id: string
  status: 'provisioning' | 'ready' | 'error'
  error?: string
}

export async function createPool(
  label: string,
  provider: 'codex' | 'claude',
  mode: 'cliproxy' | 'gopool' = 'cliproxy'
): Promise<{ pool_id: string }> {
  const res = await api.post<ApiEnvelope<{ pool_id: string; status: string }>>(
    '/api/pool/create',
    { label, provider, mode }
  )
  if (!res.data.success || !res.data.data) {
    throw new Error(res.data.message || 'Failed to create pool')
  }
  return res.data.data
}

export async function getPoolCreateStatus(
  poolId: string
): Promise<PoolCreateStatus> {
  const res = await api.get<ApiEnvelope<PoolCreateStatus>>(
    `/api/pool/create/status?id=${encodeURIComponent(poolId)}`
  )
  if (!res.data.success || !res.data.data) {
    throw new Error(res.data.message || 'Failed to check pool status')
  }
  return res.data.data
}

export async function deletePool(poolId: string): Promise<void> {
  const res = await api.post<ApiEnvelope<unknown>>('/api/pool/delete', {
    pool_id: poolId,
  })
  if (!res.data.success) {
    throw new Error(res.data.message || 'Failed to delete pool')
  }
}

// Rename a pool's display label + its routing channel's display name. The numeric
// id / container / group / card routing are untouched.
export async function renamePool(poolId: string, label: string): Promise<void> {
  const res = await api.post<ApiEnvelope<unknown>>('/api/pool/rename', {
    pool_id: poolId,
    label,
  })
  if (!res.data.success) {
    throw new Error(res.data.message || 'Failed to rename pool')
  }
}

/**
 * Derive a stable, human-legible filename from a pasted codex auth JSON. Codex
 * files carry an email/account id; use it so the pool list is readable and
 * re-pasting the same account overwrites rather than duplicating.
 */
export function deriveAuthFileName(content: string): string {
  try {
    const parsed = JSON.parse(content) as Record<string, unknown>
    const candidate =
      (typeof parsed.email === 'string' && parsed.email) ||
      (typeof parsed.account === 'string' && parsed.account) ||
      (typeof parsed.name === 'string' && parsed.name) ||
      ''
    if (candidate) {
      const slug = candidate
        .toLowerCase()
        .replaceAll(/[^a-z0-9]+/g, '-')
        .replaceAll(/^-+|-+$/g, '')
        .slice(0, 48)
      if (slug) return `codex-${slug}.json`
    }
  } catch {
    /* fall through to a timestamped default */
  }
  return `codex-${Date.now()}.json`
}
