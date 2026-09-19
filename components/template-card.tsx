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
  const guided = template.engine === 'wedding-guided';
  return (
    <article className={'film-template-card' + (selected ? ' selected' : '')}>
      <div className="template-preview">
        {guided ? (
          <video
            controls
            preload="metadata"
            playsInline
            poster={template.posterUrl}
            aria-label="婚礼故事向导完整演示片"
          >
            <source src={template.previewUrl} type="video/mp4" />
            <track
              kind="captions"
              src="/demo/wedding-story-guided.vtt"
              srcLang="zh"
              label="中文字幕"
            />
          </video>
        ) : (
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
        )}
      </div>
      <div className="template-preview-toolbar">
        <span>
          <Film size={13} aria-hidden="true" />
          {guided
            ? '完整模板演示 · 虚构人物与故事'
            : '参考效果预览 · 原作者作品'}
        </span>
        <a href={template.referenceUrl} target="_blank" rel="noreferrer">
          {guided ? '查看原始模板' : '在 Bilibili 播放'}
          <ExternalLink size={13} aria-hidden="true" />
        </a>
      </div>
      <div className="template-card-content">
        <div className="template-title-row">
          <div>
            <span className="eyebrow">
              {guided ? 'YOUR STORY / GUIDED' : 'WEDDING FILM / 01'}
            </span>
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
          <span className="fine-print">
            {guided
              ? '一键成片 · 旁白与字幕 · 五幕故事'
              : `${template.bpm ? `${template.bpm} BPM · ` : ''}${template.chapters.length} 幕故事 · ${template.shots.length} 镜头`}
          </span>
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
