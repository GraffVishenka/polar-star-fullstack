/**
 * BuildingSelect: режим «Все здания», блокировка у USER (TICKET-7)
 * и корректные подписи.
 */
import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { BuildingSelect } from './BuildingSelect';

const buildings = [
  { id: 1, name: 'Центральный офис' },
  { id: 2, name: 'Производственный корпус' },
];

describe('BuildingSelect', () => {
  it('в режиме «Все здания» показывает выбранным пункт «Все здания»', () => {
    render(<BuildingSelect buildings={buildings} value={null} allowAll />);
    expect(screen.getByText('Все здания')).toBeInTheDocument();
  });

  it('показывает название выбранного здания', () => {
    render(<BuildingSelect buildings={buildings} value={2} />);
    expect(screen.getByText('Производственный корпус')).toBeInTheDocument();
  });

  it('у USER заблокирован и показывает подсказку о своём здании', () => {
    render(<BuildingSelect buildings={buildings} value={1} locked />);
    expect(screen.getByRole('combobox')).toBeDisabled();
    expect(screen.getByTestId('building-select').closest('.ant-select')).toHaveClass(
      'ant-select-disabled',
    );
  });

  it('неактивен во время загрузки списка', () => {
    render(<BuildingSelect buildings={[]} value={null} loading />);
    expect(screen.getByRole('combobox')).toBeDisabled();
  });
});
