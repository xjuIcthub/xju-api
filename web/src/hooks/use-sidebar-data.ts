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
import {
  Activity,
  FileText,
  Key,
  LayoutDashboard,
  Radio,
  ServerCog,
  User,
  Users,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import type { SidebarData } from '@/components/layout/types'
import { ROLE } from '@/lib/roles'
import {
  XJU_ADMIN_NAV_ITEMS,
  XJU_GENERAL_NAV_ITEMS,
  XJU_PERSONAL_NAV_ITEMS,
} from '@/registry/xju-modules'

/**
 * Root navigation groups for the application sidebar.
 *
 * These are shown when the URL does not match any nested sidebar view
 * registered in `layout/lib/sidebar-view-registry.ts`.
 */
export function useSidebarData(): SidebarData {
  const { t } = useTranslation()
  const buildXjuAdminItems = (
    placement: (typeof XJU_ADMIN_NAV_ITEMS)[number]['placement']
  ) =>
    XJU_ADMIN_NAV_ITEMS.filter((item) => item.placement === placement).map(
      (item) => ({
        title: t(item.titleKey),
        url: item.url,
        icon: item.icon,
        requiredRole: item.requiredRole,
      })
    )

  // xju-api:prune (PLAN.md §5.2): the chat group (Playground + Chat presets),
  // Wallet, Redemption Codes and Subscriptions entries are removed — the
  // day-card platform keeps only issuing, stats and admin surfaces.
  return {
    navGroups: [
      {
        id: 'general',
        title: t('General'),
        items: [
          {
            title: t('Overview'),
            url: '/dashboard/overview',
            icon: Activity,
          },
          {
            title: t('Dashboard'),
            url: '/dashboard/models',
            icon: LayoutDashboard,
          },
          {
            title: t('API Keys'),
            url: '/keys',
            icon: Key,
          },
          {
            title: t('Usage Logs'),
            url: '/usage-logs/common',
            icon: FileText,
          },
          ...XJU_GENERAL_NAV_ITEMS.map((item) => ({
            ...item,
            title: t(item.titleKey),
          })),
        ],
      },
      {
        id: 'personal',
        title: t('Personal'),
        items: [
          {
            title: t('Profile'),
            url: '/profile',
            icon: User,
          },
          ...XJU_PERSONAL_NAV_ITEMS.map((item) => ({
            ...item,
            title: t(item.titleKey),
          })),
        ],
      },
      {
        id: 'admin',
        title: t('Admin'),
        items: [
          {
            title: t('Channels'),
            url: '/channels',
            icon: Radio,
          },
          // xju-api:inject — 自有 admin 侧栏项由注册中心收敛(registry/xju-modules.ts)
          ...buildXjuAdminItems('before-users'),
          {
            title: t('Users'),
            url: '/users',
            icon: Users,
          },
          ...buildXjuAdminItems('after-users'),
          {
            title: t('System Info'),
            url: '/system-info',
            icon: ServerCog,
            requiredRole: ROLE.SUPER_ADMIN,
          },
        ],
      },
    ],
  }
}
