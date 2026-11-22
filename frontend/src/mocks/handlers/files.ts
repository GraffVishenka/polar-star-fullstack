/** MSW: загрузка и выдача файлов (Б3, §3.13). */
import { http } from 'msw';
import type { FileScope } from '@/api/types';
import { guard } from '../auth';
import { db, fileMeta, nextId } from '../db';
import { API, fail, fileResponse, json, notFound, pathId, placeholderSvg } from '../http';
import { fileBody } from './public';

/** Лимиты Б3: фото 10 МБ, видео 200 МБ. */
const MAX_IMAGE_BYTES = 10 * 1024 * 1024;
const MAX_VIDEO_BYTES = 200 * 1024 * 1024;

export const fileHandlers = [
  http.post(`${API}/files`, async ({ request }) => {
    const auth = guard(request);
    if (!auth.ok) return auth.response;

    let form: FormData;
    try {
      form = await request.formData();
    } catch {
      return fail(422, 'VALIDATION_ERROR', 'Ожидается multipart/form-data');
    }

    const file = form.get('file');
    const scopeValue = String(form.get('scope') ?? 'ticket');
    const scope: FileScope = scopeValue === 'kb' ? 'kb' : 'ticket';

    if (!(file instanceof Blob)) {
      return fail(422, 'VALIDATION_ERROR', 'Файл не передан');
    }

    const contentType = file.type || 'application/octet-stream';
    const isImage = contentType.startsWith('image/');
    const isVideo = contentType.startsWith('video/');
    // Б3: сервер проверяет тип и размер, .exe под видом картинки → 415.
    if (!isImage && !isVideo) {
      return fail(415, 'UNSUPPORTED_MEDIA_TYPE', 'Можно загружать только фото и видео');
    }
    const limit = isImage ? MAX_IMAGE_BYTES : MAX_VIDEO_BYTES;
    if (file.size > limit) {
      return fail(413, 'PAYLOAD_TOO_LARGE', isImage ? 'Фото больше 10 МБ' : 'Видео больше 200 МБ');
    }

    const name =
      typeof (file as File).name === 'string' && (file as File).name
        ? (file as File).name
        : isImage
          ? 'photo.jpg'
          : 'video.mp4';

    const created = {
      id: nextId(db.files),
      filename: name,
      content_type: contentType,
      size_bytes: file.size,
      media_kind: isImage ? ('image' as const) : ('video' as const),
      created_at: new Date().toISOString(),
      scope,
      ticket_id: null as number | null,
      token: null as string | null,
    };
    db.files.push(created);
    return json({ file: fileMeta(created) }, 201);
  }),

  http.get(`${API}/files/:id`, ({ request, params }) => {
    const auth = guard(request);
    if (!auth.ok) return auth.response;
    const file = db.files.find((item) => item.id === pathId(params.id));
    if (!file) return notFound('Файл не найден');
    return fileResponse(
      file.content_type.startsWith('image/svg') ? placeholderSvg(file.filename) : fileBody(file),
      file.content_type,
    );
  }),
];
