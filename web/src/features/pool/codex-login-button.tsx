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
import { useMutation, useQuery } from '@tanstack/react-query'
import {
  CheckCircle2,
  CircleAlert,
  ClipboardPaste,
  ExternalLink,
  Loader2,
  LogIn,
} from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'

export type CodexLoginSession = {
  session_id: string
  status: 'starting' | 'waiting_callback' | 'exchanging'
  url: string
  expires_in: number
  expires_at: number
}

export type CodexLoginStatus = {
  status: 'waiting_callback' | 'exchanging' | 'ok' | 'error'
  error?: string
}

type CodexLoginButtonProps = {
  provider?: 'codex' | 'claude'
  scopeKey: readonly (string | number)[]
  startLogin: () => Promise<CodexLoginSession>
  submitCallback: (
    sessionId: string,
    redirectUrl: string
  ) => Promise<CodexLoginStatus>
  getStatus: (sessionId: string) => Promise<CodexLoginStatus>
  cancelLogin: (sessionId: string) => Promise<void>
  onComplete: () => void | Promise<void>
  disabled?: boolean
}

// xju-api:new — one browser OAuth workflow shared by the root pool workbench
// and the owner-scoped private-pool workbench.
export function CodexLoginButton({
  provider = 'codex',
  scopeKey,
  startLogin,
  submitCallback,
  getStatus,
  cancelLogin,
  onComplete,
  disabled = false,
}: CodexLoginButtonProps) {
  const { t } = useTranslation()
  const providerLabel = provider === 'claude' ? 'Claude' : 'OpenAI'
  const callbackURLPlaceholder =
    provider === 'claude'
      ? 'http://localhost:54545/callback?code=...&state=...'
      : 'http://localhost:1455/auth/callback?code=...&state=...'
  const [open, setOpen] = useState(false)
  const [session, setSession] = useState<CodexLoginSession | null>(null)
  const [callbackURL, setCallbackURL] = useState('')
  const [completed, setCompleted] = useState(false)
  const openRef = useRef(false)
  const sessionRef = useRef<CodexLoginSession | null>(null)
  const completedRef = useRef(false)
  const onCompleteRef = useRef(onComplete)

  useEffect(() => {
    sessionRef.current = session
  }, [session])
  useEffect(() => {
    completedRef.current = completed
  }, [completed])
  useEffect(() => {
    onCompleteRef.current = onComplete
  }, [onComplete])
  useEffect(
    () => () => {
      const active = sessionRef.current
      if (active && !completedRef.current) {
        void cancelLogin(active.session_id)
      }
    },
    [cancelLogin]
  )

  const statusQuery = useQuery({
    queryKey: ['pool-oauth', provider, ...scopeKey, session?.session_id],
    queryFn: () => {
      if (!session) throw new Error(t('Login session expired'))
      return getStatus(session.session_id)
    },
    enabled: Boolean(session && !completed),
    refetchInterval: (query) => {
      const status = query.state.data?.status
      return status === 'ok' || status === 'error' ? false : 2000
    },
    retry: false,
  })

  const startMutation = useMutation({
    mutationFn: startLogin,
    onSuccess: (nextSession) => {
      if (!openRef.current) {
        void cancelLogin(nextSession.session_id)
        return
      }
      setSession(nextSession)
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const callbackMutation = useMutation({
    mutationFn: () => {
      if (!session) throw new Error(t('Login session expired'))
      return submitCallback(session.session_id, callbackURL.trim())
    },
    onSuccess: () => void statusQuery.refetch(),
    onError: (error: Error) => toast.error(error.message),
  })

  useEffect(() => {
    const result = statusQuery.data
    if (!result || completed) return
    if (result.status === 'ok') {
      completedRef.current = true
      setCompleted(true)
      toast.success(t('Account added to the pool'))
      void onCompleteRef.current()
    } else if (result.status === 'error') {
      completedRef.current = true
      setCompleted(true)
      toast.error(result.error || t('Authentication failed'))
    }
  }, [completed, statusQuery.data, t])

  const openDialog = () => {
    openRef.current = true
    setOpen(true)
    setSession(null)
    sessionRef.current = null
    setCallbackURL('')
    setCompleted(false)
    completedRef.current = false
    startMutation.reset()
    callbackMutation.reset()
    startMutation.mutate()
  }

  const closeDialog = () => {
    openRef.current = false
    if (session && !completed) void cancelLogin(session.session_id)
    setOpen(false)
    setSession(null)
    sessionRef.current = null
    setCallbackURL('')
    setCompleted(false)
    completedRef.current = false
  }

  let statusMessage = t('Waiting for the localhost callback URL...')
  if (callbackMutation.isSuccess) {
    statusMessage = t('Exchanging the authorization code...')
  }
  if (completed) {
    statusMessage =
      statusQuery.data?.status === 'error'
        ? statusQuery.data.error || t('Authentication failed')
        : t('Account added. The account list has been refreshed.')
  }
  const pollError = statusQuery.isError ? statusQuery.error.message : ''
  if (pollError) statusMessage = pollError
  const failed =
    (completed && statusQuery.data?.status === 'error') || Boolean(pollError)
  let statusIcon = <Loader2 className='size-4 animate-spin' />
  if (completed || pollError) {
    statusIcon = failed ? (
      <CircleAlert className='text-destructive size-4' />
    ) : (
      <CheckCircle2 className='text-success size-4' />
    )
  }

  return (
    <>
      <Button
        type='button'
        variant='outline'
        size='sm'
        onClick={openDialog}
        disabled={disabled}
      >
        <LogIn />
        {t('Login')}
      </Button>

      <Dialog
        open={open}
        onOpenChange={(nextOpen) => {
          if (!nextOpen) closeDialog()
        }}
        title={t('Login with {{provider}}', { provider: providerLabel })}
        description={t(
          'Credentials and MFA stay with {{provider}}. This page only receives the one-time localhost callback.',
          { provider: providerLabel }
        )}
        contentClassName='max-w-xl'
        bodyClassName='space-y-4'
      >
        {startMutation.isPending && (
          <div className='text-muted-foreground flex items-center gap-2 text-sm'>
            <Loader2 className='size-4 animate-spin' />
            {t('Preparing a secure login session...')}
          </div>
        )}
        {startMutation.isError && !session && (
          <div className='flex justify-end'>
            <Button type='button' onClick={closeDialog}>
              {t('Close')}
            </Button>
          </div>
        )}
        {session && (
          <>
            <div className='border-border rounded-md border p-3'>
              <p className='text-sm font-medium'>
                1.{' '}
                {t('Open the {{provider}} login page', {
                  provider: providerLabel,
                })}
              </p>
              <p className='text-muted-foreground mt-1 text-xs'>
                {t(
                  'Complete login in the new tab. It will automatically redirect to the localhost callback.'
                )}
              </p>
              <Button
                className='mt-3'
                type='button'
                variant='outline'
                onClick={() =>
                  window.open(session.url, '_blank', 'noopener,noreferrer')
                }
              >
                <ExternalLink />
                {t('Open {{provider}} login', { provider: providerLabel })}
              </Button>
            </div>
            <div className='border-border rounded-md border p-3'>
              <p className='text-sm font-medium'>
                2. {t('Copy the localhost callback URL')}
              </p>
              <p className='text-muted-foreground mt-1 text-xs'>
                {t(
                  'A “site cannot be reached” page is expected. Press Ctrl+L, Ctrl+C and paste the complete address below.'
                )}
              </p>
              <Textarea
                className='mt-3 min-h-24 font-mono text-xs'
                value={callbackURL}
                onChange={(event) => setCallbackURL(event.target.value)}
                placeholder={callbackURLPlaceholder}
                spellCheck={false}
              />
              <div className='mt-2 flex flex-wrap gap-2'>
                <Button
                  type='button'
                  variant='outline'
                  onClick={async () => {
                    try {
                      setCallbackURL(await navigator.clipboard.readText())
                    } catch {
                      toast.error(t('Clipboard not available — paste manually'))
                    }
                  }}
                >
                  <ClipboardPaste />
                  {t('Paste')}
                </Button>
                <Button
                  type='button'
                  onClick={() => callbackMutation.mutate()}
                  disabled={
                    !callbackURL.trim() ||
                    callbackMutation.isPending ||
                    completed
                  }
                >
                  {callbackMutation.isPending ? (
                    <Loader2 className='animate-spin' />
                  ) : (
                    <LogIn />
                  )}
                  {t('Complete login')}
                </Button>
              </div>
            </div>
            <div className='border-border rounded-md border p-3 text-sm'>
              <p className='font-medium'>3. {t('Import status')}</p>
              <div className='text-muted-foreground mt-2 flex items-center gap-2'>
                {statusIcon}
                {statusMessage}
              </div>
            </div>
            <div className='flex justify-end'>
              <Button type='button' onClick={closeDialog}>
                {completed ? t('Done') : t('Cancel')}
              </Button>
            </div>
          </>
        )}
      </Dialog>
    </>
  )
}
