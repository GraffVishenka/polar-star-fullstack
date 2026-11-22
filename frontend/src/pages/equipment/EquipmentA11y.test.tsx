/**
 * Доступность `/equipment` (Ф4 + аудит перед Ф11): landmark дерева, доступные
 * имена фильтров, чекбоксов строк и кнопки ведомости.
 */
import { screen, within } from '@testing-library/react';
import { Route, Routes } from 'react-router-dom';
import { describe, expect, it } from 'vitest';
import { USER_AUTH, authenticateAs, makeAuthValue, renderWithProviders } from '@/test/utils';
import { EquipmentPage } from './EquipmentPage';

function renderPage(route = '/equipment') {
  authenticateAs(USER_AUTH.id);
  return renderWithProviders(
    <Routes>
      <Route path="/equipment" element={<EquipmentPage />} />
    </Routes>,
    { route, auth: makeAuthValue({ user: USER_AUTH }) },
  );
}

describe('EquipmentPage: доступность', () => {
  it('дерево расположения — отдельный landmark с названием', async () => {
    renderPage();

    expect(
      await screen.findByRole('navigation', { name: 'Расположение оборудования' }),
    ).toBeInTheDocument();
  });

  it('у фильтров есть доступные имена', async () => {
    renderPage();
    await screen.findByTestId('equipment-filters');

    expect(screen.getByRole('combobox', { name: 'Фильтр по зданию' })).toBeInTheDocument();
    expect(screen.getByRole('combobox', { name: 'Фильтр по складу' })).toBeInTheDocument();
    expect(
      screen.getByRole('combobox', { name: 'Фильтр по типу оборудования' }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('searchbox', {
        name: 'Поиск по названию, серийному или регистрационному номеру',
      }),
    ).toBeInTheDocument();
  });

  it('чекбокс строки имеет имя с оборудованием, кнопка ведомости — понятную подпись', async () => {
    renderPage();

    expect(
      await screen.findByLabelText('Выбрать оборудование «Ноутбук Dell Latitude», INV-1001'),
    ).toBeInTheDocument();

    const exportButton = within(screen.getByTestId('inventory-export')).getByRole('button', {
      name: /Ведомость помещения/,
    });
    // Склад не выбран — кнопка недоступна, но остаётся в порядке фокуса с подсказкой.
    expect(exportButton).toBeDisabled();
  });
});
