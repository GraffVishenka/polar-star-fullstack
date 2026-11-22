/**
 * Хелперы MSW-хендлеров: единый формат ответа и ошибок (§3.13).
 *
 * Возвращаемый тип намеренно `HttpResponse` без generic: тогда MSW выводит
 * единый `ResponseBody` для всех хендлеров и не требует приведения типов.
 */
import { HttpResponse } from 'msw';
import type { JsonBodyType } from 'msw';
import type { ApiErrorResponse, Page } from '@/api/types';

/**
 * Префикс с `*`, чтобы хендлеры срабатывали и в браузере (`/api/...`),
 * и в jsdom-тестах (абсолютный `http://localhost/api/...`).
 */
export const API = '*/api';

let requestCounter = 0;

export function mockRequestId(): string {
  requestCounter += 1;
  return `mock-${requestCounter.toString().padStart(4, '0')}`;
}

export function json<T>(data: T, status = 200): Response {
  return HttpResponse.json(data as JsonBodyType, { status });
}

export function created<T>(data: T): Response {
  return HttpResponse.json(data as JsonBodyType, { status: 201 });
}

export function noContent(): Response {
  return new HttpResponse(null, { status: 204 });
}

/** Ошибка в формате `{code,message,details,request_id}`. */
export function fail(
  status: number,
  code: string,
  message: string,
  details: Record<string, unknown> | null = null,
): Response {
  const body: ApiErrorResponse = { code, message, details, request_id: mockRequestId() };
  return HttpResponse.json(body, { status });
}

export const unauthorized = () => fail(401, 'UNAUTHORIZED', 'Требуется авторизация');
export const forbidden = (message = 'Недостаточно прав для этого действия') =>
  fail(403, 'FORBIDDEN', message);
export const notFound = (message = 'Запись не найдена') => fail(404, 'NOT_FOUND', message);
export const conflict = (code: string, message: string) => fail(409, code, message);
export const validation = (
  message = 'Проверьте заполнение полей',
  details: Record<string, unknown> | null = null,
) => fail(422, 'VALIDATION_ERROR', message, details);
export const tooManyRequests = () => fail(429, 'TOO_MANY_REQUESTS', 'Слишком много запросов');
export const passwordChangeRequired = () =>
  fail(428, 'PASSWORD_CHANGE_REQUIRED', 'Сначала смените временный пароль');

export async function readBody<T>(request: Request): Promise<Partial<T>> {
  try {
    const parsed = (await request.json()) as unknown;
    return parsed && typeof parsed === 'object' ? (parsed as Partial<T>) : {};
  } catch {
    return {};
  }
}

export function queryId(request: Request, key: string): number | null {
  const raw = new URL(request.url).searchParams.get(key);
  if (raw === null || raw.trim() === '') return null;
  const parsed = Number(raw);
  return Number.isSafeInteger(parsed) ? parsed : null;
}

export function pathId(value: string | readonly string[] | undefined): number | null {
  const raw = Array.isArray(value) ? value[0] : value;
  if (!raw) return null;
  const parsed = Number(raw);
  return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : null;
}

export function paginate<T>(items: T[], page = 1, size = 20): Page<T> {
  const safePage = page > 0 ? page : 1;
  const safeSize = size > 0 ? Math.min(size, 100) : 20;
  const start = (safePage - 1) * safeSize;
  return {
    items: items.slice(start, start + safeSize),
    total: items.length,
    page: safePage,
    size: safeSize,
  };
}

export function fileResponse(
  body: string | Blob,
  contentType: string,
  filename?: string,
): Response {
  const headers: Record<string, string> = {
    'Content-Type': contentType,
    'Cache-Control': 'no-store',
    'X-Content-Type-Options': 'nosniff',
  };
  if (filename) {
    headers['Content-Disposition'] = `attachment; filename="${filename}"`;
  }
  return new HttpResponse(body, { status: 200, headers });
}

/** Простая SVG-заглушка: в mock-режиме фото реально отображаются. */
export function placeholderSvg(label: string): string {
  const safe = label.replace(/[<>&"]/g, '');
  return `<svg xmlns="http://www.w3.org/2000/svg" width="480" height="320" viewBox="0 0 480 320">
  <rect width="480" height="320" fill="#e6f4ff"/>
  <rect x="12" y="12" width="456" height="296" fill="none" stroke="#91caff" stroke-width="2" rx="10"/>
  <text x="240" y="150" font-family="Arial, sans-serif" font-size="20" fill="#0958d9" text-anchor="middle">${safe}</text>
  <text x="240" y="180" font-family="Arial, sans-serif" font-size="13" fill="#69b1ff" text-anchor="middle">демо-изображение (MSW)</text>
</svg>`;
}
