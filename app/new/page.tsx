'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import {
  Aperture,
  ArrowLeft,
  ChevronRight,
  Clapperboard,
  LoaderCircle,
  Plus,
  Sparkles,
} from 'lucide-react';
import { ChoiceSelect } from '@/components/choice-select';
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group';
import { Toaster, toast } from '@/components/ui/toast';
import { api } from '@/lib/api';
import {
  defaultDraft,
  occasionHints,
  occasions,
  promptExamples,
  styleChoices,
  type ProjectDraft,
} from '@/lib/creative';
import type { Project, StudioConfig } from '@/lib/types';

export default function NewFilmPage() {
  const router = useRouter();
  const [config, setConfig] = useState<StudioConfig | null>(null);
  const [offline, setOffline] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [draft, setDraft] = useState<ProjectDraft>(defaultDraft);

  useEffect(() => {
    void api<StudioConfig>('config')
      .then(setConfig)
      .catch(() => setOffline(true));
  }, []);

  async function createProject(e: React.SyntheticEvent<HTMLFormElement>) {
    e.preventDefault();
    if (offline || !config) return;
    setBusy(true);
    setError('');
    try {
      const project = await api<Project>('projects', {
        method: 'POST',
        body: JSON.stringify(draft),
      });
      toast.add({
        title: '工程已创建',
        type: 'success',
        timeout: 5000,
      });
      router.push(`/?project=${project.id}`);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <Toaster>
      <div className="studio-shell new-film-shell">
        <header className="topbar">
          <Link className="brand" href="/" aria-label="誓光创作工作台">
            <span className="brand-icon">
              <Aperture size={24} strokeWidth={1.6} />
            </span>
            <span>
              誓光 <em>VOWFILM</em>
            </span>
          </Link>
          <div className="breadcrumb">
            创作空间 <ChevronRight size={14} />
            <span>开启新影片</span>
          </div>
          <div className="header-actions">
            <span className="connection">
              <i className={config?.connected ? 'online' : ''} />
              {config?.connected ? '创作引擎已连接' : '连接创作引擎'}
            </span>
            <Link className="secondary-button" href="/">
              <ArrowLeft size={16} />
              返回工作台
            </Link>
          </div>
        </header>

        <main className="new-film-page">
          <div className="new-film-hero">
            <div className="eyebrow">NEW WEDDING FILM</div>
            <h1>开启一部新影片</h1>
            <p>
              先选择现场播放用途，再把你想讲的故事与导演要求写下来。
            </p>
          </div>

          {offline && (
            <output className="message offline new-film-alert">
              生成服务暂时离线，无法创建新影片。请稍后重试或返回工作台观看已有内容。
            </output>
          )}

          <form className="new-film-form" onSubmit={createProject}>
            <section className="new-film-section">
              <div className="section-heading">
                <span className="section-step">01</span>
                <div>
                  <h2>现场播放用途</h2>
                  <p>这段影片将在婚礼现场的哪个环节播放？</p>
                </div>
              </div>
              <div className="occasion-grid" role="radiogroup" aria-label="现场播放用途">
                {Object.entries(occasions).map(([value, label]) => (
                  <button
                    key={value}
                    type="button"
                    className={`occasion-card ${draft.occasion === value ? 'selected' : ''}`}
                    aria-pressed={draft.occasion === value}
                    onClick={() => setDraft({ ...draft, occasion: value })}
                  >
                    <strong>{label}</strong>
                    <span>{occasionHints[value]}</span>
                  </button>
                ))}
              </div>
            </section>

            <section className="new-film-section">
              <div className="section-heading">
                <span className="section-step">02</span>
                <div>
                  <h2>故事与导演要求</h2>
                  <p>写下真实素材，以及你希望 GPT-6 导演怎么拍。</p>
                </div>
              </div>
              <div className="story-grid">
                <label>
                  影片名称
                  <input
                    required
                    maxLength={80}
                    value={draft.title}
                    onChange={(e) =>
                      setDraft({ ...draft, title: e.target.value })
                    }
                    placeholder="例如：把余生写成我们"
                  />
                </label>
                <label>
                  你们的故事与事实素材
                  <textarea
                    rows={6}
                    maxLength={4000}
                    value={draft.brief}
                    onChange={(e) =>
                      setDraft({ ...draft, brief: e.target.value })
                    }
                    placeholder="填写确实发生的相遇、共同爱好、希望感谢的人。没有资料时用象征性情节，不编造真实经历。"
                  />
                </label>
                <label htmlFor="custom-prompt">
                  给导演的自定义 Prompt
                  <textarea
                    id="custom-prompt"
                    rows={7}
                    maxLength={6000}
                    value={draft.customPrompt}
                    onChange={(e) =>
                      setDraft({ ...draft, customPrompt: e.target.value })
                    }
                    placeholder="例如：日常装 → 中式礼服 → 婚纱西装，用白纱遮挡切接。不要群像；音乐从轻拨弦走向庆祝高潮，片尾写‘故事继续，主角登场’。"
                  />
                </label>
                <div className="prompt-inspiration" aria-label="追加创作灵感">
                  <span>
                    <Sparkles size={14} />
                    追加灵感
                  </span>
                  {promptExamples.map((example) => (
                    <button
                      type="button"
                      key={example.name}
                      onClick={() =>
                        setDraft((value) => ({
                          ...value,
                          customPrompt: [value.customPrompt, example.text]
                            .filter(Boolean)
                            .join('\n')
                            .slice(0, 6000),
                        }))
                      }
                    >
                      {example.name}
                    </button>
                  ))}
                </div>
                <p className="fine-print">
                  你的具体要求优先于风格模板，参与故事、分镜与音乐规划。当前生成器乐配乐；可上传自己的旁白或音乐音轨。
                </p>
              </div>
            </section>

            <section className="new-film-section">
              <div className="section-heading">
                <span className="section-step">03</span>
                <div>
                  <h2>造型与成片规格</h2>
                  <p>选择基础风格与输出参数，可在工作台中继续调整。</p>
                </div>
              </div>
              <div className="spec-grid">
                <div className="form-row">
                  <label htmlFor="wardrobe-mode">
                    造型变化
                    <ChoiceSelect
                      id="wardrobe-mode"
                      value={draft.wardrobeMode}
                      onChange={(wardrobeMode) =>
                        setDraft({ ...draft, wardrobeMode })
                      }
                      label="造型变化"
                      items={[
                        { value: 'auto', label: '导演按章节安排 · 最多 3 套' },
                        { value: 'fixed', label: '全片固定一套造型' },
                        { value: 'custom', label: '自定义造型 · 最多 4 套' },
                      ]}
                    />
                  </label>
                  <label htmlFor="ending-text">
                    片尾大字 · 可选
                    <input
                      id="ending-text"
                      maxLength={20}
                      value={draft.endingText}
                      onChange={(e) =>
                        setDraft({ ...draft, endingText: e.target.value })
                      }
                      placeholder="例如：故事继续，主角登场"
                    />
                  </label>
                </div>
                {draft.wardrobeMode === 'custom' && (
                  <label htmlFor="wardrobe-prompt">
                    每一套造型与出现顺序
                    <textarea
                      id="wardrobe-prompt"
                      required
                      rows={3}
                      maxLength={1500}
                      value={draft.wardrobePrompt}
                      onChange={(e) =>
                        setDraft({ ...draft, wardrobePrompt: e.target.value })
                      }
                      placeholder="第一章：米白日常裙、浅色衬衫；第二章：红色中式礼服；第三章：象牙白婚纱、黑色普通西装。人物面貌保持一致。"
                    />
                  </label>
                )}
                <div className="form-row">
                  <label htmlFor="project-duration">
                    时长
                    <ChoiceSelect
                      id="project-duration"
                      value={String(draft.duration)}
                      onChange={(s) =>
                        setDraft({ ...draft, duration: Number(s) })
                      }
                      label="影片时长"
                      items={[60, 120, 180, 240].map((s) => ({
                        value: String(s),
                        label: `${s / 60} 分钟`,
                      }))}
                    />
                  </label>
                  <label htmlFor="project-ratio">
                    画幅
                    <ChoiceSelect
                      id="project-ratio"
                      value={draft.ratio}
                      onChange={(s) => setDraft({ ...draft, ratio: s })}
                      label="画幅"
                      items={[
                        { value: '16:9', label: '16:9 横屏' },
                        { value: '9:16', label: '9:16 竖屏' },
                      ]}
                    />
                  </label>
                </div>
                <fieldset className="style-fieldset">
                  <legend>选择基础风格参考</legend>
                  <RadioGroup
                    className="style-picker"
                    value={draft.style}
                    onValueChange={(value) =>
                      setDraft({ ...draft, style: String(value) })
                    }
                    aria-label="影片风格"
                  >
                    {styleChoices.map((style) => (
                      <label
                        key={style.id}
                        htmlFor={`new-style-${style.id}`}
                        className={`style-option ${draft.style === style.id ? 'selected' : ''}`}
                      >
                        <div>
                          <RadioGroupItem
                            id={`new-style-${style.id}`}
                            value={style.id}
                          />
                          <strong>{style.name}</strong>
                          <span>{style.bpm} BPM</span>
                        </div>
                        <small>{style.hint}</small>
                      </label>
                    ))}
                  </RadioGroup>
                  <p className="fine-print">
                    风格提供默认方向；你在 Prompt 中写明的内容会优先采用。
                  </p>
                </fieldset>
              </div>
            </section>

            {error && (
              <p className="inline-error new-film-alert" role="alert">
                {error}
              </p>
            )}

            <footer className="new-film-footer">
              <div>
                <Clapperboard size={18} />
                <span>
                  创建后将进入工作台，上传素材并开始编排分镜。
                </span>
              </div>
              <button
                className="primary-button"
                disabled={busy || offline || !config}
                type="submit"
              >
                {busy ? (
                  <LoaderCircle size={16} className="spin" />
                ) : (
                  <Plus size={16} />
                )}
                创建影片
              </button>
            </footer>
          </form>
        </main>
      </div>
    </Toaster>
  );
}
