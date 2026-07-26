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
import { Claude } from '@lobehub/icons'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Activity,
  ClipboardPaste,
  FileArchive,
  Gauge,
  Loader2,
  Pencil,
  Play,
  Plus,
  Power,
  RefreshCw,
  RotateCcw,
  ShieldCheck,
  Sparkles,
  Trash2,
  Upload,
} from 'lucide-react'
import { useEffect, useRef, useState, type ChangeEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { IconCodex } from '@/assets/custom/icon-codex'
import { Dialog } from '@/components/dialog'
import { SectionPageLayout } from '@/components/layout'
import { StatusBadge } from '@/components/status-badge'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Combobox } from '@/components/ui/combobox'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import {
  addPoolAuthFile,
  cancelPoolLogin,
  cleanPoolAuthFilesNow,
  createPool,
  deletePool,
  deletePoolAuthFile,
  deriveAuthFileName,
  getPoolLoginStatus,
  getPoolCreateStatus,
  getPoolUsage,
  getVerifyProgress,
  importPoolAuthFiles,
  isDynamicPool,
  listPoolAuthFiles,
  listPools,
  refreshPoolAccountUsage,
  renamePool,
  resetPoolAccountQuota,
  setPoolAuthFileDisabled,
  startPoolLogin,
  startPoolUsageRefreshAll,
  startVerifyAll,
  submitPoolLoginCallback,
  verifyPoolAuthFile,
  type ImportResult,
  type PoolAuthFile,
  type PoolInfo,
  type ProbeResult,
} from '@/features/pool/api'
import { CodexLoginButton } from '@/features/pool/codex-login-button'
import {
  accountState,
  cooldownLabel,
  poolStats,
  quotaPercentClass,
  recentActivity,
  STATE_META,
  subscriptionUntil,
  VERDICT_META,
  verdictBreakdown,
} from '@/features/pool/workbench-utils'
import { useStatus } from '@/hooks/use-status'
import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

function poolDisplayLabel(pool: PoolInfo): string {
  if (pool.kind !== 'private') return pool.label
  return `@${pool.owner_username || pool.owner_user_id || pool.id}`
}

function PoolProviderLogo({
  provider,
  className,
}: {
  provider: PoolInfo['provider']
  className?: string
}) {
  if (provider === 'claude') {
    return <Claude.Color className={className} aria-label='Claude' />
  }
  return <IconCodex className={className} aria-label='Codex / GPT / OpenAI' />
}

type NewPoolType = 'cpa-codex' | 'cpa-claude' | 'gopool'

export function Pool() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { status } = useStatus()
  const role = useAuthStore((state) => state.auth.user?.role ?? ROLE.GUEST)
  const canManage = role >= ROLE.ADMIN
  const [content, setContent] = useState('')
  const [pool, setPool] = useState('default')
  const [importResult, setImportResult] = useState<ImportResult | null>(null)
  // xju-api:new — per-account verify verdicts (keyed by file name) + verify-all options.
  const [verdicts, setVerdicts] = useState<Record<string, ProbeResult>>({})
  const [heavyProbe, setHeavyProbe] = useState(false)
  const [autoDisable, setAutoDisable] = useState(false)
  // xju-api:new — one-click pool creation + deletion (#4 Phase D).
  const [createOpen, setCreateOpen] = useState(false)
  const [newLabel, setNewLabel] = useState('')
  const [newPoolType, setNewPoolType] = useState<NewPoolType>('cpa-codex')
  const [creatingId, setCreatingId] = useState<string | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<PoolInfo | null>(null)
  // xju-api:new — pool rename (display label + its channel display name).
  const [renameTarget, setRenameTarget] = useState<PoolInfo | null>(null)
  const [renameLabel, setRenameLabel] = useState('')
  const fileInputRef = useRef<HTMLInputElement>(null)
  const zipInputRef = useRef<HTMLInputElement>(null)

  const autoCleanEnabled = Boolean(status?.pool_auto_clean_enabled)
  const autoCleanHours = Number(status?.pool_auto_clean_hours ?? 24)
  const usageAutoRefreshEnabled = Boolean(
    status?.pool_usage_auto_refresh_enabled
  )
  const usageAutoResetEnabled = Boolean(status?.pool_usage_auto_reset_enabled)

  const poolsQuery = useQuery({
    queryKey: ['pool', 'pools'],
    queryFn: listPools,
    staleTime: 60_000,
  })
  const pools: PoolInfo[] = poolsQuery.data ?? [
    { id: 'default', label: 'Default', provider: 'codex' },
  ]
  // xju-api:new — dual build modes (③). The active pool's build_mode only
  // steers UI guidance (import copy); the backend treats both modes alike.
  const activeBuildMode =
    pools.find((p) => p.id === pool)?.build_mode ?? 'cliproxy'
  const activePool = pools.find((p) => p.id === pool)
  const activeProvider = activePool?.provider === 'claude' ? 'claude' : 'codex'
  const isCodexPool = activeProvider === 'codex'
  // xju-api:new — the initial selection ('default') is only a pre-load
  // placeholder; the env-seeded default/k12 pools can be retired in favour of
  // dynamic pools, so 'default' may not exist. Once the real list loads, snap
  // the selection to the first pool whenever the current id isn't among them
  // (first paint, or after the active pool is deleted) so a pool is always
  // selected instead of spinning on a non-existent one.
  useEffect(() => {
    const list = poolsQuery.data
    if (!list || list.length === 0) return
    if (!list.some((p) => p.id === pool)) setPool(list[0].id)
  }, [poolsQuery.data, pool])

  // Every pool-scoped query must wait until the selection is a real configured
  // pool. The transient 'default' placeholder (before the auto-select effect
  // corrects it) 503s, and the global response interceptor turns that 503 into
  // an error toast ("pool management is not configured for pool: default").
  const poolConfigured = (poolsQuery.data ?? []).some((p) => p.id === pool)

  const listQuery = useQuery({
    queryKey: ['pool', 'auth-files', pool],
    queryFn: () => listPoolAuthFiles(pool),
    staleTime: 10_000,
    enabled: poolConfigured,
  })

  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: ['pool', 'auth-files', pool] })

  const addMutation = useMutation({
    mutationFn: async () => {
      const trimmed = content.trim()
      if (!trimmed) throw new Error(t('Paste an auth JSON first'))
      try {
        JSON.parse(trimmed)
      } catch {
        throw new Error(t('That is not valid JSON'))
      }
      return addPoolAuthFile(pool, {
        name: deriveAuthFileName(trimmed),
        content: trimmed,
      })
    },
    onSuccess: async (result) => {
      // The blob may have held many accounts (a bundle / array). Show the same
      // imported/skipped/failed breakdown the zip import uses when it wasn't a
      // plain single add.
      const multi =
        result.imported > 1 ||
        result.skipped.length > 0 ||
        result.failed.length > 0
      if (multi) {
        setImportResult(result)
        toast.success(
          t('Imported {{imported}} · skipped {{skipped}} · failed {{failed}}', {
            imported: result.imported,
            skipped: result.skipped.length,
            failed: result.failed.length,
          })
        )
      } else {
        toast.success(t('Account added to the pool'))
      }
      setContent('')
      await invalidate()
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const deleteMutation = useMutation({
    mutationFn: (name: string) => deletePoolAuthFile(pool, name),
    onSuccess: async () => {
      toast.success(t('Account removed'))
      await invalidate()
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const toggleMutation = useMutation({
    mutationFn: (args: { name: string; disabled: boolean }) =>
      setPoolAuthFileDisabled(pool, args.name, args.disabled),
    onSuccess: async () => await invalidate(),
    onError: (error: Error) => toast.error(error.message),
  })

  const importMutation = useMutation({
    mutationFn: (file: File) => importPoolAuthFiles(pool, file),
    onSuccess: async (result) => {
      setImportResult(result)
      toast.success(
        t('Imported {{imported}} · skipped {{skipped}} · failed {{failed}}', {
          imported: result.imported,
          skipped: result.skipped.length,
          failed: result.failed.length,
        })
      )
      await invalidate()
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const cleanMutation = useMutation({
    mutationFn: () => cleanPoolAuthFilesNow(pool),
    onSuccess: async (disabled) => {
      toast.success(
        disabled > 0
          ? t('Disabled {{count}} stale account(s)', { count: disabled })
          : t('Nothing to clean — all accounts are healthy')
      )
      await invalidate()
    },
    onError: (error: Error) => toast.error(error.message),
  })

  // Shared by every pool-related option switch (auto-clean / quota auto-refresh
  // / quota auto-reset) — they all write one option key and refresh /status.
  const optionMutation = useMutation({
    mutationFn: (args: { key: string; value: string }) =>
      api.put('/api/pool/settings', args),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['status'] })
      toast.success(t('Saved successfully'))
    },
    onError: () => toast.error(t('Save failed')),
  })

  // xju-api:new — per-account quota (号池额度): cached snapshots + refresh job.
  const usageQuery = useQuery({
    queryKey: ['pool', 'usage', pool],
    queryFn: () => getPoolUsage(pool),
    enabled: poolConfigured && isCodexPool,
    // Poll while a whole-pool refresh is running; otherwise the cache is stable.
    refetchInterval: (query) => (query.state.data?.job?.running ? 3000 : false),
  })
  const usageByName = usageQuery.data?.accounts ?? {}
  const usageJob = usageQuery.data?.job ?? null
  const invalidateUsage = () =>
    queryClient.invalidateQueries({ queryKey: ['pool', 'usage', pool] })

  const refreshUsageMutation = useMutation({
    mutationFn: (name: string) => refreshPoolAccountUsage(pool, name),
    onSuccess: async () => await invalidateUsage(),
    onError: (error: Error) => toast.error(error.message),
  })
  const refreshingUsageName = refreshUsageMutation.isPending
    ? refreshUsageMutation.variables
    : null

  const refreshAllUsageMutation = useMutation({
    mutationFn: () => startPoolUsageRefreshAll(pool),
    onSuccess: async () => await invalidateUsage(),
    onError: (error: Error) => toast.error(error.message),
  })

  const [resetQuotaTarget, setResetQuotaTarget] = useState<PoolAuthFile | null>(
    null
  )
  const resetQuotaMutation = useMutation({
    mutationFn: (name: string) => resetPoolAccountQuota(pool, name),
    onSuccess: async () => {
      setResetQuotaTarget(null)
      toast.success(t('Quota reset — usage windows renewed'))
      await invalidateUsage()
    },
    onError: (error: Error) => toast.error(error.message),
  })

  // xju-api:new — single-account verify (report-only, never changes state).
  const verifyMutation = useMutation({
    mutationFn: (name: string) =>
      verifyPoolAuthFile(pool, name, canManage && heavyProbe),
    onSuccess: (result) =>
      setVerdicts((prev) => ({ ...prev, [result.name]: result })),
    onError: (error: Error) => toast.error(error.message),
  })

  // xju-api:new — verify-all: kick off the background job, then poll progress.
  const verifyAllMutation = useMutation({
    mutationFn: () => startVerifyAll(pool, { heavy: heavyProbe, autoDisable }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['pool', 'verify', pool] })
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const progressQuery = useQuery({
    queryKey: ['pool', 'verify', pool],
    queryFn: () => getVerifyProgress(pool),
    enabled: canManage && poolConfigured && isCodexPool,
    // Poll while a run is in flight; otherwise leave the last snapshot in place.
    refetchInterval: (query) => (query.state.data?.running ? 2000 : false),
  })
  const progress = progressQuery.data ?? null

  // Fold the job's per-account results into the same verdict map the row badges
  // read, and refresh the list once when a run finishes (auto-disable may have
  // changed account state).
  const jobRunning = progress?.running ?? false
  const jobResultCount = progress?.results?.length ?? 0
  const prevRunning = useRef(false)
  useEffect(() => {
    if (!progress?.results?.length) return
    setVerdicts((prev) => {
      const next = { ...prev }
      for (const r of progress.results) next[r.name] = r
      return next
    })
  }, [progress?.results, jobResultCount])
  useEffect(() => {
    if (prevRunning.current && !jobRunning) {
      queryClient.invalidateQueries({
        queryKey: ['pool', 'auth-files', pool],
      })
    }
    prevRunning.current = jobRunning
  }, [jobRunning, pool, queryClient])

  // xju-api:new — create a pool, then poll provisioning until it's ready and
  // switch to its tab.
  const createMutation = useMutation({
    mutationFn: () => {
      const provider = newPoolType === 'cpa-claude' ? 'claude' : 'codex'
      const mode = newPoolType === 'gopool' ? 'gopool' : 'cliproxy'
      return createPool(newLabel.trim(), provider, mode)
    },
    onSuccess: (res) => setCreatingId(res.pool_id),
    onError: (error: Error) => toast.error(error.message),
  })
  const createStatusQuery = useQuery({
    queryKey: ['pool', 'create-status', creatingId],
    queryFn: () => getPoolCreateStatus(creatingId as string),
    enabled: !!creatingId,
    refetchInterval: (query) =>
      query.state.data?.status === 'provisioning' ? 2000 : false,
  })
  const createStatus = createStatusQuery.data?.status
  const createError = createStatusQuery.data?.error
  useEffect(() => {
    if (!creatingId || !createStatus) return
    if (createStatus === 'ready') {
      const id = creatingId
      setCreatingId(null)
      setCreateOpen(false)
      setNewLabel('')
      setNewPoolType('cpa-codex')
      queryClient.invalidateQueries({ queryKey: ['pool', 'pools'] })
      setPool(id)
      toast.success(t('Pool created — now import accounts into it'))
    } else if (createStatus === 'error') {
      setCreatingId(null)
      toast.error(createError || t('Pool provisioning failed'))
    }
  }, [createStatus, createError, creatingId, queryClient, t])

  const deletePoolMutation = useMutation({
    mutationFn: (id: string) => deletePool(id),
    onSuccess: async (_data, id) => {
      setDeleteTarget(null)
      // Fall back to the first remaining pool; the auto-select effect is the
      // safety net once the refreshed list arrives.
      if (pool === id) setPool(pools.find((p) => p.id !== id)?.id ?? '')
      await queryClient.invalidateQueries({ queryKey: ['pool', 'pools'] })
      toast.success(t('Pool deleted'))
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const renamePoolMutation = useMutation({
    mutationFn: (args: { id: string; label: string }) =>
      renamePool(args.id, args.label),
    onSuccess: async () => {
      setRenameTarget(null)
      await queryClient.invalidateQueries({ queryKey: ['pool', 'pools'] })
      toast.success(t('Pool renamed'))
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const handlePaste = async () => {
    try {
      const text = await navigator.clipboard.readText()
      if (text.trim()) setContent(text)
    } catch {
      toast.error(t('Clipboard not available — paste manually'))
    }
  }

  const handleFileUpload = async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    // Reset so selecting the same file again still fires onChange.
    event.target.value = ''
    if (!file) return
    try {
      const text = await file.text()
      if (text.trim()) setContent(text)
    } catch {
      toast.error(t('Could not read that file'))
    }
  }

  const handleZipImport = (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    // Reset so selecting the same file again still fires onChange.
    event.target.value = ''
    if (!file) return
    setImportResult(null)
    importMutation.mutate(file)
  }

  const files = listQuery.data ?? []
  // The account list is "loading" both while the pool list itself resolves (the
  // gated auth-files query is disabled until then) and while that query runs.
  const listLoading = poolsQuery.isLoading || listQuery.isLoading
  const listReady = !listLoading && !listQuery.isError
  const stats = poolStats(files, verdicts)
  const verifyingName = verifyMutation.isPending
    ? verifyMutation.variables
    : null
  let accessLabel = t('Read-only')
  if (canManage) accessLabel = t('Admin')
  if (role >= ROLE.SUPER_ADMIN) accessLabel = t('Root')
  let poolDescription = t(
    'Upstream Codex / GPT / OpenAI accounts behind the shared pool.'
  )
  if (activeProvider === 'claude') {
    poolDescription = t('Upstream Claude accounts behind the shared pool.')
  }
  if (activePool?.kind === 'private') {
    const owner = activePool.owner_username
      ? `@${activePool.owner_username}`
      : `#${activePool.owner_user_id}`
    poolDescription = t('Private pool owned by {{owner}}.', { owner })
  }
  let addAccountDescription = t(
    'Enriched login → paste the codex auth JSON. Bulk .zip import also works. The pool reloads instantly.'
  )
  if (activeBuildMode === 'gopool') {
    addAccountDescription = t(
      'Bulk import a .zip of many accounts, or paste a single codex auth JSON. The pool reloads instantly.'
    )
  }
  if (activeProvider === 'claude') {
    addAccountDescription = t(
      'Claude accounts are imported only through the secure Anthropic OAuth login.'
    )
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        <span className='inline-flex min-w-0 items-center gap-2'>
          <PoolProviderLogo
            provider={activeProvider}
            className='size-5 shrink-0'
          />
          <span className='truncate'>{t('All Pools')}</span>
          <Badge variant='outline' className='shrink-0'>
            {accessLabel}
          </Badge>
        </span>
      </SectionPageLayout.Title>
      {/* xju-api:edit — the searchable pool switcher + create action share the
          title row. Private pools use their concise @username identity. */}
      <SectionPageLayout.Actions>
        {pools.length > 1 && (
          <Combobox
            options={pools.map((item) => ({
              value: item.id,
              label: poolDisplayLabel(item),
              icon: (
                <PoolProviderLogo provider={item.provider} className='size-4' />
              ),
            }))}
            value={pool}
            onValueChange={(value) => {
              if (!value) return
              setPool(value)
              setImportResult(null)
            }}
            placeholder={t('Search pools...')}
            emptyText={t('No results found')}
            className='w-48 sm:w-60'
          />
        )}
        {canManage && (
          <Button
            type='button'
            variant='outline'
            size='sm'
            onClick={() => setCreateOpen(true)}
          >
            <Plus className='size-4' />
            {t('New pool')}
          </Button>
        )}
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        {!canManage && (
          <div className='border-border bg-muted/40 text-muted-foreground mb-4 rounded-md border px-4 py-3 text-sm'>
            {activeProvider === 'claude'
              ? t(
                  'Shared pools are read-only for regular users. Claude pools expose account status only; use My Pool to manage your own Codex accounts.'
                )
              : t(
                  'Shared pools are read-only for regular users. You can only test account availability and check quota; use My Pool to manage your own accounts.'
                )}
          </div>
        )}
        <div
          className={
            canManage
              ? 'grid gap-4 xl:grid-cols-[minmax(0,1fr)_22rem]'
              : 'grid gap-4'
          }
        >
          {/* Accounts list */}
          <Card data-card-hover='false'>
            <CardHeader className='flex flex-row items-start justify-between gap-3 space-y-0'>
              <div className='min-w-0'>
                <CardTitle className='text-base'>
                  {t('Accounts in pool')}
                </CardTitle>
                <CardDescription>{poolDescription}</CardDescription>
              </div>
              {/* xju-api:edit — stats + actions live on the card's right,
                  balancing the title/description on the left. Stats are three
                  orthogonal dimensions: total / enabled (operator toggle) /
                  online (health), no longer conflated. */}
              <div className='flex shrink-0 flex-col items-end gap-2'>
                {files.length > 0 && (
                  <div className='flex flex-wrap items-center justify-end gap-x-3 gap-y-0.5 text-xs'>
                    <span className='text-muted-foreground'>
                      <span className='text-foreground font-semibold'>
                        {stats.total}
                      </span>{' '}
                      {t('Total')}
                    </span>
                    <span className='text-muted-foreground'>
                      <span className='text-foreground font-semibold'>
                        {stats.enabled}
                      </span>{' '}
                      {t('Enabled')}
                    </span>
                    <span className='text-success'>
                      <span className='font-semibold'>{stats.online}</span>{' '}
                      {t('Online')}
                    </span>
                  </div>
                )}
                <div className='flex items-center gap-2'>
                  {/* xju-api:new — rename a dynamically-created pool (display
                      label + its channel display name). */}
                  {canManage && isDynamicPool(pool) && (
                    <Button
                      type='button'
                      variant='outline'
                      size='sm'
                      onClick={() => {
                        const p = pools.find((x) => x.id === pool)
                        setRenameLabel(p?.label ?? pool)
                        setRenameTarget(p ?? { id: pool, label: pool })
                      }}
                    >
                      <Pencil className='size-4' />
                      {t('Rename')}
                    </Button>
                  )}
                  {/* xju-api:new — delete a dynamically-created pool; the
                      env-seeded default/k12 pools cannot be removed here. */}
                  {canManage && isDynamicPool(pool) && (
                    <Button
                      type='button'
                      variant='outline'
                      size='sm'
                      className='text-destructive hover:text-destructive'
                      onClick={() =>
                        setDeleteTarget(
                          pools.find((p) => p.id === pool) ?? {
                            id: pool,
                            label: pool,
                          }
                        )
                      }
                    >
                      <Trash2 className='size-4' />
                      {t('Delete pool')}
                    </Button>
                  )}
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    onClick={() => invalidate()}
                    disabled={listQuery.isFetching}
                  >
                    <RefreshCw
                      className={listQuery.isFetching ? 'animate-spin' : ''}
                    />
                    {t('Refresh')}
                  </Button>
                </div>
              </div>
            </CardHeader>
            <CardContent>
              <div className='border-border overflow-hidden rounded-md border'>
                {listLoading && (
                  <div className='text-muted-foreground flex items-center gap-2 p-4 text-sm'>
                    <Loader2 className='size-4 animate-spin' />
                    {t('Loading...')}
                  </div>
                )}
                {!listLoading && listQuery.isError && (
                  <div className='text-destructive p-4 text-sm'>
                    {(listQuery.error as Error).message}
                  </div>
                )}
                {listReady && files.length === 0 && (
                  <div className='text-muted-foreground p-4 text-sm'>
                    {t('No accounts yet.')}
                  </div>
                )}
                {listReady && files.length > 0 && (
                  <ul className='divide-border divide-y'>
                    {files.map((file) => {
                      const state = accountState(file)
                      const meta = STATE_META[state]
                      const label = file.email || file.account || file.name
                      const plan = file.id_token?.plan_type
                      const subUntil = subscriptionUntil(file)
                      const activity = recentActivity(file)
                      const cooldown = cooldownLabel(file)
                      const usage = usageByName[file.name]
                      const verdict = verdicts[file.name]
                      const verdictMeta = verdict
                        ? VERDICT_META[verdict.verdict]
                        : null
                      return (
                        <li
                          key={file.name}
                          className='hover:bg-muted flex items-center justify-between gap-3 px-3 py-2.5 transition-colors'
                        >
                          <div className='min-w-0'>
                            <div className='flex items-center gap-2'>
                              <span className='truncate text-sm font-medium'>
                                {label}
                              </span>
                              <StatusBadge
                                label={t(meta.labelKey)}
                                variant={meta.variant}
                                copyable={false}
                              />
                              {isCodexPool && plan && (
                                <Badge
                                  variant='outline'
                                  className='shrink-0 uppercase'
                                >
                                  {plan}
                                </Badge>
                              )}
                              {verdictMeta && (
                                <StatusBadge
                                  label={`✓ ${t(verdictMeta.labelKey)}`}
                                  variant={verdictMeta.variant}
                                  copyable={false}
                                />
                              )}
                            </div>
                            <p className='text-muted-foreground truncate font-mono text-xs'>
                              {file.name}
                            </p>
                            <div className='text-muted-foreground mt-0.5 flex flex-wrap items-center gap-x-2.5 gap-y-0.5 text-xs'>
                              {isCodexPool && subUntil && (
                                <span
                                  className={
                                    state === 'expired'
                                      ? 'text-destructive'
                                      : ''
                                  }
                                >
                                  {state === 'expired'
                                    ? t('Expired {{date}}', {
                                        date: subUntil.toLocaleDateString(),
                                      })
                                    : t('Subscription until {{date}}', {
                                        date: subUntil.toLocaleDateString(),
                                      })}
                                </span>
                              )}
                              {activity && (
                                <span
                                  className={
                                    activity.rate < 100 ? 'text-warning' : ''
                                  }
                                >
                                  {t('{{rate}}% ok · {{total}} recent', {
                                    rate: activity.rate,
                                    total: activity.total,
                                  })}
                                </span>
                              )}
                              {cooldown !== null && (
                                <span className='text-warning'>
                                  {t('cooldown · retries in {{time}}', {
                                    time: cooldown,
                                  })}
                                </span>
                              )}
                              {isCodexPool && usage && !usage.error && (
                                <>
                                  {usage.five_hour_used_percent != null && (
                                    <span
                                      className={quotaPercentClass(
                                        usage.five_hour_used_percent
                                      )}
                                    >
                                      {t('5h {{percent}}%', {
                                        percent: Math.round(
                                          usage.five_hour_used_percent
                                        ),
                                      })}
                                    </span>
                                  )}
                                  {usage.weekly_used_percent != null && (
                                    <span
                                      className={quotaPercentClass(
                                        usage.weekly_used_percent
                                      )}
                                    >
                                      {t('Wk {{percent}}%', {
                                        percent: Math.round(
                                          usage.weekly_used_percent
                                        ),
                                      })}
                                    </span>
                                  )}
                                  {usage.limit_reached && (
                                    <span className='text-destructive'>
                                      {t('Quota exhausted')}
                                    </span>
                                  )}
                                  {(usage.reset_credits ?? 0) > 0 && (
                                    <span className='text-info'>
                                      {t('Reset credits: {{count}}', {
                                        count: usage.reset_credits,
                                      })}
                                    </span>
                                  )}
                                </>
                              )}
                              {file.status_message && (
                                <span
                                  className='truncate'
                                  title={file.status_message}
                                >
                                  {file.status_message}
                                </span>
                              )}
                            </div>
                          </div>
                          <div className='flex shrink-0 items-center gap-1'>
                            <Button
                              type='button'
                              variant='ghost'
                              size='icon-sm'
                              aria-label={t('Verify')}
                              title={t('Verify now')}
                              onClick={() => verifyMutation.mutate(file.name)}
                              disabled={verifyMutation.isPending}
                            >
                              {verifyingName === file.name ? (
                                <Loader2 className='size-4 animate-spin' />
                              ) : (
                                <Activity className='size-4' />
                              )}
                            </Button>
                            {isCodexPool && (
                              <Button
                                type='button'
                                variant='ghost'
                                size='icon-sm'
                                aria-label={t('Refresh quota')}
                                title={t('Refresh quota')}
                                onClick={() =>
                                  refreshUsageMutation.mutate(file.name)
                                }
                                disabled={refreshUsageMutation.isPending}
                              >
                                {refreshingUsageName === file.name ? (
                                  <Loader2 className='size-4 animate-spin' />
                                ) : (
                                  <Gauge className='size-4' />
                                )}
                              </Button>
                            )}
                            {isCodexPool &&
                              canManage &&
                              (usage?.reset_credits ?? 0) > 0 && (
                                <Button
                                  type='button'
                                  variant='ghost'
                                  size='icon-sm'
                                  aria-label={t('Reset quota')}
                                  title={t('Reset quota')}
                                  onClick={() => setResetQuotaTarget(file)}
                                  disabled={resetQuotaMutation.isPending}
                                >
                                  <RotateCcw className='size-4' />
                                </Button>
                              )}
                            <Button
                              type='button'
                              variant='ghost'
                              size='icon-sm'
                              aria-label={
                                file.disabled ? t('Enable') : t('Disable')
                              }
                              title={file.disabled ? t('Enable') : t('Disable')}
                              className={cn(
                                !canManage && 'hidden',
                                file.disabled && 'text-success'
                              )}
                              onClick={() =>
                                toggleMutation.mutate({
                                  name: file.name,
                                  disabled: !file.disabled,
                                })
                              }
                              disabled={toggleMutation.isPending}
                            >
                              <Power className='size-4' />
                            </Button>
                            <Button
                              type='button'
                              variant='ghost'
                              size='icon-sm'
                              className={cn(
                                'text-destructive hover:text-destructive',
                                !canManage && 'hidden'
                              )}
                              aria-label={t('Remove')}
                              title={t('Remove')}
                              onClick={() => deleteMutation.mutate(file.name)}
                              disabled={deleteMutation.isPending}
                            >
                              <Trash2 className='size-4' />
                            </Button>
                          </div>
                        </li>
                      )
                    })}
                  </ul>
                )}
              </div>
            </CardContent>
          </Card>

          {/* Right column: add + auto-clean */}
          <div className={canManage ? 'grid content-start gap-4' : 'hidden'}>
            <Card data-card-hover='false'>
              <CardHeader>
                <CardTitle className='text-base'>{t('Add account')}</CardTitle>
                <CardDescription>{addAccountDescription}</CardDescription>
              </CardHeader>
              <CardContent className='grid gap-2'>
                <div className='flex flex-wrap justify-end gap-2'>
                  {isCodexPool && (
                    <>
                      <input
                        ref={zipInputRef}
                        type='file'
                        accept='.zip'
                        className='hidden'
                        onChange={handleZipImport}
                      />
                      <input
                        ref={fileInputRef}
                        type='file'
                        accept='.json,application/json'
                        className='hidden'
                        onChange={handleFileUpload}
                      />
                    </>
                  )}
                  <CodexLoginButton
                    key={`${pool}-${activeProvider}`}
                    provider={activeProvider}
                    scopeKey={['root', pool]}
                    startLogin={() => startPoolLogin(pool, activeProvider)}
                    submitCallback={(sessionId, redirectUrl) =>
                      submitPoolLoginCallback(
                        activeProvider,
                        sessionId,
                        redirectUrl
                      )
                    }
                    getStatus={(sessionId) =>
                      getPoolLoginStatus(activeProvider, sessionId)
                    }
                    cancelLogin={(sessionId) =>
                      cancelPoolLogin(activeProvider, sessionId)
                    }
                    onComplete={invalidate}
                    disabled={!poolConfigured}
                  />
                  {isCodexPool && (
                    <>
                      <Button
                        type='button'
                        variant='outline'
                        size='sm'
                        onClick={() => zipInputRef.current?.click()}
                        disabled={importMutation.isPending}
                      >
                        {importMutation.isPending ? (
                          <Loader2 className='animate-spin' />
                        ) : (
                          <FileArchive />
                        )}
                        {t('Import .zip')}
                      </Button>
                      <Button
                        type='button'
                        variant='outline'
                        size='sm'
                        onClick={() => fileInputRef.current?.click()}
                      >
                        <Upload />
                        {t('Upload')}
                      </Button>
                      <Button
                        type='button'
                        variant='outline'
                        size='sm'
                        onClick={handlePaste}
                      >
                        <ClipboardPaste />
                        {t('Paste')}
                      </Button>
                    </>
                  )}
                </div>
                {isCodexPool && (
                  <>
                    <Textarea
                      value={content}
                      onChange={(event) => setContent(event.target.value)}
                      placeholder='{ "email": "...", "OPENAI_API_KEY": "..." }'
                      className='h-36 font-mono text-xs'
                      spellCheck={false}
                    />
                    <Button
                      type='button'
                      onClick={() => addMutation.mutate()}
                      disabled={addMutation.isPending || !content.trim()}
                    >
                      {addMutation.isPending ? (
                        <Loader2 className='animate-spin' />
                      ) : (
                        <Plus />
                      )}
                      {t('Add to pool')}
                    </Button>
                  </>
                )}
                {isCodexPool && importResult && (
                  <div className='border-border mt-1 rounded-md border p-2 text-xs'>
                    <p className='font-medium'>
                      {t(
                        'Imported {{imported}} · skipped {{skipped}} · failed {{failed}}',
                        {
                          imported: importResult.imported,
                          skipped: importResult.skipped.length,
                          failed: importResult.failed.length,
                        }
                      )}
                    </p>
                    {importResult.failed.length > 0 && (
                      <ul className='text-destructive mt-1 max-h-24 overflow-auto'>
                        {importResult.failed.map((f) => (
                          <li key={f.name} className='truncate font-mono'>
                            {f.name}: {f.error}
                          </li>
                        ))}
                      </ul>
                    )}
                    {importResult.skipped.length > 0 && (
                      <ul className='text-muted-foreground mt-1 max-h-24 overflow-auto'>
                        {importResult.skipped.map((s) => (
                          <li key={s.name} className='truncate font-mono'>
                            {s.name}: {s.reason}
                          </li>
                        ))}
                      </ul>
                    )}
                  </div>
                )}
              </CardContent>
            </Card>

            <Card data-card-hover='false'>
              <CardHeader>
                <CardTitle className='flex items-center gap-1.5 text-base'>
                  <Sparkles className='size-4' />
                  {t('Auto-clean')}
                </CardTitle>
                <CardDescription>
                  {t(
                    'Hourly: disable accounts that stay unavailable past {{hours}}h.',
                    { hours: autoCleanHours }
                  )}
                </CardDescription>
              </CardHeader>
              <CardContent className='grid gap-3'>
                <div className='flex items-center justify-between'>
                  <span className='text-sm font-medium'>
                    {t('Enable auto-clean')}
                  </span>
                  <Switch
                    checked={autoCleanEnabled}
                    disabled={optionMutation.isPending}
                    onCheckedChange={(v) =>
                      optionMutation.mutate({
                        key: 'PoolAutoCleanEnabled',
                        value: String(v),
                      })
                    }
                  />
                </div>
                <Button
                  type='button'
                  variant='outline'
                  onClick={() => cleanMutation.mutate()}
                  disabled={cleanMutation.isPending}
                >
                  {cleanMutation.isPending ? (
                    <Loader2 className='animate-spin' />
                  ) : (
                    <Play />
                  )}
                  {t('Clean now')}
                </Button>
              </CardContent>
            </Card>

            {/* xju-api:new — per-account quota (号池额度): whole-pool refresh +
                auto-refresh / auto-reset toggles. */}
            {isCodexPool && (
              <Card data-card-hover='false'>
                <CardHeader>
                  <CardTitle className='flex items-center gap-1.5 text-base'>
                    <Gauge className='size-4' />
                    {t('Account quota')}
                  </CardTitle>
                  <CardDescription>
                    {t(
                      'Per-account ChatGPT usage windows (5h / weekly) and reset credits.'
                    )}
                  </CardDescription>
                </CardHeader>
                <CardContent className='grid gap-3'>
                  <div className='flex items-start justify-between gap-3'>
                    <div className='min-w-0'>
                      <span className='text-sm font-medium'>
                        {t('Auto refresh hourly')}
                      </span>
                      <p className='text-muted-foreground text-xs'>
                        {t(
                          'Fetch every account’s quota in the background, once an hour.'
                        )}
                      </p>
                    </div>
                    <Switch
                      checked={usageAutoRefreshEnabled}
                      disabled={optionMutation.isPending}
                      onCheckedChange={(v) =>
                        optionMutation.mutate({
                          key: 'PoolUsageAutoRefreshEnabled',
                          value: String(v),
                        })
                      }
                    />
                  </div>
                  <div className='flex items-start justify-between gap-3'>
                    <div className='min-w-0'>
                      <span className='text-sm font-medium'>
                        {t('Auto reset when exhausted')}
                      </span>
                      <p className='text-muted-foreground text-xs'>
                        {t(
                          'Spend one reset credit automatically when an account runs out of quota.'
                        )}
                      </p>
                    </div>
                    <Switch
                      checked={usageAutoResetEnabled}
                      disabled={optionMutation.isPending}
                      onCheckedChange={(v) =>
                        optionMutation.mutate({
                          key: 'PoolUsageAutoResetEnabled',
                          value: String(v),
                        })
                      }
                    />
                  </div>
                  <Button
                    type='button'
                    onClick={() => refreshAllUsageMutation.mutate()}
                    disabled={
                      refreshAllUsageMutation.isPending ||
                      Boolean(usageJob?.running)
                    }
                  >
                    {usageJob?.running ? (
                      <Loader2 className='animate-spin' />
                    ) : (
                      <Gauge />
                    )}
                    {t('Refresh all quota')}
                  </Button>
                  <p className='text-muted-foreground text-xs'>
                    {t(
                      'Only exhausted or unknown accounts are fetched; accounts with quota left are skipped.'
                    )}
                  </p>
                  {usageJob && (usageJob.running || usageJob.done > 0) && (
                    <div className='border-border rounded-md border p-2 text-xs'>
                      {usageJob.running ? (
                        <p>
                          {t('Refreshing quota {{done}}/{{total}}...', {
                            done: usageJob.done,
                            total: usageJob.total,
                          })}
                        </p>
                      ) : (
                        <p className='font-medium'>
                          {t(
                            'Quota refreshed {{total}} · skipped {{skipped}} · auto-reset {{resets}} · failed {{errors}}',
                            {
                              total: usageJob.total,
                              skipped: usageJob.skipped,
                              resets: usageJob.resets,
                              errors: usageJob.errors,
                            }
                          )}
                        </p>
                      )}
                    </div>
                  )}
                </CardContent>
              </Card>
            )}

            {/* xju-api:new — active verification (号池验活 Part A) */}
            {isCodexPool && (
              <Card data-card-hover='false'>
                <CardHeader>
                  <CardTitle className='flex items-center gap-1.5 text-base'>
                    <ShieldCheck className='size-4' />
                    {t('Verify accounts')}
                  </CardTitle>
                  <CardDescription>
                    {t(
                      'Probe each account live to confirm it is actually online, instead of trusting the passive status.'
                    )}
                  </CardDescription>
                </CardHeader>
                <CardContent className='grid gap-3'>
                  <div className='flex items-start justify-between gap-3'>
                    <div className='min-w-0'>
                      <span className='text-sm font-medium'>
                        {t('Deep probe')}
                      </span>
                      <p className='text-muted-foreground text-xs'>
                        {t(
                          'Also run a tiny inference to catch quota-exhausted accounts (uses a little quota).'
                        )}
                      </p>
                    </div>
                    <Switch
                      checked={heavyProbe}
                      onCheckedChange={setHeavyProbe}
                      disabled={jobRunning}
                    />
                  </div>
                  <div className='flex items-start justify-between gap-3'>
                    <div className='min-w-0'>
                      <span className='text-sm font-medium'>
                        {t('Auto-disable dead')}
                      </span>
                      <p className='text-muted-foreground text-xs'>
                        {t(
                          'Disable accounts found credential-dead or subscription-expired.'
                        )}
                      </p>
                    </div>
                    <Switch
                      checked={autoDisable}
                      onCheckedChange={setAutoDisable}
                      disabled={jobRunning}
                    />
                  </div>
                  <Button
                    type='button'
                    onClick={() => verifyAllMutation.mutate()}
                    disabled={verifyAllMutation.isPending || jobRunning}
                  >
                    {jobRunning ? (
                      <Loader2 className='animate-spin' />
                    ) : (
                      <ShieldCheck />
                    )}
                    {t('Verify all')}
                  </Button>
                  {progress && (progress.running || progress.done > 0) && (
                    <div className='border-border rounded-md border p-2 text-xs'>
                      {progress.running ? (
                        <p>
                          {t('Verifying {{done}}/{{total}}...', {
                            done: progress.done,
                            total: progress.total,
                          })}
                        </p>
                      ) : (
                        <p className='font-medium'>
                          {t('Verified {{total}} · disabled {{disabled}}', {
                            total: progress.total,
                            disabled: progress.disabled,
                          })}
                        </p>
                      )}
                      <div className='text-muted-foreground mt-1 flex flex-wrap gap-x-2.5 gap-y-0.5'>
                        {verdictBreakdown(progress.results ?? []).map(
                          ([verdict, count]) => (
                            <span key={verdict}>
                              {t(VERDICT_META[verdict].labelKey)}: {count}
                            </span>
                          )
                        )}
                      </div>
                    </div>
                  )}
                </CardContent>
              </Card>
            )}
          </div>
        </div>

        {/* xju-api:new — create pool dialog (#4 Phase D). Only a name is needed;
            everything else clones the existing pools. Provisioning runs on the
            host, so the dialog polls until the new pool is ready. */}
        <Dialog
          open={createOpen}
          onOpenChange={(o) => {
            if (!creatingId) {
              setCreateOpen(o)
              if (!o) {
                setNewLabel('')
                setNewPoolType('cpa-codex')
              }
            }
          }}
          title={t('New pool')}
          description={t(
            'Spin up a new isolated account pool. Only a name is required — it gets its own upstream instance and routing channel, then you import accounts into it.'
          )}
          contentClassName='max-w-md'
          bodyClassName='space-y-3'
        >
          <div className='grid gap-1'>
            <label className='text-muted-foreground text-xs'>{t('Name')}</label>
            <Input
              autoFocus
              value={newLabel}
              placeholder={t('e.g. Campus, Trial, Team B')}
              disabled={!!creatingId}
              onChange={(e) => setNewLabel(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && newLabel.trim() && !creatingId) {
                  createMutation.mutate()
                }
              }}
            />
          </div>
          <div className='grid gap-1'>
            <label className='text-muted-foreground text-xs'>
              {t('Pool type')}
            </label>
            <div className='grid grid-cols-1 gap-2 sm:grid-cols-3'>
              <Button
                type='button'
                variant={newPoolType === 'cpa-codex' ? 'default' : 'outline'}
                disabled={!!creatingId}
                onClick={() => setNewPoolType('cpa-codex')}
              >
                {t('CPA (Codex)')}
              </Button>
              <Button
                type='button'
                variant={newPoolType === 'cpa-claude' ? 'default' : 'outline'}
                disabled={!!creatingId}
                onClick={() => setNewPoolType('cpa-claude')}
              >
                {t('CPA (Claude)')}
              </Button>
              <Button
                type='button'
                variant={newPoolType === 'gopool' ? 'default' : 'outline'}
                disabled={!!creatingId}
                onClick={() => setNewPoolType('gopool')}
              >
                {t('go-pool')}
              </Button>
            </div>
          </div>
          {creatingId && (
            <div className='text-muted-foreground flex items-center gap-2 text-sm'>
              <Loader2 className='size-4 animate-spin' />
              {t('Provisioning {{id}}...', { id: creatingId })}
            </div>
          )}
          <div className='flex justify-end gap-2'>
            <Button
              type='button'
              onClick={() => createMutation.mutate()}
              disabled={
                !newLabel.trim() || createMutation.isPending || !!creatingId
              }
            >
              {createMutation.isPending || creatingId ? (
                <Loader2 className='animate-spin' />
              ) : (
                <Plus />
              )}
              {t('Create')}
            </Button>
          </div>
        </Dialog>

        {/* xju-api:new — rename pool dialog. Only the display label + channel
            display name change; the numeric id and card routing are untouched. */}
        <Dialog
          open={!!renameTarget}
          onOpenChange={(o) => {
            if (!o) setRenameTarget(null)
          }}
          title={t('Rename pool')}
          description={t(
            'Rename this pool, its channel, and its routing group. Cards already issued in the old group are migrated to the new name so they keep working.'
          )}
          contentClassName='max-w-md'
          bodyClassName='space-y-3'
        >
          <div className='grid gap-1'>
            <label className='text-muted-foreground text-xs'>
              {t('New name')}
            </label>
            <Input
              autoFocus
              value={renameLabel}
              placeholder={t('e.g. Campus, Trial, Team B')}
              onChange={(e) => setRenameLabel(e.target.value)}
              onKeyDown={(e) => {
                if (
                  e.key === 'Enter' &&
                  renameLabel.trim() &&
                  renameTarget &&
                  !renamePoolMutation.isPending
                ) {
                  renamePoolMutation.mutate({
                    id: renameTarget.id,
                    label: renameLabel.trim(),
                  })
                }
              }}
            />
          </div>
          <div className='flex justify-end gap-2'>
            <Button
              type='button'
              variant='outline'
              onClick={() => setRenameTarget(null)}
              disabled={renamePoolMutation.isPending}
            >
              {t('Cancel')}
            </Button>
            <Button
              type='button'
              onClick={() => {
                if (renameTarget) {
                  renamePoolMutation.mutate({
                    id: renameTarget.id,
                    label: renameLabel.trim(),
                  })
                }
              }}
              disabled={!renameLabel.trim() || renamePoolMutation.isPending}
            >
              {renamePoolMutation.isPending && (
                <Loader2 className='animate-spin' />
              )}
              {t('Save')}
            </Button>
          </div>
        </Dialog>

        {/* xju-api:new — quota reset confirm: a reset credit is a scarce,
            irreversible resource, so it never fires from a single click. */}
        <AlertDialog
          open={!!resetQuotaTarget}
          onOpenChange={(o) => {
            if (!o) setResetQuotaTarget(null)
          }}
        >
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>
                {t('Reset quota for "{{label}}"?', {
                  label: resetQuotaTarget
                    ? resetQuotaTarget.email ||
                      resetQuotaTarget.account ||
                      resetQuotaTarget.name
                    : '',
                })}
              </AlertDialogTitle>
              <AlertDialogDescription>
                {t(
                  'This consumes one of the account’s reset credits to renew its usage windows. A used credit cannot be restored.'
                )}
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel disabled={resetQuotaMutation.isPending}>
                {t('Cancel')}
              </AlertDialogCancel>
              <AlertDialogAction
                disabled={resetQuotaMutation.isPending}
                onClick={(e) => {
                  e.preventDefault()
                  if (resetQuotaTarget) {
                    resetQuotaMutation.mutate(resetQuotaTarget.name)
                  }
                }}
              >
                {resetQuotaMutation.isPending && (
                  <Loader2 className='animate-spin' />
                )}
                {t('Reset quota')}
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>

        {/* xju-api:new — delete pool confirm (#4 Phase D). */}
        <AlertDialog
          open={!!deleteTarget}
          onOpenChange={(o) => {
            if (!o) setDeleteTarget(null)
          }}
        >
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>
                {t('Delete pool "{{label}}"?', {
                  label: deleteTarget?.label ?? '',
                })}
              </AlertDialogTitle>
              <AlertDialogDescription>
                {t(
                  'This stops and removes the pool’s upstream instance and its routing channel. The account files are kept on the server. This cannot be undone from here.'
                )}
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel disabled={deletePoolMutation.isPending}>
                {t('Cancel')}
              </AlertDialogCancel>
              <AlertDialogAction
                className='bg-destructive hover:bg-destructive/90 text-white'
                disabled={deletePoolMutation.isPending}
                onClick={(e) => {
                  e.preventDefault()
                  if (deleteTarget) deletePoolMutation.mutate(deleteTarget.id)
                }}
              >
                {deletePoolMutation.isPending && (
                  <Loader2 className='animate-spin' />
                )}
                {t('Delete pool')}
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
