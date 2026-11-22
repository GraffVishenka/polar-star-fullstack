/**
 * Копирование текста в буфер обмена.
 *
 * `navigator.clipboard` доступен только в secure context (HTTPS/localhost);
 * для внутренних сетей с http остаётся fallback через скрытый textarea.
 * Возвращает `true`, если копирование удалось.
 */
export async function copyText(text: string): Promise<boolean> {
  try {
    if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch {
    // Падаем в fallback ниже (нет разрешения/не secure context).
  }

  try {
    const area = document.createElement('textarea');
    area.value = text;
    area.setAttribute('readonly', '');
    area.style.position = 'fixed';
    area.style.top = '-1000px';
    area.style.opacity = '0';
    document.body.appendChild(area);
    area.select();
    const copied = document.execCommand('copy');
    area.remove();
    return copied;
  } catch {
    return false;
  }
}
