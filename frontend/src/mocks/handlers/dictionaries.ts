/**
 * MSW: справочники, доступные авторизованным.
 *
 * `GET /api/problem-types` — аддитивное расширение контракта §3.13: без него
 * USER не может заполнить форму заявки (формы создания и фильтры). См.
 * `src/api/dictionaries.ts`.
 */
import { http } from 'msw';
import { guard } from '../auth';
import { db } from '../db';
import { API, json } from '../http';

export const dictionaryHandlers = [
  http.get(`${API}/problem-types`, ({ request }) => {
    const auth = guard(request);
    if (!auth.ok) return auth.response;
    return json(db.problemTypes);
  }),
];
