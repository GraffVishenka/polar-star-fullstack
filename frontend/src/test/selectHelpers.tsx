/**
 * Выбор опции в antd Select для тестов.
 *
 * antd рендерит два списка: скрытый a11y-`listbox` (role=option с aria-label)
 * и видимый `.ant-select-item-option` (title). Клик по скрытому элементу не
 * меняет значение, поэтому выбираем именно видимую опцию.
 */
import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

export async function pickSelectOption(selectTestId: string, optionLabel: string): Promise<void> {
  const select = screen.getByTestId(selectTestId);
  await userEvent.click(within(select).getByRole('combobox'));

  const option = await waitFor(() => {
    const dropdowns = [...document.querySelectorAll<HTMLElement>('.ant-select-dropdown')].filter(
      (dropdown) => !dropdown.classList.contains('ant-select-dropdown-hidden'),
    );
    for (const dropdown of dropdowns) {
      const found = [...dropdown.querySelectorAll<HTMLElement>('.ant-select-item-option')].find(
        (item) => item.getAttribute('title') === optionLabel,
      );
      if (found) return found;
    }
    throw new Error(`Опция «${optionLabel}» не найдена в ${selectTestId}`);
  });

  await userEvent.click(option);
}
