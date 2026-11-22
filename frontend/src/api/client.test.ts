/** Тесты API-клиента: 401 → refresh → retry (single-flight), ошибки, скачивание. */
import { HttpResponse, http } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import {
  __resetApiClientForTests,
  apiDownload,
  apiRequest,
  buildUrl,
  getAccessToken,
  onAuthLost,
  parseContentDispositionFilename,
  setAccessToken,
} from './client';
import { ApiError, setErrorNotifier } from './errors';
import { server } from '@/mocks/server';

const PROTECTED = '*/api/protected';
const REFRESH = '*/api/auth/refresh';

function unauthorizedBody(requestId = 'req-401') {
  return {
    code: 'UNAUTHORIZED',
    message: 'Требуется авторизация',
    details: null,
    request_id: requestId,
  };
}

describe('apiRequest: refresh и повтор при 401', () => {
  it('обновляет токен один раз и повторяет исходный запрос', async () => {
    let attempts = 0;
    let refreshCalls = 0;

    server.use(
      http.get(PROTECTED, ({ request }) => {
        attempts += 1;
        return request.headers.get('Authorization') === 'Bearer fresh-token'
          ? HttpResponse.json({ ok: true })
          : HttpResponse.json(unauthorizedBody(), { status: 401 });
      }),
      http.post(REFRESH, () => {
        refreshCalls += 1;
        return HttpResponse.json({
          access_token: 'fresh-token',
          token_type: 'Bearer',
          expires_in: 900,
        });
      }),
    );

    setAccessToken('stale-token');
    const data = await apiRequest<{ ok: boolean }>('/protected');

    expect(data).toEqual({ ok: true });
    expect(attempts).toBe(2);
    expect(refreshCalls).toBe(1);
    expect(getAccessToken()).toBe('fresh-token');
  });

  it('параллельные 401 используют один refresh (single-flight)', async () => {
    let refreshCalls = 0;

    server.use(
      http.get(PROTECTED, ({ request }) =>
        request.headers.get('Authorization') === 'Bearer fresh-token'
          ? HttpResponse.json({ ok: true })
          : HttpResponse.json(unauthorizedBody(), { status: 401 }),
      ),
      http.post(REFRESH, async () => {
        refreshCalls += 1;
        // Небольшая задержка, чтобы оба запроса успели получить 401.
        await new Promise((resolve) => setTimeout(resolve, 10));
        return HttpResponse.json({
          access_token: 'fresh-token',
          token_type: 'Bearer',
          expires_in: 900,
        });
      }),
    );

    setAccessToken('stale-token');
    const results = await Promise.all([
      apiRequest<{ ok: boolean }>('/protected'),
      apiRequest<{ ok: boolean }>('/protected'),
    ]);

    expect(results).toEqual([{ ok: true }, { ok: true }]);
    expect(refreshCalls).toBe(1);
  });

  it('если refresh не удался — токен очищается и срабатывает onAuthLost', async () => {
    const onLost = vi.fn();
    onAuthLost(onLost);

    server.use(
      http.get(PROTECTED, () => HttpResponse.json(unauthorizedBody(), { status: 401 })),
      http.post(REFRESH, () => HttpResponse.json(unauthorizedBody('req-refresh'), { status: 401 })),
    );

    setAccessToken('stale-token');
    await expect(apiRequest('/protected')).rejects.toBeInstanceOf(ApiError);

    expect(getAccessToken()).toBeNull();
    expect(onLost).toHaveBeenCalledTimes(1);
  });

  it('401 на /auth/login не запускает refresh', async () => {
    let refreshCalls = 0;
    server.use(
      http.post('*/api/auth/login', () =>
        HttpResponse.json(
          {
            code: 'INVALID_CREDENTIALS',
            message: 'Неверный логин или пароль',
            details: null,
            request_id: 'r',
          },
          { status: 401 },
        ),
      ),
      http.post(REFRESH, () => {
        refreshCalls += 1;
        return HttpResponse.json({ access_token: 'x', token_type: 'Bearer', expires_in: 900 });
      }),
    );

    await expect(
      apiRequest('/auth/login', {
        method: 'POST',
        body: { login: 'admin', password: 'wrong' },
        skipAuth: true,
      }),
    ).rejects.toMatchObject({ code: 'INVALID_CREDENTIALS', status: 401 });

    expect(refreshCalls).toBe(0);
  });
});

describe('apiRequest: ошибки §3.13', () => {
  it('5xx уведомляет обработчик с request_id', async () => {
    const notifier = vi.fn();
    setErrorNotifier(notifier);

    server.use(
      http.get('*/api/boom', () =>
        HttpResponse.json(
          { code: 'INTERNAL_ERROR', message: 'panic', details: null, request_id: 'req-500' },
          { status: 500 },
        ),
      ),
    );

    await expect(apiRequest('/boom')).rejects.toBeInstanceOf(ApiError);
    expect(notifier).toHaveBeenCalledTimes(1);
    expect(notifier.mock.calls[0]?.[0]).toMatchObject({
      code: 'INTERNAL_ERROR',
      status: 500,
      requestId: 'req-500',
    });
  });

  it('4xx не уведомляет автоматически, но содержит русский userMessage', async () => {
    const notifier = vi.fn();
    setErrorNotifier(notifier);

    server.use(
      http.get('*/api/bad', () =>
        HttpResponse.json(
          {
            code: 'BUILDING_REQUIRED_FOR_USER',
            message: 'building required',
            details: { building_id: 'required' },
            request_id: 'req-422',
          },
          { status: 422 },
        ),
      ),
    );

    await expect(apiRequest('/bad')).rejects.toMatchObject({
      code: 'BUILDING_REQUIRED_FOR_USER',
      status: 422,
      userMessage: 'Для сотрудника обязательно выберите здание',
    });
    expect(notifier).not.toHaveBeenCalled();
  });

  it('сетевой сбой превращается в ApiError NETWORK_ERROR', async () => {
    server.use(http.get('*/api/offline', () => HttpResponse.error()));
    await expect(apiRequest('/offline')).rejects.toMatchObject({
      code: 'NETWORK_ERROR',
      isNetworkError: true,
    });
  });
});

describe('buildUrl', () => {
  it('сериализует массивы и пропускает пустые значения', () => {
    const url = buildUrl('/tickets', {
      status: ['new', 'done'],
      page: 2,
      building_id: undefined,
      search: '',
    });
    const parsed = new URL(url);
    expect(parsed.pathname).toBe('/api/tickets');
    expect(parsed.searchParams.getAll('status')).toEqual(['new', 'done']);
    expect(parsed.searchParams.get('page')).toBe('2');
    expect(parsed.searchParams.has('building_id')).toBe(false);
    expect(parsed.searchParams.has('search')).toBe(false);
  });
});

describe('скачивание документов', () => {
  it('разбирает имя файла из Content-Disposition', () => {
    expect(parseContentDispositionFilename('attachment; filename="ticket_12.docx"')).toBe(
      'ticket_12.docx',
    );
    expect(
      parseContentDispositionFilename(
        "attachment; filename*=UTF-8''%D0%B7%D0%B0%D1%8F%D0%B2%D0%BA%D0%B0.docx",
      ),
    ).toBe('заявка.docx');
    expect(parseContentDispositionFilename(null)).toBeNull();
  });

  it('возвращает blob и имя файла', async () => {
    server.use(
      http.get(
        '*/api/tickets/1/export.docx',
        () =>
          new HttpResponse(new Blob(['demo']), {
            status: 200,
            headers: {
              'Content-Type':
                'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
              'Content-Disposition': 'attachment; filename="ticket_1.docx"',
            },
          }),
      ),
    );

    const result = await apiDownload('/tickets/1/export.docx');
    expect(result.filename).toBe('ticket_1.docx');
    expect(result.blob.size).toBeGreaterThan(0);
  });

  it('ошибка 422 приходит как ApiError с русским текстом', async () => {
    server.use(
      http.get('*/api/warehouses/1/inventory.xlsx', () =>
        HttpResponse.json(
          { code: 'VALIDATION_ERROR', message: 'too many', details: null, request_id: 'r-1' },
          { status: 422 },
        ),
      ),
    );

    await expect(apiDownload('/warehouses/1/inventory.xlsx')).rejects.toMatchObject({
      status: 422,
      userMessage: 'Проверьте заполнение полей',
    });
  });
});

describe('сброс состояния', () => {
  it('__resetApiClientForTests очищает токен', () => {
    setAccessToken('some-token');
    __resetApiClientForTests();
    expect(getAccessToken()).toBeNull();
  });
});
