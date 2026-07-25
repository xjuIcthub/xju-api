/*
Copyright (C) 2026 xju-api contributors

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { Link } from '@tanstack/react-router'
import {
  ArrowRight,
  BadgeDollarSign,
  BarChart3,
  BookOpenText,
  Box,
  CheckCircle2,
  Gift,
  KeyRound,
  MonitorCog,
  ShieldCheck,
  type LucideIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { SectionPageLayout } from '@/components/layout'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { DEFAULT_POOL_REDEMPTION_RATE_LABEL } from '@/lib/default-pool'

const OPENAI_ENDPOINT = 'https://api.selab.top/v1'
const CLAUDE_ENDPOINT = 'https://api.selab.top'

const COPY = {
  zh: {
    intro:
      '你可以直接使用 Default 共享号池，也可以接入自己的私人号池；选择一条路线完成后，再配置 Claude Code、Codex 或其他兼容客户端。',
    start: '推荐配置流程',
    defaultFlow: {
      title: '使用 Default 号池',
      description:
        '无需准备上游账号，兑换额度后即可使用；Default 请求会按当前倍率消耗账户额度。',
      steps: [
        {
          title: '兑换 Default 额度',
          description: `进入“额度充值”输入兑换码。当前兑换标准为 ${DEFAULT_POOL_REDEMPTION_RATE_LABEL} 额度。`,
        },
        {
          title: '创建 API 密钥',
          description: '进入“API 密钥”新建密钥，并将分组明确选择为 Default。',
        },
        {
          title: '配置客户端',
          description:
            '在 API 密钥行点击 Codex 或 CC Switch 图标，生成并复制对应配置。',
        },
        {
          title: '查看余额与用量',
          description:
            '在“额度充值”查看余额，在用量记录中查看请求消耗；余额不足时先兑换新额度。',
        },
      ],
    },
    privateFlow: {
      title: '使用私人号池',
      description:
        '使用你自己的上游账号，不检查也不扣减 Default 额度，但请求仍会正常记录用量。',
      steps: [
        {
          title: '创建私人号池',
          description: '进入“我的号池”并创建属于当前账号的隔离号池。',
        },
        {
          title: '导入并验证账号',
          description:
            '支持登录导入、CPA/Sub2 JSON、上传、粘贴和 ZIP；至少保持一个账号在线。',
        },
        {
          title: '创建 API 密钥',
          description:
            '新建密钥会默认锁定到你的私人号池，私人号池不限用户额度但仍统计用量。',
        },
        {
          title: '配置客户端',
          description:
            '在 API 密钥行点击 Codex 或 CC Switch 图标，可生成对应配置并复制。',
        },
      ],
    },
    openRecharge: '兑换 Default 额度',
    openPool: '打开我的号池',
    openKeys: '管理 API 密钥',
    endpoints: '客户端端点',
    endpointsDescription: 'API 密钥使用你在“API 密钥”页面创建的用户 Token。',
    openaiLabel: 'OpenAI / Codex 兼容端点',
    openaiNote: '适用于 Codex、Cherry Studio 和其他 OpenAI 兼容客户端。',
    claudeLabel: 'Claude Code 端点',
    claudeNote: '不要在末尾添加 /v1；Claude Code 会自动拼接 /v1/messages。',
    ccSwitch: 'CC Switch 配置',
    ccSwitchDescription:
      '在 API 密钥行点击 CC Switch Logo，默认选择 Claude 模式并生成 Deep Link 与 Config JSON。',
    modelMapping:
      '默认模型映射：Opus → gpt-5.6-sol，Sonnet → gpt-5.6-terra，Haiku → gpt-5.6-lunar。',
    security: '密钥安全',
    securityDescription:
      'API 密钥默认遮罩显示，只复制到你信任的客户端；不要截图、公开粘贴或发送给第三方。',
  },
  en: {
    intro:
      'Use the shared Default pool or connect your own private pool, then configure Claude Code, Codex, or another compatible client.',
    start: 'Recommended setup flow',
    defaultFlow: {
      title: 'Use the Default pool',
      description:
        'No upstream account is required. Redeem balance first; Default requests consume it at the current multiplier.',
      steps: [
        {
          title: 'Redeem Default balance',
          description: `Enter a code on Balance Recharge. The current conversion is ${DEFAULT_POOL_REDEMPTION_RATE_LABEL} in balance.`,
        },
        {
          title: 'Create an API key',
          description:
            'Create a key on the API Keys page and explicitly select the Default group.',
        },
        {
          title: 'Configure a client',
          description:
            'Use the Codex or CC Switch icon on the key row to generate and copy its configuration.',
        },
        {
          title: 'Track balance and usage',
          description:
            'Check remaining balance on Balance Recharge and request consumption in usage logs.',
        },
      ],
    },
    privateFlow: {
      title: 'Use a private pool',
      description:
        'Use your own upstream accounts without checking or deducting Default balance; requests remain metered.',
      steps: [
        {
          title: 'Create a private pool',
          description:
            'Open My Pool and provision an isolated pool for your account.',
        },
        {
          title: 'Import and verify accounts',
          description:
            'Use login import, CPA/Sub2 JSON, upload, paste, or ZIP, then keep at least one account online.',
        },
        {
          title: 'Create an API key',
          description:
            'New keys route to your private pool by default. Usage is unlimited by user quota but remains metered.',
        },
        {
          title: 'Configure a client',
          description:
            'Use the Codex or CC Switch icon on an API key row to generate and copy the client configuration.',
        },
      ],
    },
    openRecharge: 'Redeem Default balance',
    openPool: 'Open My Pool',
    openKeys: 'Manage API keys',
    endpoints: 'Client endpoints',
    endpointsDescription:
      'Authenticate with a user token created on the API Keys page.',
    openaiLabel: 'OpenAI / Codex compatible endpoint',
    openaiNote:
      'Use this for Codex, Cherry Studio, and other OpenAI-compatible clients.',
    claudeLabel: 'Claude Code endpoint',
    claudeNote:
      'Do not append /v1. Claude Code automatically appends /v1/messages.',
    ccSwitch: 'CC Switch setup',
    ccSwitchDescription:
      'Click the CC Switch logo on an API key row to generate a Claude preset, Deep Link, and Config JSON.',
    modelMapping:
      'Default mapping: Opus → gpt-5.6-sol, Sonnet → gpt-5.6-terra, Haiku → gpt-5.6-lunar.',
    security: 'Key security',
    securityDescription:
      'Tokens are masked by default. Copy them only into trusted clients and never share screenshots or public pastes.',
  },
} as const

const DEFAULT_STEP_ICONS = [Gift, KeyRound, MonitorCog, BarChart3] as const
const PRIVATE_STEP_ICONS = [Box, ShieldCheck, KeyRound, MonitorCog] as const

interface SetupFlowProps {
  flow: {
    title: string
    description: string
    steps: readonly {
      title: string
      description: string
    }[]
  }
  icons: readonly LucideIcon[]
  icon: LucideIcon
}

function SetupFlow({ flow, icons, icon: FlowIcon }: SetupFlowProps) {
  return (
    <div className='space-y-3'>
      <div className='flex items-start gap-3'>
        <div className='bg-primary/10 text-primary flex size-9 shrink-0 items-center justify-center rounded-lg'>
          <FlowIcon className='size-5' />
        </div>
        <div>
          <h3 className='font-serif text-base font-semibold'>{flow.title}</h3>
          <p className='text-muted-foreground mt-1 text-sm leading-5'>
            {flow.description}
          </p>
        </div>
      </div>
      <div className='grid gap-3 md:grid-cols-2 xl:grid-cols-4'>
        {flow.steps.map((step, index) => {
          const Icon = icons[index]
          return (
            <Card key={step.title} data-card-hover='false'>
              <CardHeader className='gap-3'>
                <div className='flex items-center justify-between gap-2'>
                  <div className='bg-muted flex size-9 items-center justify-center rounded-lg'>
                    <Icon className='size-5' />
                  </div>
                  <Badge variant='secondary'>{index + 1}</Badge>
                </div>
                <CardTitle className='text-base'>{step.title}</CardTitle>
                <CardDescription className='leading-5'>
                  {step.description}
                </CardDescription>
              </CardHeader>
            </Card>
          )
        })}
      </div>
    </div>
  )
}

export function TutorialDocumentation() {
  const { t, i18n } = useTranslation()
  const copy = i18n.resolvedLanguage?.startsWith('zh') ? COPY.zh : COPY.en

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Tutorial Documentation')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='mx-auto flex w-full max-w-6xl flex-col gap-5 pb-8'>
          <Card className='overflow-hidden'>
            <CardContent className='bg-muted/30 flex flex-col gap-5 p-5 sm:p-7 lg:flex-row lg:items-center lg:justify-between'>
              <div className='flex min-w-0 items-start gap-4'>
                <div className='bg-primary/10 text-primary flex size-11 shrink-0 items-center justify-center rounded-xl'>
                  <BookOpenText className='size-6' />
                </div>
                <div>
                  <h1 className='font-serif text-xl font-semibold sm:text-2xl'>
                    {t('Tutorial Documentation')}
                  </h1>
                  <p className='text-muted-foreground mt-2 max-w-2xl text-sm leading-6'>
                    {copy.intro}
                  </p>
                </div>
              </div>
              <div className='flex shrink-0 flex-wrap gap-2'>
                <Button render={<Link to='/recharge' />}>
                  {copy.openRecharge}
                  <ArrowRight />
                </Button>
                <Button variant='outline' render={<Link to='/my-pool' />}>
                  {copy.openPool}
                </Button>
                <Button variant='outline' render={<Link to='/keys' />}>
                  {copy.openKeys}
                </Button>
              </div>
            </CardContent>
          </Card>

          <section className='space-y-3'>
            <div className='flex items-center gap-2'>
              <CheckCircle2 className='text-success size-5' />
              <h2 className='font-serif text-lg font-semibold'>{copy.start}</h2>
            </div>
            <div className='space-y-6'>
              <SetupFlow
                flow={copy.defaultFlow}
                icons={DEFAULT_STEP_ICONS}
                icon={BadgeDollarSign}
              />
              <SetupFlow
                flow={copy.privateFlow}
                icons={PRIVATE_STEP_ICONS}
                icon={Box}
              />
            </div>
          </section>

          <Card data-card-hover='false'>
            <CardHeader>
              <CardTitle>{copy.endpoints}</CardTitle>
              <CardDescription>{copy.endpointsDescription}</CardDescription>
            </CardHeader>
            <CardContent className='grid gap-3'>
              {[
                {
                  label: copy.openaiLabel,
                  value: OPENAI_ENDPOINT,
                  note: copy.openaiNote,
                },
                {
                  label: copy.claudeLabel,
                  value: CLAUDE_ENDPOINT,
                  note: copy.claudeNote,
                },
              ].map((endpoint) => (
                <div
                  key={endpoint.value}
                  className='bg-muted/25 rounded-xl border p-4'
                >
                  <p className='text-sm font-medium'>{endpoint.label}</p>
                  <div className='mt-2 flex items-center gap-2'>
                    <code className='bg-background min-w-0 flex-1 overflow-x-auto rounded-lg border px-3 py-2 text-sm'>
                      {endpoint.value}
                    </code>
                    <CopyButton
                      value={endpoint.value}
                      variant='outline'
                      tooltip={t('Copy to clipboard')}
                    />
                  </div>
                  <p className='text-muted-foreground mt-2 text-xs leading-5'>
                    {endpoint.note}
                  </p>
                </div>
              ))}
            </CardContent>
          </Card>

          <div className='grid gap-4 lg:grid-cols-2'>
            <Alert>
              <MonitorCog />
              <AlertTitle>{copy.ccSwitch}</AlertTitle>
              <AlertDescription>
                <p>{copy.ccSwitchDescription}</p>
                <p>{copy.modelMapping}</p>
              </AlertDescription>
            </Alert>
            <Alert>
              <ShieldCheck />
              <AlertTitle>{copy.security}</AlertTitle>
              <AlertDescription>{copy.securityDescription}</AlertDescription>
            </Alert>
          </div>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
