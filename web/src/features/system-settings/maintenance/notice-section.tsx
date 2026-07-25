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
import { zodResolver } from '@hookform/resolvers/zod'
import { Save } from 'lucide-react'
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Textarea } from '@/components/ui/textarea'

import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

const noticeSchema = z.object({
  Notice: z.string().optional(),
})

type NoticeFormValues = z.infer<typeof noticeSchema>

type NoticeSectionProps = {
  defaultValue: string
  inlineActions?: boolean
  title?: string
}

export function NoticeSection({
  defaultValue,
  inlineActions = false,
  title,
}: NoticeSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const form = useForm<NoticeFormValues>({
    resolver: zodResolver(noticeSchema),
    defaultValues: {
      Notice: defaultValue ?? '',
    },
  })

  useEffect(() => {
    form.reset({ Notice: defaultValue ?? '' })
  }, [defaultValue, form])

  const onSubmit = async (values: NoticeFormValues) => {
    const normalized = values.Notice ?? ''
    try {
      const result = await updateOption.mutateAsync({
        key: 'Notice',
        value: normalized,
      })
      if (result.success) {
        form.reset({ Notice: normalized })
      }
    } catch {
      // useUpdateOption already reports the request error.
    }
  }

  return (
    <SettingsSection title={title ?? t('System Notice')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          {!inlineActions && (
            <SettingsPageFormActions
              onSave={form.handleSubmit(onSubmit)}
              isSaving={updateOption.isPending}
              isSaveDisabled={!form.formState.isDirty}
              saveLabel='Save notice'
            />
          )}
          <FormField
            control={form.control}
            name='Notice'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Announcement content')}</FormLabel>
                <FormControl>
                  <Textarea
                    rows={8}
                    placeholder={t(
                      'Planned maintenance on Friday at 22:00 UTC...'
                    )}
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Supports Markdown and sanitized HTML. No content length limit.'
                  )}
                  {' '}
                  {t(
                    'Saving this notice automatically records it in Timeline with the server publish time. Earlier notices are kept.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          {inlineActions && (
            <div data-settings-form-span='full' className='flex justify-end'>
              <Button
                type='submit'
                size='sm'
                disabled={updateOption.isPending || !form.formState.isDirty}
              >
                <Save data-icon='inline-start' />
                <span>
                  {t(updateOption.isPending ? 'Saving...' : 'Save notice')}
                </span>
              </Button>
            </div>
          )}
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
