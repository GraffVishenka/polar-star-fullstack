/** 403-страница доступна по маршруту и объясняет ситуацию. */
import { screen } from '@testing-library/react';
import { Route, Routes } from 'react-router-dom';
import { describe, expect, it } from 'vitest';
import { makeAnonymousAuthValue, renderWithProviders } from '@/test/utils';
import { ForbiddenPage } from './ForbiddenPage';

describe('ForbiddenPage', () => {
  it('показывает 403 с объяснением', () => {
    renderWithProviders(
      <Routes>
        <Route path="/403" element={<ForbiddenPage />} />
      </Routes>,
      { route: '/403', auth: makeAnonymousAuthValue() },
    );

    expect(screen.getByTestId('forbidden-page')).toBeInTheDocument();
    expect(screen.getByText('Доступ запрещён')).toBeInTheDocument();
  });
});
