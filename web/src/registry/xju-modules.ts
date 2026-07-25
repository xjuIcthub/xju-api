/*
Copyright (C) 2026 xju-api contributors

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import {
  BadgeDollarSign,
  BookOpenText,
  Box,
  Boxes,
  Megaphone,
  SlidersHorizontal,
} from 'lucide-react'

import { ROLE } from '@/lib/roles'

// xju-api:new — 自有模块注册中心(REFACTOR-PLAN §5.1 注册反转)。
//
// 自有侧栏项 / 模块开关键 / URL→配置映射 / 模块元数据全部收敛在此;
// 上游共享文件(use-sidebar-data.ts / use-sidebar-config.ts /
// maintenance/sidebar-modules-section.tsx)只 import + 泛型 merge,
// 不再写死任何 xju 专有字面量。新增自有页面时只改本文件与路由。

/** 侧栏 general 组注入项。 */
export const XJU_GENERAL_NAV_ITEMS = [
  {
    titleKey: 'Tutorial Documentation',
    url: '/docs' as const,
    icon: BookOpenText,
  },
]

/** 侧栏 personal 组注入项。私人号池属于当前账号，而非全局控制台。 */
export const XJU_PERSONAL_NAV_ITEMS = [
  {
    titleKey: 'My Pool',
    url: '/my-pool' as const,
    icon: Box,
  },
  {
    titleKey: 'Balance Recharge',
    url: '/recharge' as const,
    icon: BadgeDollarSign,
  },
]

/** 侧栏 admin 组注入项(use-sidebar-data.ts 消费;title 在消费点过 t())。 */
export const XJU_ADMIN_NAV_ITEMS = [
  {
    titleKey: 'Account Pool',
    url: '/pool' as const,
    icon: Boxes,
    requiredRole: ROLE.SUPER_ADMIN,
    placement: 'before-users' as const,
  },
  {
    titleKey: 'Default Pool Pricing',
    url: '/default-pricing' as const,
    icon: SlidersHorizontal,
    requiredRole: ROLE.SUPER_ADMIN,
    placement: 'before-users' as const,
  },
  {
    titleKey: 'Announcement Publishing',
    url: '/announcements' as const,
    icon: Megaphone,
    requiredRole: ROLE.SUPER_ADMIN,
    placement: 'after-users' as const,
  },
]

/** 侧栏模块开关默认值(merge 进 DEFAULT_SIDEBAR_MODULES 对应 section)。 */
export const XJU_SIDEBAR_MODULE_DEFAULTS: Record<
  string,
  Record<string, boolean>
> = {
  console: { docs: true },
  personal: { private_pool: true, recharge: true },
  admin: { pool: true, default_pricing: true, announcements: true },
}

/** URL → 配置键映射(merge 进 URL_TO_CONFIG_MAP)。 */
export const XJU_URL_TO_CONFIG: Record<
  string,
  { section: string; module: string }
> = {
  '/docs': { section: 'console', module: 'docs' },
  '/my-pool': { section: 'personal', module: 'private_pool' },
  '/recharge': { section: 'personal', module: 'recharge' },
  '/pool': { section: 'admin', module: 'pool' },
  '/default-pricing': { section: 'admin', module: 'default_pricing' },
  '/announcements': {
    section: 'admin',
    module: 'announcements',
  },
}

/** 管理端「侧栏模块」开关面板的标题/描述元数据(消费点过 t())。 */
export const XJU_SIDEBAR_MODULE_META: Record<
  string,
  Record<string, { titleKey: string; descriptionKey: string }>
> = {
  console: {
    docs: {
      titleKey: 'Tutorial Documentation',
      descriptionKey:
        'Step-by-step guides for Default and private pools, API keys, and client setup.',
    },
  },
  personal: {
    private_pool: {
      titleKey: 'My Pool',
      descriptionKey: 'Manage the upstream accounts in your private pool.',
    },
    recharge: {
      titleKey: 'Balance Recharge',
      descriptionKey: 'Recharge the balance used by the Default shared pool.',
    },
  },
  admin: {
    pool: {
      titleKey: 'Account Pool',
      descriptionKey: 'Manage upstream codex accounts in the shared pool.',
    },
    default_pricing: {
      titleKey: 'Default Pool Pricing',
      descriptionKey: 'Adjust the usage price multiplier for the Default pool.',
    },
    announcements: {
      titleKey: 'Announcement Publishing',
      descriptionKey: 'Publish and manage platform announcements.',
    },
  },
}
