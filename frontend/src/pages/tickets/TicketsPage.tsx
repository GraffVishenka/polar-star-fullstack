/**
 * `/tickets` — доска заявок в 4 колонки (Ф2).
 *
 * Права (TICKET-7): USER видит только своё здание, фильтр здания заблокирован;
 * ADMIN — все здания и режим «Все здания». Пагинация и счётчики — серверные
 * (`total` каждой колонки), фильтры хранятся в URL.
 */
import { useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { PlusOutlined } from '@ant-design/icons';
import { Alert, Button, Col, Flex, Row } from 'antd';
import { fetchAdminUsers } from '@/api/admin';
import { fetchProblemTypesForSelection } from '@/api/dictionaries';
import { fetchBuildings } from '@/api/equipment';
import { TICKET_STATUS_ORDER } from '@/app/config';
import { queryKeys } from '@/app/queryClient';
import { useAuth } from '@/auth/AuthContext';
import { PageHeader } from '@/components/PageHeader';
import { TicketColumn } from './components/TicketColumn';
import { TicketCreateModal } from './components/TicketCreateModal';
import { TicketDrawer } from './components/TicketDrawer';
import { TicketFiltersBar } from './components/TicketFiltersBar';
import { needsBuildingAssignment } from './components/ticketPermissions';
import { useTicketFilters } from './hooks/useTicketFilters';
import { useTicketStatusMeta } from './hooks/useTicketStatusMeta';

export function TicketsPage() {
  const { user, isAdmin } = useAuth();
  const { filters, apiParams, hasActiveFilters, setFilter, resetFilters } = useTicketFilters();
  const { meta, statuses } = useTicketStatusMeta();

  const noBuilding = needsBuildingAssignment(user);

  const buildingsQuery = useQuery({
    queryKey: queryKeys.buildings,
    queryFn: () => fetchBuildings(),
    staleTime: 5 * 60_000,
  });

  const problemTypesQuery = useQuery({
    queryKey: ['dictionaries', 'problem-types'],
    queryFn: fetchProblemTypesForSelection,
    staleTime: 5 * 60_000,
    enabled: !noBuilding,
  });

  const executorsQuery = useQuery({
    queryKey: queryKeys.adminUsers({ size: 100, is_active: true }),
    queryFn: () => fetchAdminUsers({ size: 100, is_active: true }),
    enabled: isAdmin && !noBuilding,
    staleTime: 60_000,
  });

  const [openTicketId, setOpenTicketId] = useState<number | null>(null);
  const [createOpen, setCreateOpen] = useState(false);

  // Ключ кэша колонок: меняется только вместе с фильтрами.
  const filtersKey = useMemo(() => JSON.stringify(apiParams), [apiParams]);

  if (noBuilding) {
    return (
      <Flex vertical gap={16} data-testid="tickets-page">
        <PageHeader title="Заявки" />
        <Alert
          type="warning"
          showIcon
          message="Здание не назначено, обратитесь к администратору"
          description="Пока сотруднику не назначено здание, список заявок и создание недоступны (AUTH-9, 409 BUILDING_NOT_ASSIGNED)."
          data-testid="building-not-assigned"
        />
      </Flex>
    );
  }

  return (
    <Flex vertical gap={16} data-testid="tickets-page">
      <PageHeader
        title="Заявки"
        description={
          isAdmin
            ? 'Все здания · доска по 4 статусам, просрочка — по данным сервера'
            : `Заявки здания «${user?.building_name ?? '—'}» · ограничение проверяет сервер (TICKET-7)`
        }
        extra={
          <Button
            type="primary"
            icon={<PlusOutlined />}
            onClick={() => setCreateOpen(true)}
            data-testid="open-create"
          >
            Создать заявку
          </Button>
        }
      />

      <TicketFiltersBar
        filters={filters}
        statuses={statuses}
        problemTypes={problemTypesQuery.data ?? []}
        executors={executorsQuery.data?.items ?? []}
        buildings={(buildingsQuery.data ?? []).map((building) => ({
          id: building.id,
          name: building.name,
        }))}
        isAdmin={isAdmin}
        buildingsLoading={buildingsQuery.isLoading}
        executorsLoading={executorsQuery.isLoading}
        hasActiveFilters={hasActiveFilters}
        onChange={setFilter}
        onReset={resetFilters}
        userFullName={user?.full_name}
      />

      <Row gutter={[12, 12]} data-testid="tickets-board">
        {TICKET_STATUS_ORDER.map((status) => (
          <Col key={status} xs={24} sm={12} xl={6}>
            <TicketColumn
              status={status}
              meta={meta(status)}
              apiParams={apiParams}
              filtersKey={filtersKey}
              onOpen={setOpenTicketId}
            />
          </Col>
        ))}
      </Row>

      <TicketDrawer
        ticketId={openTicketId}
        open={openTicketId !== null}
        onClose={() => setOpenTicketId(null)}
      />

      <TicketCreateModal
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        onCreated={(ticket) => setOpenTicketId(ticket.id)}
      />
    </Flex>
  );
}
