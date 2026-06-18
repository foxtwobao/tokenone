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
import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { formatLogQuota, formatTokens } from '@/lib/format'
import { cn } from '@/lib/utils'
import { useIsAdmin } from '@/hooks/use-admin'
import { Skeleton } from '@/components/ui/skeleton'
import {
  StaticDataTable,
  type StaticDataTableColumn,
} from '@/components/data-table'
import { staticDataTableClassNames } from '@/components/data-table/static/static-data-table-classnames'
import { getDailyUsageSummary, getUserDailyUsageSummary } from '../api'
import { buildApiParams } from '../lib/utils'
import type { UsageSummaryItem, UsageSummaryTotals } from '../types'
import { useUsageLogsContext } from './usage-logs-provider'

const route = getRouteApi('/_authenticated/usage-logs/$section')

const EMPTY_TOTAL: UsageSummaryTotals = {
  request_count: 0,
  input_tokens: 0,
  cache_tokens: 0,
  output_tokens: 0,
  total_tokens: 0,
  quota: 0,
}

function SummaryMetric(props: {
  label: string
  value: string | number
  className?: string
}) {
  return (
    <div className='border-border/60 bg-muted/20 flex min-h-14 min-w-0 flex-col justify-center rounded-md border px-3 py-2'>
      <span className='text-muted-foreground text-xs'>{props.label}</span>
      <span
        className={cn(
          'text-foreground/90 truncate font-mono text-sm font-semibold tabular-nums',
          props.className
        )}
      >
        {props.value}
      </span>
    </div>
  )
}

function formatVisible(
  visible: boolean,
  value: number,
  formatter: (value: number) => string
) {
  return visible ? formatter(value) : '••••'
}

export function DailyUsageSummary() {
  const { t } = useTranslation()
  const isAdmin = useIsAdmin()
  const searchParams = route.useSearch()
  const { sensitiveVisible } = useUsageLogsContext()

  const { data, isLoading, isFetching } = useQuery({
    queryKey: ['daily-usage-summary', isAdmin, searchParams],
    queryFn: async () => {
      const params = buildApiParams({
        page: 1,
        pageSize: 1,
        searchParams,
        columnFilters: [],
        isAdmin,
      })
      const result = isAdmin
        ? await getDailyUsageSummary(params)
        : await getUserDailyUsageSummary(params)
      if (!result.success) {
        toast.error(result.message || t('Failed to load usage summary'))
        return { total: EMPTY_TOTAL, items: [] }
      }
      return result.data || { total: EMPTY_TOTAL, items: [] }
    },
    placeholderData: (previousData) => previousData,
  })

  const columns = useMemo<StaticDataTableColumn<UsageSummaryItem>[]>(
    () => [
      {
        id: 'name',
        header: isAdmin ? t('Account') : t('API Key'),
        className: staticDataTableClassNames.compactHeaderCell,
        cellClassName: staticDataTableClassNames.compactCell,
        cell: (row) => {
          const label = isAdmin
            ? row.username || `#${row.user_id || 0}`
            : row.token_name || `#${row.token_id || 0}`
          return (
            <span className='text-foreground/90 font-medium'>{label}</span>
          )
        },
      },
      {
        id: 'request_count',
        header: t('Requests'),
        className: staticDataTableClassNames.compactHeaderCellRight,
        cellClassName: staticDataTableClassNames.compactNumericCell,
        cell: (row) =>
          sensitiveVisible ? row.request_count.toLocaleString() : '••••',
      },
      {
        id: 'input_tokens',
        header: t('Input Tokens'),
        className: staticDataTableClassNames.compactHeaderCellRight,
        cellClassName: staticDataTableClassNames.compactMutedNumericCell,
        cell: (row) =>
          formatVisible(sensitiveVisible, row.input_tokens, formatTokens),
      },
      {
        id: 'cache_tokens',
        header: t('Cache'),
        className: staticDataTableClassNames.compactHeaderCellRight,
        cellClassName: staticDataTableClassNames.compactMutedNumericCell,
        cell: (row) =>
          formatVisible(sensitiveVisible, row.cache_tokens, formatTokens),
      },
      {
        id: 'output_tokens',
        header: t('Output Tokens'),
        className: staticDataTableClassNames.compactHeaderCellRight,
        cellClassName: staticDataTableClassNames.compactMutedNumericCell,
        cell: (row) =>
          formatVisible(sensitiveVisible, row.output_tokens, formatTokens),
      },
      {
        id: 'quota',
        header: t('Cost'),
        className: staticDataTableClassNames.compactHeaderCellRight,
        cellClassName: staticDataTableClassNames.compactNumericCell,
        cell: (row) =>
          formatVisible(sensitiveVisible, row.quota, formatLogQuota),
      },
    ],
    [isAdmin, sensitiveVisible, t]
  )

  const summary = data ?? { total: EMPTY_TOTAL, items: [] }
  const loading = isLoading || (isFetching && !data)

  if (loading) {
    return (
      <div className='border-border/70 bg-card/50 rounded-lg border p-3'>
        <div className='mb-3 flex flex-wrap items-center justify-between gap-2'>
          <Skeleton className='h-5 w-40' />
          <div className='grid w-full grid-cols-3 gap-2 sm:w-auto sm:min-w-96'>
            <Skeleton className='h-14' />
            <Skeleton className='h-14' />
            <Skeleton className='h-14' />
          </div>
        </div>
        <Skeleton className='h-40 w-full' />
      </div>
    )
  }

  return (
    <div className='border-border/70 bg-card/50 rounded-lg border p-3'>
      <div className='mb-3 flex flex-wrap items-start justify-between gap-3'>
        <div className='min-w-0'>
          <h3 className='text-foreground text-sm font-semibold'>
            {t('Daily Usage Summary')}
          </h3>
        </div>
        <div className='grid w-full grid-cols-1 gap-2 sm:w-auto sm:min-w-[28rem] sm:grid-cols-3'>
          <SummaryMetric
            label={t('Requests')}
            value={
              sensitiveVisible
                ? summary.total.request_count.toLocaleString()
                : '••••'
            }
          />
          <SummaryMetric
            label={t('Tokens')}
            value={formatVisible(
              sensitiveVisible,
              summary.total.total_tokens,
              formatTokens
            )}
          />
          <SummaryMetric
            label={t('Cost')}
            value={formatVisible(
              sensitiveVisible,
              summary.total.quota,
              formatLogQuota
            )}
          />
        </div>
      </div>

      <StaticDataTable
        columns={columns}
        data={summary.items}
        className='overflow-x-auto'
        tableClassName='min-w-[720px] text-[13px]'
        headerRowClassName={staticDataTableClassNames.mutedHeaderRow}
        emptyContent={
          <span className='text-muted-foreground text-sm'>
            {t('No usage summary available for the selected range.')}
          </span>
        }
        getRowKey={(row, index) =>
          isAdmin ? `user-${row.user_id || index}` : `token-${row.token_id || index}`
        }
      />
    </div>
  )
}
