/** StatusTag: 4 фиксированных статуса, подписи и цвета — из пропсов (TICKET-1). */
import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { StatusTag } from './StatusTag';

describe('StatusTag', () => {
  it('показывает русские названия по умолчанию для всех 4 статусов', () => {
    render(
      <div>
        <StatusTag status="new" />
        <StatusTag status="in_progress" />
        <StatusTag status="deferred" />
        <StatusTag status="done" />
      </div>,
    );

    expect(screen.getByTestId('status-tag-new')).toHaveTextContent('Новая');
    expect(screen.getByTestId('status-tag-in_progress')).toHaveTextContent('В работе');
    expect(screen.getByTestId('status-tag-deferred')).toHaveTextContent('Отложена');
    expect(screen.getByTestId('status-tag-done')).toHaveTextContent('Завершена');
  });

  it('берёт название и цвет из настроек сервера (status_label/status_color)', () => {
    render(<StatusTag status="deferred" label="Ждём запчасти" color="#123456" />);
    const tag = screen.getByTestId('status-tag-deferred');

    expect(tag).toHaveTextContent('Ждём запчасти');
    // Цвет из пропса применяется инлайном (antd нормализует hex в rgb).
    expect(tag).toHaveStyle({ backgroundColor: 'rgb(18, 52, 86)' });
    expect(tag).toHaveAttribute('data-status', 'deferred');
  });

  it('игнорирует пустые label/color и падает на значения по умолчанию', () => {
    render(<StatusTag status="done" label="   " color="" />);
    expect(screen.getByTestId('status-tag-done')).toHaveTextContent('Завершена');
  });
});
