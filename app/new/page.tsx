'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { BrandLogo } from '@/components/brand-logo';
import { useRouter } from 'next/navigation';
import {
  ArrowLeft,
  ChevronRight,
  Clapperboard,
  LoaderCircle,
  Plus,
} from 'lucide-react';
import { CreativeFields } from '@/components/creative-fields';
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
            <p>先选择创作场景，再写下你的素材与导演要求。</p>
          </div>

          {offline && (
            <output className="message offline new-film-alert">
              生成服务暂时离线，无法创建新影片。请先检查{' '}
              <Link href="/settings">创作引擎配置</Link> 或稍后重试。
            </output>
          )}

          <form className="new-film-form" onSubmit={createProject}>
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
            <section className="new-film-section">
              <div className="section-heading">
                <span className="section-step">01</span>
                <div>
                  <h2>
                    {draft.scene === 'commerce'
                      ? '商品与广告要求'
                      : '素材与创作想法'}
                  </h2>
                  <p>写下已有资料即可，名称和成片规格可在高级设置中调整。</p>
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
                <span>创建后将进入工作台，上传素材并开始编排分镜。</span>
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
                创建影片
              </button>
            </footer>
          </form>
        </main>
      </div>
    </Toaster>
  );
}
