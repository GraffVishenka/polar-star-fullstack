/**
 * DeadlineBadge: просрочка берётся ТОЛЬКО из серверного `is_overdue`
 * (TICKET-16, NFR-7). Клиент не сравнивает даты.
 */
import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { DeadlineBadge, formatDeadline } from './DeadlineBadge';

describe('DeadlineBadge', () => {
  it('не помечает просрочку сам, даже если дата в прошлом', () => {
    render(<DeadlineBadge deadline="2020-01-01" />);
    const badge = screen.getByTestId('deadline-badge');

    expect(badge).toHaveAttribute('data-overdue', 'false');
    expect(screen.queryByText('Просрочено')).not.toBeInTheDocument();
    expect(badge).toHaveTextContent('01.01.2020');
  });

  it('показывает «Просрочено» по серверному флагу is_overdue', () => {
    render(<DeadlineBadge deadline="2026-03-05" isOverdue />);
    const badge = screen.getByTestId('deadline-badge');

    expect(badge).toHaveAttribute('data-overdue', 'true');
    expect(screen.getByText('Просрочено')).toBeInTheDocument();
  });

  it('для завершённой заявки не показывает просрочку', () => {
    render(<DeadlineBadge deadline="2026-03-05" isOverdue completedAt="2026-03-04T10:00:00Z" />);
    expect(screen.getByTestId('deadline-badge')).toHaveAttribute('data-overdue', 'false');
    expect(screen.queryByText('Просрочено')).not.toBeInTheDocument();
  });

  it('без дедлайна показывает прочерк', () => {
    render(<DeadlineBadge deadline={null} withLabel />);
    expect(screen.getByTestId('deadline-badge-empty')).toHaveTextContent('Дедлайн: —');
  });

  it('форматирует дату в DD.MM.YYYY и терпит мусор', () => {
    expect(formatDeadline('2026-12-31')).toBe('31.12.2026');
    expect(formatDeadline('не дата')).toBe('не дата');
  });
});
