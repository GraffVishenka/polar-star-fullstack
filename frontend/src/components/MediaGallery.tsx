/**
 * MediaGallery — фото и видео вложений (TICKET-14, PUB-6, KB-5).
 *
 * Изображения открываются во встроенном просмотрщике antd, видео
 * воспроизводится inline (`controls`, без автоплея). Для анонимной карточки
 * элементы приходят с публичными URL, содержащими токен заявки.
 */
import { Flex, Image, Space, Typography } from 'antd';
import { FileOutlined, VideoCameraOutlined } from '@ant-design/icons';
import type { FileMeta } from '@/api/types';

export interface MediaGalleryProps {
  files: FileMeta[];
  /** Что показать, если вложений нет (null — ничего не рендерить). */
  emptyText?: string | null;
  /** Плотная сетка для комментариев. */
  compact?: boolean;
  /** Максимальная высота превью в пикселях. */
  maxPreviewHeight?: number;
}

function isImage(file: FileMeta): boolean {
  return file.media_kind === 'image' || file.content_type.startsWith('image/');
}

function isVideo(file: FileMeta): boolean {
  return file.media_kind === 'video' || file.content_type.startsWith('video/');
}

export function MediaGallery({
  files,
  emptyText = 'Вложений нет',
  compact = false,
  maxPreviewHeight = 180,
}: MediaGalleryProps) {
  if (!files.length) {
    if (emptyText === null) return null;
    return (
      <Typography.Text type="secondary" style={{ fontSize: 13 }} data-testid="media-gallery-empty">
        {emptyText}
      </Typography.Text>
    );
  }

  const images = files.filter(isImage);

  return (
    <Flex vertical gap={8} data-testid="media-gallery">
      <Image.PreviewGroup>
        <Flex wrap gap={8}>
          {images.map((file) => (
            <Image
              key={file.id}
              src={file.url}
              alt={file.filename}
              width={compact ? 88 : 132}
              height={compact ? 88 : 132}
              style={{ objectFit: 'cover', borderRadius: 8, border: '1px solid #f0f0f0' }}
              preview={{
                src: file.url,
                mask: <span>Открыть</span>,
              }}
              data-testid="media-gallery-image"
            />
          ))}
        </Flex>
      </Image.PreviewGroup>

      <Flex vertical gap={8}>
        {files.filter(isVideo).map((file) => (
          <div key={file.id} data-testid="media-gallery-video">
            <video
              src={file.url}
              controls
              preload="metadata"
              style={{
                width: '100%',
                maxWidth: 520,
                maxHeight: maxPreviewHeight * 1.6,
                borderRadius: 8,
                background: '#000',
              }}
            />
            <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block' }}>
              <VideoCameraOutlined /> {file.filename}
            </Typography.Text>
          </div>
        ))}
      </Flex>

      {files.some((file) => !isImage(file) && !isVideo(file)) ? (
        <Space direction="vertical" size={4}>
          {files
            .filter((file) => !isImage(file) && !isVideo(file))
            .map((file) => (
              <a key={file.id} href={file.url} target="_blank" rel="noreferrer">
                <FileOutlined /> {file.filename}
              </a>
            ))}
        </Space>
      ) : null}
    </Flex>
  );
}
