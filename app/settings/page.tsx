'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { BrandLogo } from '@/components/brand-logo';
import {
  ArrowLeft,
  ChevronRight,
  KeyRound,
  LoaderCircle,
  Save,
  Server,
} from 'lucide-react';
import { Toaster, toast } from '@/components/ui/toast';
import { api } from '@/lib/api';
import type { StudioConfig } from '@/lib/types';

type ProviderDraft = {
  baseUrl: string;
  apiKey: string;
  llmModel: string;
  videoModel: string;
  audioModel: string;
};

export default function SettingsPage() {
  const [config, setConfig] = useState<StudioConfig | null>(null);
  const [draft, setDraft] = useState<ProviderDraft>({
    baseUrl: '',
    apiKey: '',
    llmModel: '',
    videoModel: '',
    audioModel: '',
  });
  const [changeKey, setChangeKey] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    void api<StudioConfig>('config')
      .then((settings) => {
        setConfig(settings);
        setDraft({
          baseUrl: settings.baseUrl || 'https://open.embervale.cn',
          apiKey: '',
          llmModel: settings.llmModel || '',
          videoModel: settings.videoModel || '',
          audioModel: settings.audioModel || '',
        });
      })
      .catch((e) => setError((e as Error).message));
  }, []);

  async function saveSettings(e: React.SyntheticEvent<HTMLFormElement>) {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      const body: Record<string, string> = {
        baseUrl: draft.baseUrl.trim(),
        llmModel: draft.llmModel.trim(),
        videoModel: draft.videoModel.trim(),
        audioModel: draft.audioModel.trim(),
      };
      if (changeKey || !config?.apiKeySet) {
        body.apiKey = draft.apiKey.trim();
      }
      const settings = await api<StudioConfig>('config', {
        method: 'PATCH',
        body: JSON.stringify(body),
      });
      setConfig(settings);
      setDraft((value) => ({ ...value, apiKey: '' }));
      setChangeKey(false);
      toast.add({
        title: '创作引擎配置已保存',
        type: 'success',
        timeout: 5000,
      });
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
            <span>创作引擎配置</span>
          </div>
          <div className="header-actions">
            <span className="connection">
              <i className={config?.connected ? 'online' : ''} />
              {config?.connected ? '创作引擎已连接' : '等待配置 API Key'}
            </span>
            <Link className="secondary-button" href="/">
              <ArrowLeft size={16} />
              返回工作台
            </Link>
          </div>
        </header>

        <main className="new-film-page settings-page">
          <div className="new-film-hero">
            <div className="eyebrow">PROVIDER SETTINGS</div>
            <h1>自定义创作引擎</h1>
            <p>
              配置 OpenAI 兼容 API 的地址、密钥与模型。支持星网、OpenAI、Azure
              OpenAI 或任何兼容 <code>/v1/chat/completions</code> 的服务。
            </p>
          </div>

          <form className="new-film-form" onSubmit={saveSettings}>
            <section className="new-film-section">
              <div className="section-heading">
                <span className="section-step">
                  <Server size={18} />
                </span>
                <div>
                  <h2>API 连接</h2>
                  <p>LLM 分镜、视频生成与配乐都通过此网关访问。</p>
                </div>
              </div>
              <div className="story-grid">
                <label htmlFor="provider-base-url">
                  API 地址
                  <input
                    id="provider-base-url"
                    required
                    type="url"
                    value={draft.baseUrl}
                    onChange={(e) =>
                      setDraft({ ...draft, baseUrl: e.target.value })
                    }
                    placeholder="https://open.embervale.cn"
                  />
                </label>
                <label htmlFor="provider-api-key">
                  API Key
                  <input
                    id="provider-api-key"
                    type="password"
                    autoComplete="off"
                    value={draft.apiKey}
                    onChange={(e) => {
                      setChangeKey(true);
                      setDraft({ ...draft, apiKey: e.target.value });
                    }}
                    placeholder={
                      config?.apiKeySet
                        ? `已保存 ${config.apiKeyHint || '****'}，留空则保持不变`
                        : 'sk-…'
                    }
                    required={!config?.apiKeySet}
                  />
                </label>
                <p className="fine-print">
                  Key 保存在服务端 <code>data/provider.json</code>
                  ，不会写入浏览器或 Git。修改 Key 时重新输入完整内容即可。
                </p>
              </div>
            </section>

            <section className="new-film-section">
              <div className="section-heading">
                <span className="section-step">
                  <KeyRound size={18} />
                </span>
                <div>
                  <h2>模型标识</h2>
                  <p>分别用于 GPT 分镜编排、H3 视频生成与分段配乐生成。</p>
                </div>
              </div>
              <div className="story-grid">
                <label htmlFor="provider-llm-model">
                  LLM 模型
                  <input
                    id="provider-llm-model"
                    required
                    value={draft.llmModel}
                    onChange={(e) =>
                      setDraft({ ...draft, llmModel: e.target.value })
                    }
                    placeholder="openai/gpt-6-astra"
                  />
                </label>
                <label htmlFor="provider-video-model">
                  视频模型
                  <input
                    id="provider-video-model"
                    required
                    value={draft.videoModel}
                    onChange={(e) =>
                      setDraft({ ...draft, videoModel: e.target.value })
                    }
                    placeholder="starnet/minimax-h3"
                  />
                </label>
                <label htmlFor="provider-audio-model">
                  配乐模型
                  <input
                    id="provider-audio-model"
                    value={draft.audioModel}
                    onChange={(e) =>
                      setDraft({ ...draft, audioModel: e.target.value })
                    }
                    placeholder="volcengine/doubao-seed-audio-1-0"
                  />
                </label>
                <p className="fine-print">
                  也可在 `.env` 中设置 <code>STARNET_*</code> 或{' '}
                  <code>OPENAI_BASE_URL</code> / <code>OPENAI_API_KEY</code>{' '}
                  作为启动默认值；页面保存后会覆盖并持久化到本地。
                </p>
              </div>
            </section>

            {error && (
              <p className="inline-error new-film-alert" role="alert">
                {error}
              </p>
            )}

            <footer className="new-film-footer">
              <div>
                <Server size={18} />
                <span>
                  保存后立即生效，无需重启服务。新建影片会使用当前模型配置。
                </span>
              </div>
              <button className="primary-button" disabled={busy} type="submit">
                {busy ? (
                  <LoaderCircle size={16} className="spin" />
                ) : (
                  <Save size={16} />
                )}
                保存配置
              </button>
            </footer>
          </form>
        </main>
      </div>
    </Toaster>
  );
}
