'use client';

import { Check, ExternalLink, Film } from 'lucide-react';
import type { FilmTemplate } from '@/lib/templates';

export function TemplateCard({
  template,
  selected,
  onSelect,
  disabled,
}: {
  template: FilmTemplate;
  selected: boolean;
  onSelect: () => void;
  disabled: boolean;
}) {
  return (
    <article className={'film-template-card' + (selected ? ' selected' : '')}>
      <div className="template-preview">
        <iframe
          title="婚庆模板参考片预览"
          src={
            'https://player.bilibili.com/player.html?bvid=' +
            template.referenceBvid +
            '&page=1&autoplay=0'
          }
          allow="fullscreen"
          allowFullScreen
          loading="lazy"
        />
      </div>
      <div className="template-preview-toolbar">
        <span>
          <Film size={13} aria-hidden="true" />
          参考效果预览 · 原作者作品
        </span>
        <a href={template.referenceUrl} target="_blank" rel="noreferrer">
          在 Bilibili 播放
          <ExternalLink size={13} aria-hidden="true" />
        </a>
      </div>
      <div className="template-card-content">
        <div className="template-title-row">
          <div>
            <span className="eyebrow">WEDDING FILM / 01</span>
            <h3>{template.name}</h3>
          </div>
          <span className="template-spec">
            {template.duration} 秒 · {template.ratio}
          </span>
        </div>
        <p>{template.subtitle}</p>
        <div className="template-chapter-strip" aria-label="模板结构">
          {template.chapters.map((chapter, i) => (
            <span key={chapter.title}>
              <small>0{i + 1}</small>
              {chapter.title.split(' · ')[0]}
            </span>
          ))}
        </div>
        <div className="template-card-actions">
          <span className="fine-print">六幕故事 · 分章造型 · 12 镜头</span>
          <button
            type="button"
            className={selected ? 'secondary-button' : 'primary-button'}
            aria-pressed={selected}
            onClick={onSelect}
            disabled={disabled}
          >
            {selected && <Check size={16} aria-hidden="true" />}
            {selected ? '已选择此模板' : '选择此模板'}
          </button>
        </div>
      </div>
    </article>
  );
}
