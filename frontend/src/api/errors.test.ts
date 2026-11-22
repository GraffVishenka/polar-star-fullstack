/** Тесты маппинга ошибок API в русские тексты и подписи тостов. */
import { describe, expect, it } from 'vitest';
import {
  ApiError,
  describeApiError,
  ERROR_MESSAGES,
  isApiError,
  messageForCode,
  parseApiErrorPayload,
} from '@/api/errors';

describe('errors', () => {
  it('переводит доменные коды в русские сообщения', () => {
    expect(messageForCode('BUILDING_REQUIRED_FOR_USER')).toBe(
      'Для сотрудника обязательно выберите здание',
    );
    expect(messageForCode('LAST_ADMIN')).toContain('последнего администратора');
    expect(messageForCode('CYCLE_DETECTED')).toContain('самого себя');
  });

  it('использует статус, если код неизвестен', () => {
    expect(messageForCode('SOMETHING_NEW', 404)).toBe(ERROR_MESSAGES.NOT_FOUND);
    expect(messageForCode(undefined, 503)).toContain('временно недоступен');
  });

  it('для 5xx добавляет request_id в описание тоста', () => {
    const error = new ApiError({
      code: 'INTERNAL_ERROR',
      status: 500,
      requestId: 'req-42',
    });
    expect(error.userMessage).toBe('Ошибка сервера, попробуйте позже');
    expect(describeApiError(error)).toEqual({
      title: 'Ошибка сервера, попробуйте позже',
      description: 'Код запроса: req-42',
    });
  });

  it('не показывает request_id для 4xx', () => {
    const error = new ApiError({ code: 'VALIDATION_ERROR', status: 422, requestId: 'req-1' });
    expect(describeApiError(error)).toEqual({ title: 'Проверьте заполнение полей' });
  });

  it('классифицирует ошибки по статусу', () => {
    const forbidden = new ApiError({ code: 'FORBIDDEN', status: 403 });
    const server = new ApiError({ code: 'INTERNAL_ERROR', status: 502 });
    const network = new ApiError({ code: 'NETWORK_ERROR', status: 0, isNetworkError: true });

    expect(forbidden.isClientError).toBe(true);
    expect(forbidden.shouldNotify).toBe(false);
    expect(server.isServerError).toBe(true);
    expect(server.shouldNotify).toBe(true);
    expect(network.shouldNotify).toBe(true);
    expect(isApiError(forbidden)).toBe(true);
    expect(isApiError(new Error('x'))).toBe(false);
  });

  it('терпит не-JSON тело ошибки и отсутствие полей', () => {
    expect(parseApiErrorPayload(null)).toEqual({});
    expect(parseApiErrorPayload('text')).toEqual({});
    expect(parseApiErrorPayload({ code: 'NOT_FOUND', request_id: 'r-1' })).toMatchObject({
      code: 'NOT_FOUND',
      request_id: 'r-1',
      details: null,
    });
  });
});
