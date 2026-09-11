'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { BrandLogo } from '@/components/brand-logo';
import { useRouter } from 'next/navigation';
import {
  ArrowLeft,
  Bot,
  ChevronRight,
  Clapperboard,
  LayoutTemplate,
  LoaderCircle,
  Plus,
} from 'lucide-react';
import { CreativeFields } from '@/components/creative-fields';
import { TemplateCard } from '@/components/template-card';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { filmTemplates, templateDraft } from '@/lib/templates';
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group';
import { Toaster, toast } from '@/components/ui/toast';
import { api } from '@/lib/api';
import { useAccount } from '@/components/account-provider';
import {
  defaultDraft,
  sceneChoices,
  changeScene,
  type ProjectDraft,
} from '@/lib/creative';
import type { Project, StudioConfig } from '@/lib/types';

export default function NewFilmPage() {
  const router = useRouter();
  const { auth } = useAccount();
  const [config, setConfig] = useState<StudioConfig | null>(null);
  const [offline, setOffline] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [mode, setMode] = useState('template');
  const [agentDraft, setAgentDraft] = useState<ProjectDraft>(defaultDraft);
  const [presetDraft, setPresetDraft] = useState<ProjectDraft>(() =>
    templateDraft(filmTemplates[0]),
  );
  const draft = mode === 'template' ? presetDraft : agentDraft;
  const setDraft = mode === 'template' ? setPresetDraft : setAgentDraft;
  const selectedTemplate =
    filmTemplates.find((item) => item.id === presetDraft.templateId) ||
    filmTemplates[0];

  useEffect(() => {
    // The URL is the deep-link entry point for Agent mode.
    if (new URLSearchParams(window.location.search).get('mode') === 'agent') {
      // oxlint-disable-next-line react-compiler
      setMode('agent');
    }
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
            <BrandLogo />
          </Link>
          <div className="breadcrumb">
            创作空间 <ChevronRight size={14} />
            <span>开启新影片</span>
          </div>
          <div className="header-actions">
            <span className="connection">
              <i className={config?.connected ? 'online' : ''} />
              {config?.connected ? '创作引擎已连接' : '等待配置 API Key'}
            </span>
            {auth?.permissions.includes('config:manage') && (
              <Link className="secondary-button" href="/settings">
                引擎配置
              </Link>
            )}
            <Link className="secondary-button" href="/">
              <ArrowLeft size={16} />
              返回工作台
            </Link>
          </div>
        </header>

        <main className="new-film-page">
          <div className="new-film-hero">
            <div className="eyebrow">NEW CREATIVE PROJECT</div>
            <h1>开启一部新影片</h1>
            <p>选一套喜欢的模板，或让 Agent 为你的故事自由创作。</p>
          </div>

          {offline && (
            <output className="message offline new-film-alert">
              生成服务暂时离线，无法创建新影片。请先检查{' '}
              <Link href="/settings">创作引擎配置</Link> 或稍后重试。
            </output>
          )}

          <form className="new-film-form" onSubmit={createProject}>
            <Tabs
              value={mode}
              onValueChange={(value) => {
                setMode(String(value));
                setError('');
              }}
              className="creation-modes"
            >
              <TabsList className="creation-mode-tabs" aria-label="创作方式">
                <TabsTrigger value="template" disabled={busy}>
                  <LayoutTemplate size={19} aria-hidden="true" />
                  <span>
                    模板创作<small>固定结构，替换素材</small>
                  </span>
                </TabsTrigger>
                <TabsTrigger value="agent" disabled={busy}>
                  <Bot size={19} aria-hidden="true" />
                  <span>
                    Agent 创作<small>描述想法，自由编排</small>
                  </span>
                </TabsTrigger>
              </TabsList>
              <TabsContent value="template" className="template-selection">
                <div className="template-section-heading">
                  <div>
                    <span className="eyebrow">TEMPLATE COLLECTION</span>
                    <h2>先看效果，再开始创作</h2>
                  </div>
                  <span>婚庆 · 1 套模板</span>
                </div>
                <div className="template-selection-layout">
                  <div className="film-template-grid">
                    {filmTemplates.map((template) => (
                      <TemplateCard
                        key={template.id}
                        template={template}
                        selected={draft.templateId === template.id}
                        disabled={busy}
                        onSelect={() =>
                          setPresetDraft((previous) => ({
                            ...previous,
                            ...templateDraft(template),
                            title: previous.title,
                            brief: previous.brief,
                            customPrompt: previous.customPrompt,
                            endingText: previous.endingText,
                          }))
                        }
                      />
                    ))}
                  </div>
                  <aside className="template-guide">
                    <span className="eyebrow">YOUR STORY, OUR FRAME</span>
                    <h3>
                      故事由你提供，
                      <br />
                      节奏已经就位。
                    </h3>
                    <p>{selectedTemplate.description}</p>
                    <ol>
                      <li>
                        <strong>看看预览</strong>
                        <span>确认氛围与六幕叙事结构</span>
                      </li>
                      <li>
                        <strong>填入你们的资料</strong>
                        <span>保留真实故事，补充片尾寄语</span>
                      </li>
                      <li>
                        <strong>上传素材，生成影片</strong>
                        <span>固定 12 个镜头，逐镜生成与合成</span>
                      </li>
                    </ol>
                    <p className="template-guide-note">
                      预览区直接播放参考原片，模板会重新编排自己的镜头、造型与字幕规则。
                    </p>
                  </aside>
                </div>
              </TabsContent>
              <TabsContent value="agent">
                <div className="agent-introduction">
                  <Bot size={26} aria-hidden="true" />
                  <div>
                    <h2>让导演 Agent 从你的想法出发</h2>
                    <p>
                      根据场景、故事和素材编排专属方案，可修改分镜、局部重做，适合没有固定脚本的创作。
                    </p>
                  </div>
                </div>
                <section className="new-film-section">
                  <div className="section-heading">
                    <span className="section-step">
                      <Clapperboard size={18} />
                    </span>
                    <div>
                      <h2>选择创作场景</h2>
                      <p>场景决定叙事方式、素材要求和成片规格。</p>
                    </div>
                  </div>
                  <RadioGroup
                    className="scene-grid"
                    value={draft.scene}
                    onValueChange={(value) =>
                      setDraft(changeScene(draft, String(value)))
                    }
                    aria-label="创作场景"
                  >
                    {sceneChoices.map((s) => (
                      <label
                        key={s.id}
                        className={`scene-card ${draft.scene === s.id ? 'selected' : ''}`}
                        htmlFor={`scene-${s.id}`}
                      >
                        <span className="eyebrow">{s.eyebrow}</span>
                        <div>
                          <RadioGroupItem id={`scene-${s.id}`} value={s.id} />
                          <h3>{s.name}</h3>
                        </div>
                        <p>{s.description}</p>
                      </label>
                    ))}
                  </RadioGroup>
                </section>
              </TabsContent>
            </Tabs>
            <section className="new-film-section">
              <div className="section-heading">
                <span className="section-step">01</span>
                <div>
                  <h2>
                    {mode === 'template'
                      ? '把你们的故事填进来'
                      : draft.scene === 'commerce'
                        ? '商品与广告要求'
                        : '素材与创作想法'}
                  </h2>
                  <p>
                    {mode === 'template'
                      ? '姓名、相处细节和寄语均可选填；素材将在创建后上传。'
                      : '写下已有资料即可，名称和成片规格可在高级设置中调整。'}
                  </p>
                </div>
              </div>
              <fieldset
                className="new-creative-fields"
                disabled={busy || offline}
              >
                <CreativeFields
                  draft={draft}
                  setDraft={setDraft}
                  disabled={busy || offline}
                  error={error}
                />
              </fieldset>
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
                  {mode === 'template'
                    ? `使用「${selectedTemplate.name}」· 60 秒横屏 · 12 个固定镜头`
                    : '创建后将进入工作台，上传素材并开始编排分镜。'}
                </span>
              </div>
              <button
                className="primary-button"
                disabled={
                  busy ||
                  offline ||
                  !config ||
                  !auth?.permissions.includes('project:write')
                }
                type="submit"
              >
                {busy ? (
                  <LoaderCircle size={16} className="spin" />
                ) : (
                  <Plus size={16} />
                )}
                {mode === 'template' ? '使用模板创建' : '交给 Agent 创作'}
              </button>
            </footer>
          </form>
        </main>
      </div>
    </Toaster>
  );
}
