'use client';
/* oxlint-disable next/no-img-element -- Project media uses authenticated URLs and the existing thumbnail pipeline. */

import { useCallback, useEffect, useRef, useState } from 'react';
import Link from 'next/link';
import { BrandLogo } from '@/components/brand-logo';
import {
  Aperture,
  ArrowRight,
  Check,
  ChevronRight,
  Clapperboard,
  Download,
  Film,
  FolderOpen,
  ImagePlus,
  LoaderCircle,
  Maximize2,
  Music2,
  Plus,
  RefreshCw,
  Settings2,
  Sparkles,
  Upload,
  WandSparkles,
  X,
} from 'lucide-react';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Progress } from '@/components/ui/progress';
import { Sidebar, SidebarProvider } from '@/components/ui/sidebar';
import { Empty } from '@/components/ui/empty';
import { Toaster, toast } from '@/components/ui/toast';
import { DirectorSettings } from '@/components/director-settings';
import { ChoiceSelect } from '@/components/choice-select';
import { api } from '@/lib/api';
import { TaskQuote } from '@/components/task-quote';
import { useAccount } from '@/components/account-provider';
import type { Quote } from '@/lib/platform';
import {
  creativeDefaults,
  occasions,
  projectDraft,
  changeScene,
  sceneChoices,
  sceneChoice,
  styleChoices,
} from '@/lib/creative';
import type { Project, Shot, StudioConfig } from '@/lib/types';

const styles: Record<string, string> = {
  joyful: '欢快庆典',
  romantic: '浪漫电影',
  epic: '史诗仪式',
  travel: '旅行纪实',
  editorial: '时尚短片',
  garden: '花园电影',
  seaside: '海边誓约',
  vintage: '复古胶片',
};
const transitions: Record<string, string> = {
  cut: '节拍切接',
  match: '动作 / 构图衔接',
  dissolve: '情绪叠化',
  dipwhite: '闪光转场',
};
const statuses: Record<string, string> = {
  draft: '等待创作',
  planning: '正在编排',
  planned: '分镜已就绪',
  generating: '正在生成',
  rendering: '正在剪辑',
  completed: '成片已就绪',
  failed: '需要处理',
  cancelled: '已暂停',
};
const shotStatuses: Record<string, string> = {
  pending: '待生成',
  submitting: '提交中',
  running: '生成中',
  completed: '已完成',
  failed: '需要重试',
};
const clock = (seconds: number) =>
  `${Math.floor(seconds / 60)
    .toString()
    .padStart(2, '0')}:${Math.floor(seconds % 60)
    .toString()
    .padStart(2, '0')}`;
function captionsURL(project: Project) {
  const time = (s: number) =>
    new Date(Math.max(0, s) * 1000).toISOString().slice(11, 23);
  const cues = project.shots
    .filter((s) => s.caption.trim())
    .map(
      (s) =>
        `${time(s.timelineStart + 1)} --> ${time(s.timelineStart + s.editSeconds - 1)}\n${s.caption.replaceAll('-->', '—')}`,
    )
    .join('\n\n');
  const intro =
    project.scene === 'commerce'
      ? ''
      : '00:00:00.000 --> 00:00:01.000\n[背景配乐]\n\n';
  return `data:text/vtt;charset=utf-8,${encodeURIComponent(`WEBVTT\n\n${intro}${cues}\n`)}`;
}

export default function Studio() {
  const { auth, refresh: refreshAccount } = useAccount();
  const canWrite = !!auth?.permissions.includes('project:write');
  const [quote, setQuote] = useState<Quote | null>(null);
  const [projects, setProjects] = useState<Project[]>([]);
  const [project, setProject] = useState<Project | null>(null);
  const [config, setConfig] = useState<StudioConfig | null>(null);
  const [offline, setOffline] = useState(false);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [selectedShot, setSelectedShot] = useState<Shot | null>(null);
  const [editPrompt, setEditPrompt] = useState('');
  const [tab, setTab] = useState('storyboard');
  const [uploadRole, setUploadRole] = useState('reference');
  const [bindings, setBindings] = useState<Record<string, string>>({});
  const [playback, setPlayback] = useState(0);
  const [draft, setDraft] = useState({
    ...creativeDefaults,
    title: '',
    brief: '',
    duration: 60,
    style: 'joyful',
    ratio: '16:9',
  });
  const draftProjectID = useRef<string | null>(null);
  const directorNotes = useRef<HTMLElement>(null);
  const uploadInput = useRef<HTMLInputElement>(null);
  const video = useRef<HTMLVideoElement>(null);
  const activeID = useRef<string | null>(null);
  const running =
    !!project &&
    ['planning', 'generating', 'rendering'].includes(project.status);
  const refresh = useCallback(async (id?: string) => {
    const list = await api<Project[]>('projects');
    setProjects(list);
    const selected = id || activeID.current || list[0]?.id;
    if (selected) {
      const p = await api<Project>(`projects/${selected}`);
      activeID.current = p.id;
      setProject(p);
    }
  }, []);
  useEffect(() => {
    void Promise.all([api<Project[]>('projects'), api<StudioConfig>('config')])
      .then(([list, settings]) => {
        setProjects(list);
        setConfig(settings);
        const requested = new URLSearchParams(window.location.search).get(
          'project',
        );
        const first = list.find((p) => p.id === requested) || list[0];
        if (first) {
          activeID.current = first.id;
          setProject(first);
        }
      })
      .catch(async (e) => {
        try {
          const response = await fetch('/demo/project.json');
          if (!response.ok) throw e;
          const saved = (await response.json()) as Project;
          activeID.current = saved.id;
          setProjects([saved]);
          setProject(saved);
          setOffline(true);
          setConfig({
            connected: false,
            baseUrl: '',
            llmModel: saved.llmModel || 'openai/gpt-6-astra',
            videoModel: saved.videoModel || '',
            maxDuration: 240,
            generationConcurrency: 2,
          });
        } catch {
          setError(e.message);
        }
      });
  }, [refresh]);
  useEffect(() => {
    if (!running) return;
    const timer = setInterval(() => {
      void refresh().catch((e) => setError(e.message));
    }, 3500);
    return () => clearInterval(timer);
  }, [running, refresh]);
  const setNotice = (title: string) =>
    toast.add({ title, type: 'success', timeout: 5000 });
  async function action(path: string) {
    if (offline || !canWrite) return;
    setBusy(true);
    setError('');
    try {
      if (!path.endsWith('/cancel')) {
        if (draftDirty) {
          openSettings();
          throw new Error('请先保存导演手记中的修改，再开始编排或生成');
        }
        const parts = path.split('/');
        const q = await api<Quote>('billing/quote', {
          method: 'POST',
          body: JSON.stringify({
            projectId: parts[1],
            action: parts[parts.length - 1],
          }),
        });
        await refreshAccount();
        setQuote(q);
        return;
      }
      const p = await api<Project>(path, { method: 'POST', body: '{}' });
      activeID.current = p.id;
      setProject(p);
      await refresh(p.id);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  useEffect(() => {
    if (project && draftProjectID.current !== project.id) {
      setDraft(projectDraft(project));
      draftProjectID.current = project.id;
    }
  }, [project]);
  const draftDirty =
    !!project &&
    JSON.stringify(draft) !== JSON.stringify(projectDraft(project));
  function openSettings() {
    directorNotes.current?.scrollIntoView({
      behavior: 'smooth',
      block: 'start',
    });
    directorNotes.current
      ?.querySelector<HTMLTextAreaElement>('textarea')
      ?.focus({ preventScroll: true });
  }
  async function saveProject(e: React.SyntheticEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!project || !canWrite || offline || running || busy || !draftDirty)
      return;
    setBusy(true);
    setError('');
    try {
      const p = await api<Project>(`projects/${project.id}`, {
        method: 'PATCH',
        body: JSON.stringify(draft),
      });
      await refresh(p.id);
      setDraft(projectDraft(p));
      setNotice('工程已保存');
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function upload(files: FileList | null) {
    if (!files || !project) return;
    setBusy(true);
    setError('');
    try {
      for (const file of Array.from(files)) {
        const form = new FormData();
        form.append('file', file);
        form.append('role', uploadRole);
        await api(`projects/${project.id}/assets`, {
          method: 'POST',
          body: form,
        });
      }
      await refresh();
      setNotice('素材已添加');
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
      if (uploadInput.current) uploadInput.current.value = '';
    }
  }
  async function bindAsset(id: string) {
    if (!project) return;
    setBusy(true);
    setError('');
    try {
      await api(`projects/${project.id}/assets/${id}`, {
        method: 'PATCH',
        body: JSON.stringify({ providerAssetId: bindings[id] }),
      });
      await refresh();
      setNotice('人物素材已验证并绑定');
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function saveShot(regenerate: boolean) {
    if (!project || !selectedShot) return;
    setBusy(true);
    setError('');
    try {
      await api(`projects/${project.id}/shots/${selectedShot.id}`, {
        method: 'PATCH',
        body: JSON.stringify({ prompt: editPrompt }),
      });
      if (regenerate) {
        const q = await api<Quote>('billing/quote', {
          method: 'POST',
          body: JSON.stringify({
            projectId: project.id,
            action: 'shot',
            shotId: selectedShot.id,
          }),
        });
        await refreshAccount();
        setQuote(q);
      }
      setSelectedShot(null);
      await refresh();
      setNotice(
        regenerate
          ? '指令已保存，请确认重做报价'
          : '指令已保存，生成后将更新影片',
      );
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  useEffect(() => {
    const context = (
      document as Document & {
        modelContext?: {
          registerTool: (
            tool: unknown,
            options: { signal: AbortSignal },
          ) => unknown;
        };
      }
    ).modelContext;
    if (!context?.registerTool) return;
    const lifecycle = new AbortController();
    const register = (tool: unknown) => {
      try {
        void Promise.resolve(
          context.registerTool(tool, { signal: lifecycle.signal }),
        ).catch(() => {});
      } catch {}
    };
    register({
      name: 'read_film_project',
      title: '查看影片工程',
      description:
        'Read the current film project and its real generation status.',
      inputSchema: {
        type: 'object',
        properties: {},
        additionalProperties: false,
      },
      annotations: { readOnlyHint: true },
      execute: async () =>
        activeID.current ? api(`projects/${activeID.current}`) : null,
    });
    register({
      name: 'create_film_project',
      title: '创建影片工程',
      description:
        'Create and open a draft film project. Does not start paid generation.',
      inputSchema: {
        type: 'object',
        properties: {
          title: {
            type: 'string',
            maxLength: 80,
            description: 'Optional; leave blank for automatic naming.',
          },
          brief: { type: 'string' },
          customPrompt: { type: 'string', maxLength: 6000 },
          scene: { type: 'string', enum: sceneChoices.map((s) => s.id) },
          occasion: { type: 'string', enum: Object.keys(occasions) },
          wardrobeMode: { type: 'string', enum: ['auto', 'fixed', 'custom'] },
          wardrobePrompt: { type: 'string', maxLength: 1500 },
          endingText: { type: 'string', maxLength: 20 },
          duration: { type: 'integer', enum: [15, 60, 120, 180, 240] },
        },
        required: [],
        additionalProperties: false,
      },
      annotations: { readOnlyHint: false },
      execute: async (input: unknown) => {
        const value = input as {
          title?: string;
          brief?: string;
          customPrompt?: string;
          scene?: string;
          occasion?: string;
          wardrobeMode?: string;
          wardrobePrompt?: string;
          endingText?: string;
          duration?: number;
        };
        if (
          !value ||
          (value.title !== undefined && typeof value.title !== 'string') ||
          (value.duration !== undefined &&
            !(value.scene === 'commerce' ? [15] : [60, 120, 180, 240]).includes(
              value.duration,
            ))
        )
          throw new Error('请检查影片名称和时长格式');
        const p = await api<Project>('projects', {
          method: 'POST',
          body: JSON.stringify({
            ...value,
            style: sceneChoice(value.scene).style,
            ratio: sceneChoice(value.scene).ratio,
          }),
        });
        await refresh(p.id);
        return { id: p.id, status: p.status };
      },
    });
    return () => lifecycle.abort();
  }, [refresh]);
  const stage = !project
    ? 0
    : project.status === 'planning'
      ? 0
      : project.status === 'planned'
        ? 1
        : project.status === 'generating'
          ? 2
          : ['rendering', 'completed'].includes(project.status)
            ? 3
            : 0;
  const completed =
    project?.shots.filter((s) => s.status === 'completed').length || 0;
  const primaryLabel =
    project?.status === 'completed'
      ? '重新合成'
      : project?.status === 'failed' || project?.status === 'cancelled'
        ? '继续生成'
        : '一键生成影片';
  const editShot = (s: Shot) => {
    setSelectedShot(s);
    setEditPrompt(s.prompt);
  };

  return (
    <Toaster>
      <TaskQuote
        key={quote?.id || 'closed'}
        quote={quote}
        onClose={() => setQuote(null)}
        onComplete={() => refresh()}
      />
      <div className="studio-shell">
        <header className="topbar">
          <Link className="brand" href="/" aria-label="誓光创作工作台">
            <BrandLogo />
          </Link>
          <div className="breadcrumb">
            创作空间 <ChevronRight size={14} />
            <span>{project?.title || '一个新故事'}</span>
          </div>
          <div className="header-actions">
            <span className="connection">
              <i className={config?.connected ? 'online' : ''} />
              {config?.connected ? '创作引擎已连接' : '连接创作引擎'}
            </span>
            {!offline && auth?.permissions.includes('config:manage') && (
              <Link className="secondary-button settings-link" href="/settings">
                <Settings2 size={16} />
                <span>引擎配置</span>
              </Link>
            )}
            <button
              className="avatar"
              onClick={openSettings}
              aria-label="项目设置"
            >
              V
            </button>
          </div>
        </header>
        <SidebarProvider className="workspace">
          <Sidebar
            collapsible="none"
            className="project-rail"
            aria-label="影片列表"
          >
            <div className="rail-top">
              <span className="eyebrow">我的影片</span>
              {offline || !config || !canWrite ? (
                <button className="icon-button" aria-label="新建影片" disabled>
                  <Plus size={18} />
                </button>
              ) : (
                <Link className="icon-button" href="/new" aria-label="新建影片">
                  <Plus size={18} />
                </Link>
              )}
            </div>
            <div className="project-list">
              {projects.map((p) => (
                <button
                  className={`project-card ${p.id === project?.id ? 'active' : ''}`}
                  key={p.id}
                  onClick={() => {
                    setPlayback(0);
                    if (offline) return;
                    void refresh(p.id).catch((e) => setError(e.message));
                  }}
                >
                  <div className="project-mini">
                    {p.posterUrl ? (
                      <img src={p.posterUrl} alt="" />
                    ) : (
                      <Film size={20} />
                    )}
                  </div>
                  <span>
                    <strong>{p.title}</strong>
                    <small>
                      {clock(p.duration)} · {statuses[p.status]}
                    </small>
                  </span>
                </button>
              ))}
            </div>
            {offline || !config || !canWrite ? (
              <button className="new-project" disabled>
                <Plus size={16} />
                新建影片
              </button>
            ) : (
              <Link className="new-project" href="/new">
                <Plus size={16} />
                新建影片
              </Link>
            )}
            <div className="rail-footer">
              <Aperture size={20} />
              <p>
                让值得铭记的时刻
                <br />
                <strong>成为一部电影。</strong>
              </p>
              <span>VOWFILM STUDIO / 01</span>
            </div>
          </Sidebar>
          <main className="main-workspace">
            <div className="project-heading">
              <div>
                <div className="project-scene-field">
                  <label htmlFor="workspace-scene">创作场景</label>
                  <ChoiceSelect
                    id="workspace-scene"
                    label="创作场景"
                    value={draft.scene}
                    onChange={(scene) =>
                      setDraft((value) => changeScene(value, scene))
                    }
                    items={sceneChoices.map((scene) => ({
                      value: scene.id,
                      label: scene.name,
                    }))}
                    disabled={
                      !project || !canWrite || offline || running || busy
                    }
                  />
                  {project && draft.scene !== (project.scene || 'wedding') && (
                    <output className="scene-draft-hint">未保存</output>
                  )}
                </div>
                <h1>{project?.title || '开启你的第一部影片'}</h1>
                <div className="project-meta">
                  <span>{styles[project?.style || 'garden']}</span>
                  <b>·</b>
                  <span>{clock(project?.duration || 60)}</span>
                  <b>·</b>
                  <span>{project?.ratio || '16:9'}</span>
                  {project?.demo && (
                    <span className="demo-tag">创意演示工程</span>
                  )}
                </div>
              </div>
              <div className="heading-actions">
                {offline || !config || !canWrite ? (
                  <button
                    className="secondary-button mobile-new"
                    aria-label="新建影片"
                    disabled
                  >
                    <Plus size={16} />
                  </button>
                ) : (
                  <Link
                    className="secondary-button mobile-new"
                    href="/new"
                    aria-label="新建影片"
                  >
                    <Plus size={16} />
                  </Link>
                )}
              </div>
            </div>
            {offline && (
              <output className="message offline">
                <span>
                  生成服务暂时离线。你仍可观看、下载这部演示成片和分镜脚本。
                </span>
                <button
                  className="text-button"
                  onClick={() => window.location.reload()}
                >
                  重新连接
                </button>
              </output>
            )}
            {error && (
              <div className="message error" role="alert">
                <span>{error}</span>
                <button onClick={() => setError('')} aria-label="关闭提示">
                  <X size={16} />
                </button>
              </div>
            )}
            <div className="stage-bar">
              {(project?.scene === 'commerce'
                ? ['广告创意', '完整脚本', '15秒直出', '成片就绪']
                : ['故事编排', '分镜设计', '视频生成', '剪辑成片']
              ).map((label, i) => (
                <div
                  key={label}
                  className={`stage ${i === stage ? 'current' : ''} ${i < stage || project?.status === 'completed' ? 'done' : ''}`}
                >
                  <span>
                    {i < stage || project?.status === 'completed' ? (
                      <Check size={13} />
                    ) : (
                      `0${i + 1}`
                    )}
                  </span>
                  <strong>{label}</strong>
                  {i < 3 && <div className="stage-line" />}
                </div>
              ))}
            </div>
            <div className="creation-grid">
              <section className="viewer-panel" aria-label="影片预览">
                <div className="panel-top">
                  <span>
                    <span className="small-dot" />
                    {project?.filmUrl ? '成片预览' : '创作预览'}
                  </span>
                  <span>
                    {(project?.videoModel || config?.videoModel)?.includes(
                      'mini',
                    )
                      ? 'SD2 MINI'
                      : 'SEEDANCE'}{' '}
                    <b> / </b> 720P
                  </span>
                </div>
                <div className="film-viewer">
                  {project?.filmUrl ? (
                    <video
                      ref={video}
                      key={project.filmUrl}
                      src={project.filmUrl}
                      poster={project.posterUrl || undefined}
                      controls
                      loop={project.occasion === 'warmup'}
                      playsInline
                      preload="metadata"
                      onTimeUpdate={(e) =>
                        setPlayback(e.currentTarget.currentTime)
                      }
                      aria-label={project.title}
                    >
                      <track
                        kind="captions"
                        src={captionsURL(project)}
                        srcLang="zh"
                        label="中文心语与音乐"
                      />
                    </video>
                  ) : project?.posterUrl ? (
                    <>
                      <img
                        className="film-poster"
                        src={project.posterUrl}
                        alt="影片画面"
                      />
                      <div className="poster-caption">
                        <span>VOWFILM ORIGINAL</span>
                        <h2>{project.title}</h2>
                        <p>
                          {running
                            ? '让故事，一帧一帧发生。'
                            : '故事就绪，等待光影发生。'}
                        </p>
                      </div>
                    </>
                  ) : (
                    <Empty className="preview-empty">
                      <Clapperboard size={36} strokeWidth={1} />
                      <p>故事的下一帧，由你开启</p>
                      <span>添加人物、商品或场景参考，开始创作</span>
                    </Empty>
                  )}
                  {running && (
                    <div className="render-status">
                      <LoaderCircle size={15} className="spin" />
                      <span>{project?.message}</span>
                    </div>
                  )}
                </div>
                <div className="viewer-bottom">
                  <div>
                    <Film size={15} />
                    <span>
                      {project?.filmUrl
                        ? '影片已完成'
                        : `${completed} / ${project?.shots.length || 0} 个镜头已生成`}
                    </span>
                  </div>
                  <span className="timecode">
                    {clock(playback)} <i>/</i> {clock(project?.duration || 60)}
                  </span>
                  <button
                    className="icon-button"
                    aria-label="全屏播放"
                    disabled={!project?.filmUrl}
                    onClick={() => {
                      void video.current?.requestFullscreen();
                    }}
                  >
                    <Maximize2 size={17} />
                  </button>
                </div>
              </section>
              <aside
                className="director-panel"
                id="director-notes"
                ref={directorNotes}
              >
                <div className="panel-top">
                  <span>
                    <Sparkles size={15} />
                    导演手记
                  </span>
                  <span>
                    {config?.llmModel.includes('gpt-6') ? 'GPT‑6' : 'STORY'}
                  </span>
                </div>
                <div className="director-body">
                  {project?.synopsis && (
                    <p className="story-text">{project.synopsis}</p>
                  )}
                  {project && (
                    <DirectorSettings
                      key={project.id}
                      draft={draft}
                      setDraft={setDraft}
                      onSubmit={saveProject}
                      disabled={!canWrite || offline || running}
                      busy={busy}
                      dirty={draftDirty}
                      error={error}
                    />
                  )}
                  <div className="generation-area">
                    {running ? (
                      <>
                        <div className="generation-label">
                          <span>{statuses[project?.status || '']}</span>
                          <strong>{project?.progress}%</strong>
                        </div>
                        <Progress
                          value={project?.progress || 0}
                          aria-label="影片生成进度"
                        />
                        <button
                          className="secondary-button full-width"
                          disabled={!canWrite || busy}
                          onClick={() =>
                            project &&
                            void action(`projects/${project.id}/cancel`)
                          }
                        >
                          暂停创作
                        </button>
                      </>
                    ) : (
                      <>
                        <button
                          className="primary-button full-width"
                          disabled={
                            !canWrite || busy || !project || !config?.connected
                          }
                          onClick={() =>
                            project &&
                            void action(
                              `projects/${project.id}/${project.status === 'completed' ? 'render' : 'generate'}`,
                            )
                          }
                        >
                          {busy ? (
                            <LoaderCircle size={17} className="spin" />
                          ) : (
                            <WandSparkles size={17} />
                          )}
                          {primaryLabel}
                          <ArrowRight size={16} />
                        </button>
                        <p className="generation-note">
                          {project?.status === 'completed'
                            ? '保留现有镜头，重新输出影片'
                            : project?.scene === 'commerce'
                              ? '完整广告脚本 · 15秒直出 · 原生声音'
                              : '自动编排 · 生成镜头 · 配乐剪辑'}
                        </p>
                      </>
                    )}
                  </div>
                </div>
              </aside>
            </div>
            <Tabs
              value={tab}
              onValueChange={(v) => setTab(String(v))}
              className="content-tabs"
            >
              <div className="tabs-toolbar">
                <TabsList variant="line">
                  <TabsTrigger value="storyboard">
                    <Clapperboard size={16} />
                    {project?.scene === 'commerce'
                      ? '整片脚本'
                      : '分镜脚本'}{' '}
                    <span className="tab-count">
                      {project?.shots.length || 0}
                    </span>
                  </TabsTrigger>
                  <TabsTrigger value="treatment">
                    <WandSparkles size={16} />
                    导演方案
                  </TabsTrigger>
                  <TabsTrigger value="assets">
                    <FolderOpen size={16} />
                    素材库{' '}
                    <span className="tab-count">
                      {project?.assets.length || 0}
                    </span>
                  </TabsTrigger>
                  <TabsTrigger value="activity">
                    <Aperture size={16} />
                    制作记录
                  </TabsTrigger>
                </TabsList>
                {(tab === 'storyboard' || tab === 'treatment') && (
                  <button
                    className="text-button"
                    disabled={
                      !canWrite || offline || busy || running || !project
                    }
                    onClick={() =>
                      project && void action(`projects/${project.id}/plan`)
                    }
                  >
                    <RefreshCw size={14} />
                    重新编排
                  </button>
                )}
              </div>
              <TabsContent value="storyboard">
                {project?.scene === 'commerce' &&
                  project.shots.length > 0 &&
                  project.generationMode !== 'commerce-direct-15s-v3' && (
                    <p className="fine-print">
                      当前是旧版分镜。请确认导演手记的时长为15秒，保存后点击「重新编排」使用
                      v3。
                    </p>
                  )}
                <div className="section-description">
                  <span>
                    {project?.scene === 'commerce'
                      ? '一条完整指令，生成15秒广告与原生声音。'
                      : '每一个镜头，都让故事向前一步。'}
                  </span>
                  <span>
                    {project?.scene === 'commerce'
                      ? '点击卡片编辑完整指令或重新生成整条广告'
                      : '点击镜头可编辑指令或局部重做'}
                  </span>
                </div>
                <div className="shot-grid">
                  {project?.shots.map((shot, i) => (
                    <button
                      className="shot-card"
                      key={shot.id}
                      onClick={() => editShot(shot)}
                    >
                      <div className="shot-image">
                        {shot.thumbnailUrl ? (
                          <img
                            src={shot.thumbnailUrl}
                            alt={shot.title}
                            loading="lazy"
                          />
                        ) : (
                          <div className="shot-placeholder">
                            <Clapperboard size={25} strokeWidth={1} />
                          </div>
                        )}
                        <span className="shot-number">
                          {String(i + 1).padStart(2, '0')}
                        </span>
                        <span className={`shot-state ${shot.status}`}>
                          {shot.status === 'completed' && <Check size={11} />}
                          {shotStatuses[shot.status]}
                        </span>
                        <span className="shot-duration">
                          {shot.editSeconds?.toFixed(1) || shot.duration}s
                        </span>
                      </div>
                      <div className="shot-copy">
                        <span className="shot-chapter">
                          {shot.chapter}
                          {shot.lookId &&
                            ` · ${project.treatment?.looks.find((look) => look.id === shot.lookId)?.name || shot.lookId}`}
                        </span>
                        <h3>{shot.title}</h3>
                        <p>{shot.description}</p>
                        <div>
                          <span>{shot.camera}</span>
                          <span>
                            {shot.changeToLookId
                              ? '章节变装'
                              : transitions[shot.transition] || '自然切接'}{' '}
                            <ArrowRight size={11} />
                          </span>
                        </div>
                      </div>
                    </button>
                  ))}
                </div>
                {!project?.shots.length && (
                  <Empty className="empty-section">
                    <Clapperboard size={30} />
                    <p>还没有分镜</p>
                    <span>写下你的想法，让导演安排第一个镜头。</span>
                    <button
                      className="secondary-button"
                      disabled={
                        !canWrite || offline || !project || busy || running
                      }
                      onClick={() =>
                        project && void action(`projects/${project.id}/plan`)
                      }
                    >
                      <Sparkles size={15} />
                      编排分镜
                    </button>
                  </Empty>
                )}
              </TabsContent>
              <TabsContent value="treatment">
                <section className="treatment-panel">
                  <div className="treatment-heading">
                    <div>
                      <span className="chapter-label">为当前场景编排</span>
                      <h2>{occasions[project?.occasion || 'opening']}</h2>
                    </div>
                    <button
                      className="secondary-button"
                      disabled={!canWrite || offline || running || busy}
                      onClick={openSettings}
                    >
                      <Settings2 size={15} />
                      编辑导演手记
                    </button>
                  </div>
                  {project?.customPrompt && (
                    <div className="director-request">
                      <strong>你的 Prompt</strong>
                      <p>{project.customPrompt}</p>
                    </div>
                  )}
                  {project?.scene === 'commerce' &&
                  project.generationMode === 'commerce-direct-15s-v3' &&
                  project.shots[0] ? (
                    <div className="director-request">
                      <strong>15秒完整广告 Prompt · v3</strong>
                      <p style={{ whiteSpace: 'pre-wrap' }}>
                        {project.shots[0].prompt}
                      </p>
                      <button
                        className="secondary-button"
                        onClick={() => editShot(project.shots[0])}
                      >
                        编辑完整指令
                      </button>
                    </div>
                  ) : project?.treatment ? (
                    <>
                      <p className="treatment-concept">
                        {project.treatment.concept}
                      </p>
                      <div className="treatment-cues">
                        <p>
                          <strong>开头如何抓住注意</strong>
                          {project.treatment.openingHook}
                        </p>
                        <p>
                          <strong>如何交还现场</strong>
                          {project.treatment.closingLine}
                          {(!project.scene || project.scene === 'wedding') &&
                            project.occasion !== 'warmup' && (
                              <small>
                                片尾留画 {project.occasion === 'story' ? 2 : 3}{' '}
                                秒，音乐降至安静
                              </small>
                            )}
                        </p>
                      </div>
                      <div className="treatment-acts">
                        {project.treatment.acts.map((act, i) => {
                          const look = project.treatment?.looks.find(
                            (look) => look.id === act.lookId,
                          );
                          const next = project.treatment?.acts[i + 1];
                          return (
                            <article key={act.id}>
                              <div className="act-heading">
                                <span>{String(i + 1).padStart(2, '0')}</span>
                                <div>
                                  <h3>{act.title}</h3>
                                  <small>
                                    {clock(act.start)}–{clock(act.end)} · 镜头{' '}
                                    {act.firstShot}–{act.lastShot}
                                  </small>
                                </div>
                              </div>
                              <p>{act.storyBeat}</p>
                              <div className="act-setting">{act.setting}</div>
                              <div className="look-detail">
                                <strong>{look?.name}</strong>
                                <p>
                                  {project.scene === 'commerce'
                                    ? '商品'
                                    : '主要人物'}{' '}
                                  · {look?.bride}
                                </p>
                                <p>
                                  {project.scene === 'commerce'
                                    ? '环境'
                                    : '其他人物'}{' '}
                                  · {look?.groom}
                                </p>
                              </div>
                              {next && (
                                <div className="wardrobe-bridge">
                                  {next.lookId !== act.lookId
                                    ? '换装衔接'
                                    : '章节衔接'}{' '}
                                  ·{' '}
                                  {{
                                    veil: '前景白纱遮挡',
                                    spin: '同方向转身',
                                    prop: '同一道具特写',
                                    cut: '自然切接',
                                  }[act.bridge] || '动作匹配'}
                                </div>
                              )}
                            </article>
                          );
                        })}
                      </div>
                      <div className="treatment-cues">
                        <p>
                          <strong>已采纳要求</strong>
                          {project.treatment.mustHave.join('；') ||
                            '按故事与风格规划'}
                        </p>
                        <p>
                          <strong>避免出现</strong>
                          {project.treatment.avoid.join('；') ||
                            '不编造真实经历，不在同镜头内换脸变装'}
                        </p>
                      </div>
                      <div className="director-request">
                        <strong>
                          音乐方向 · 目标 {project.treatment.bpm} BPM
                        </strong>
                        <p>{project.treatment.musicDirection}</p>
                      </div>
                      {!!project.treatment.notes?.length && (
                        <div className="treatment-notes">
                          <strong>导演说明</strong>
                          {project.treatment.notes.map((note, i) => (
                            <p key={i}>{note}</p>
                          ))}
                        </div>
                      )}
                      <p className="fine-print">
                        这是生成前的导演方案。造型按章固定、换装点编入相邻镜头；实际人物一致性和遮挡效果仍需查看生成画面。
                      </p>
                    </>
                  ) : (
                    <Empty>
                      <WandSparkles size={28} />
                      <p>
                        {project?.scene === 'commerce'
                          ? '先写一条完整的15秒广告指令'
                          : '先把故事和造型排成一部影片'}
                      </p>
                      <span>
                        {project?.scene === 'commerce'
                          ? '编排后可查看素材职责、完整时间轴、动作和台词，再提交一次视频生成。'
                          : '编排后可查看三至四章叙事、每章造型、换装衔接与 Prompt 的采纳情况。'}
                      </span>
                      <button
                        className="primary-button"
                        disabled={
                          !canWrite || offline || running || busy || !project
                        }
                        onClick={() =>
                          project && void action(`projects/${project.id}/plan`)
                        }
                      >
                        生成导演方案与分镜
                      </button>
                    </Empty>
                  )}
                </section>
              </TabsContent>
              <TabsContent value="assets">
                <div className="asset-toolbar">
                  <div>
                    <h2>故事的起点</h2>
                    <p>
                      上传人物、商品、场景或音乐素材。真人照片需绑定已授权的人物素材。
                    </p>
                  </div>
                  <div className="upload-controls">
                    <ChoiceSelect
                      value={uploadRole}
                      onChange={setUploadRole}
                      label="素材用途"
                      items={[
                        { value: 'reference', label: '场景参考' },
                        ...(project?.scene === 'wedding' || !project?.scene
                          ? [
                              { value: 'bride', label: '新娘照片' },
                              { value: 'groom', label: '新郎照片' },
                            ]
                          : [{ value: 'person', label: '人物照片' }]),
                        { value: 'product', label: '商品照片' },

                        { value: 'music', label: '背景音乐' },
                      ]}
                    />
                    <button
                      className="secondary-button"
                      disabled={
                        !canWrite || offline || busy || running || !project
                      }
                      onClick={() => uploadInput.current?.click()}
                    >
                      <Upload size={16} />
                      上传素材
                    </button>
                    <input
                      ref={uploadInput}
                      type="file"
                      hidden
                      multiple
                      accept={
                        uploadRole === 'music'
                          ? 'audio/mpeg,audio/wav,audio/ogg,audio/mp4'
                          : 'image/jpeg,image/png,image/webp'
                      }
                      onChange={(e) => void upload(e.target.files)}
                    />
                  </div>
                </div>
                <div className="asset-grid">
                  {project?.assets.map((asset) => (
                    <div key={asset.id} className="asset-card">
                      {asset.role === 'music' ? (
                        <div className="asset-audio">
                          <Music2 size={30} />
                        </div>
                      ) : (
                        <img src={asset.url} alt={asset.name} loading="lazy" />
                      )}
                      <div className="asset-info">
                        <strong>{asset.name}</strong>
                        <small>
                          {
                            {
                              person: '人物',
                              product: '商品',
                              bride: '新娘',
                              groom: '新郎',
                              reference: '场景参考',
                              music: '背景音乐',
                            }[asset.role]
                          }
                        </small>
                        {['bride', 'groom', 'person'].includes(asset.role) &&
                          (asset.providerAssetId ? (
                            <span className="asset-ready">
                              <Check size={12} />
                              已绑定人物素材
                            </span>
                          ) : (
                            <div className="asset-bind">
                              <label htmlFor={`bind-${asset.id}`}>
                                已授权素材 ID
                              </label>
                              <input
                                id={`bind-${asset.id}`}
                                value={bindings[asset.id] || ''}
                                placeholder="asset://asset-…"
                                onChange={(e) =>
                                  setBindings((prev) => ({
                                    ...prev,
                                    [asset.id]: e.target.value,
                                  }))
                                }
                              />
                              <button
                                className="text-button"
                                disabled={
                                  offline || busy || !bindings[asset.id]
                                }
                                onClick={() => void bindAsset(asset.id)}
                              >
                                验证并绑定 <ArrowRight size={12} />
                              </button>
                            </div>
                          ))}
                      </div>
                    </div>
                  ))}
                </div>
                {!project?.assets.length && (
                  <button
                    className="upload-zone"
                    disabled={
                      !canWrite || offline || !project || busy || running
                    }
                    onClick={() => uploadInput.current?.click()}
                  >
                    <ImagePlus size={28} strokeWidth={1.3} />
                    <strong>把照片放进你们的故事</strong>
                    <span>JPG、PNG、WebP · 每个文件不超过 20 MB</span>
                  </button>
                )}
                <p className="asset-help">
                  真人照片请先完成{' '}
                  <a
                    href="https://www.volcengine.com/docs/82379/2315856?lang=zh"
                    target="_blank"
                    rel="noreferrer"
                  >
                    本人授权与素材入库
                  </a>
                  ，然后绑定素材 ID。
                </p>
              </TabsContent>
              <TabsContent value="activity">
                <div className="activity-list">
                  {project?.events
                    .slice()
                    .reverse()
                    .map((event, i) => (
                      <div className="activity-item" key={`${event.at}-${i}`}>
                        <span className="activity-dot" />
                        <div>
                          <strong>{event.message}</strong>
                          <small>
                            {new Date(event.at).toLocaleString('zh-CN', {
                              hour12: false,
                            })}
                          </small>
                        </div>
                      </div>
                    ))}
                </div>
              </TabsContent>
            </Tabs>
            {project?.shots.length ? (
              <section className="timeline-section">
                <div className="timeline-title">
                  <span>
                    <Film size={15} />
                    影片时间线
                  </span>
                  <span>{clock(project.duration)}</span>
                </div>
                <div className="timeline-ruler">
                  {Array.from({ length: 5 }, (_, i) => (
                    <span key={i}>{clock((project.duration * i) / 4)}</span>
                  ))}
                </div>
                <div className="timeline-track">
                  {project.shots.map((shot, i) => (
                    <button
                      key={shot.id}
                      onClick={() => {
                        if (video.current && project.filmUrl) {
                          video.current.currentTime = shot.timelineStart || 0;
                          void video.current.play();
                        } else editShot(shot);
                      }}
                      title={`${shot.title} · ${clock(shot.timelineStart || 0)}`}
                      style={{
                        flex: shot.editSeconds || 8,
                        backgroundImage: shot.thumbnailUrl
                          ? `linear-gradient(0deg,rgba(0,0,0,.55),transparent),url("${shot.thumbnailUrl}")`
                          : undefined,
                      }}
                    >
                      <span>{String(i + 1).padStart(2, '0')}</span>
                    </button>
                  ))}
                </div>
                <div className="music-track">
                  <Music2 size={14} />
                  <span>
                    {project.generationMode === 'commerce-direct-15s-v3'
                      ? '原生声音 · 与画面一起生成'
                      : project.assets.some((a) => a.role === 'music')
                        ? '项目配乐'
                        : project.musicSource === 'sonilo'
                          ? 'AI 配乐 · 为这部影片创作'
                          : '影片配乐'}
                  </span>
                  {project.scene !== 'commerce' && (
                    <span className="score-tempo">
                      目标{' '}
                      {project.treatment?.bpm ||
                        styleChoices.find((s) => s.id === project.style)?.bpm ||
                        96}{' '}
                      BPM
                    </span>
                  )}
                </div>
                {!!project.musicSections?.length && (
                  <div className="score-sections" aria-label="分段配乐设计">
                    {project.musicSections.map((section) => (
                      <button
                        key={section.name}
                        style={{ flex: section.end - section.start }}
                        title={`${section.instruments} · ${clock(section.start)}–${clock(section.end)}`}
                        onClick={() => {
                          if (video.current)
                            video.current.currentTime = section.start;
                        }}
                      >
                        <div
                          className="score-energy"
                          style={{ height: `${section.energy * 0.35 + 8}px` }}
                        />
                        <strong>{section.name}</strong>
                        <small>{clock(section.start)}</small>
                      </button>
                    ))}
                  </div>
                )}
                {(project.musicFile || project.musicUrl) && (
                  <div className="score-audition">
                    <span>单独试听配乐</span>
                    <audio
                      controls
                      preload="none"
                      src={
                        project.musicUrl ||
                        `/api/media/${project.id}/${project.musicFile}`
                      }
                      aria-label="影片配乐试听"
                    >
                      <track
                        kind="captions"
                        src="data:text/vtt,WEBVTT%0A%0A00:00:00.000%20--%3E%2000:04:00.000%0A%5B%E7%BA%AF%E9%9F%B3%E4%B9%90%5D"
                        srcLang="zh"
                        label="纯音乐"
                      />
                    </audio>
                  </div>
                )}
              </section>
            ) : null}
            <footer className="workspace-footer">
              <span>
                <span className="small-dot" />
                {offline
                  ? '演示成片归档'
                  : running
                    ? '正在保存制作进度'
                    : '工程自动保存'}
              </span>
              <div>
                {project?.filmUrl && (
                  <a
                    className="primary-button"
                    href={`${project.filmUrl}?download=1`}
                    download={`${project.title}.mp4`}
                  >
                    <Download size={16} />
                    下载成片
                  </a>
                )}
                {project && (
                  <a
                    className="text-button"
                    href={
                      offline
                        ? '/demo/project.json'
                        : `/api/projects/${project.id}/export`
                    }
                    download={`${project.title}.json`}
                  >
                    导出工程 <ArrowRight size={14} />
                  </a>
                )}
              </div>
            </footer>
          </main>
        </SidebarProvider>
        <Dialog
          open={!!selectedShot}
          onOpenChange={(open) => {
            if (!open) setSelectedShot(null);
          }}
        >
          <DialogContent className="shot-dialog">
            <DialogHeader>
              <DialogTitle>{selectedShot?.title}</DialogTitle>
              <DialogDescription>
                {selectedShot?.chapter} · {selectedShot?.camera}
              </DialogDescription>
            </DialogHeader>
            {selectedShot?.videoUrl && (
              <video
                src={selectedShot.videoUrl}
                muted
                controls
                playsInline
                preload="metadata"
                poster={selectedShot.thumbnailUrl}
              />
            )}
            {selectedShot?.lookId && (
              <div className="shot-look">
                <strong>
                  {project?.treatment?.looks.find(
                    (look) => look.id === selectedShot.lookId,
                  )?.name || selectedShot.lookId}
                </strong>
                <span>
                  {selectedShot.changeToLookId
                    ? `此镜结尾衔接下一套造型：${project?.treatment?.looks.find((look) => look.id === selectedShot.changeToLookId)?.name || selectedShot.changeToLookId}`
                    : '本章内保持这套造型与人物身份'}
                </span>
              </div>
            )}
            {(selectedShot?.entryAction || selectedShot?.transitionReason) && (
              <div className="shot-continuity">
                <p>
                  <strong>入场</strong>
                  {selectedShot.entryAction}
                </p>
                <p>
                  <strong>出场</strong>
                  {selectedShot.exitAction}
                </p>
                <p>
                  <strong>衔接</strong>
                  {selectedShot.transitionReason}
                </p>
              </div>
            )}
            <label className="prompt-label">
              {project?.scene === 'commerce'
                ? '整条广告生成指令'
                : '镜头生成指令'}
              <textarea
                rows={7}
                value={editPrompt}
                onChange={(e) => setEditPrompt(e.target.value)}
                maxLength={10000}
                disabled={!canWrite || offline || running}
              />
            </label>
            {(error || selectedShot?.error) && (
              <p className="inline-error" role="alert">
                {error || selectedShot?.error}
              </p>
            )}
            <div className="dialog-actions">
              <button
                className="secondary-button"
                disabled={!canWrite || offline || busy || running}
                onClick={() => void saveShot(false)}
              >
                保存指令
              </button>
              <button
                className="primary-button"
                disabled={!canWrite || offline || busy || running}
                onClick={() => void saveShot(true)}
              >
                <RefreshCw size={15} />
                {project?.scene === 'commerce'
                  ? '重新生成整条广告'
                  : '重做这个镜头'}
              </button>
            </div>
            <p className="fine-print">
              {project?.scene === 'commerce'
                ? '重新调用视频模型直出完整15秒广告，原生声音也会重新生成。'
                : '重做会调用视频模型。保留其余镜头，完成后可重新合成影片。'}
            </p>
          </DialogContent>
        </Dialog>
      </div>
    </Toaster>
  );
}
