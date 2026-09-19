'use client';
/* oxlint-disable next/no-img-element -- Authenticated production artifacts. */

import { useCallback, useEffect, useRef, useState } from 'react';
import Link from 'next/link';
import {
  ArrowLeft,
  ArrowRight,
  Check,
  Download,
  FileText,
  LoaderCircle,
  RefreshCw,
  Upload,
} from 'lucide-react';
import { BrandLogo } from '@/components/brand-logo';
import { StudioNavLinks } from '@/components/studio-nav-links';
import { TaskQuote } from '@/components/task-quote';
import { useAccount } from '@/components/account-provider';
import { api } from '@/lib/api';
import type { Project, WeddingArtifact } from '@/lib/types';
import type { Quote } from '@/lib/platform';

const guide: Record<
  number,
  {
    description: string;
    kinds: [string, string][];
    action?: string;
    label?: string;
  }
> = {
  1: {
    description:
      '把故事采集卡发给新人，收回经历、照片对应关系和制作要求。没有、记不清、不想呈现都是有效回答。核对关键事实后进入写作。',
    kinds: [
      ['facts', '完整故事采集卡'],
      ['photo', '人物 / 场景参考照片'],
    ],
  },
  2: {
    description:
      '按原始六段写作方法准备完整写作包和旁白初稿。通读事实、自然口播、情绪与结尾；本步由制作方审稿。',
    kinds: [
      ['script', '纯旁白正文'],
      ['writing', '完整写作提示词'],
      ['factmap', '事实来源表 JSON'],
    ],
    action: 'plan',
    label: '生成写作包与初稿',
  },
  3: {
    description:
      '把当前版本的全文发给新人。请核对经历、称呼、需要呈现与避开的内容，并记录新人对这个版本的明确确认。',
    kinds: [['script', '新人确认的全文']],
  },
  4: {
    description:
      '用同一段文字试听 3–5 个实际音色，选择音色、情绪与语速。上传试听音频和记录音色参数的选择表。',
    kinds: [
      ['voice', '音色试听文件'],
      ['selection', '音色选择 JSON'],
    ],
  },
  5: {
    description:
      '按选定音色生成完整旁白，上传原音频和实际语义时间点。完整试听漏字、读音、情绪和停顿；时间点来自音频，不按字数估算。',
    kinds: [
      ['narration', '完整旁白音频'],
      ['timing', '旁白时间点 JSON'],
    ],
  },
  6: {
    description:
      '根据已确认旁白和实际时间点设计镜头。核对每镜事实来源、人物时期、首帧、动作、结尾和衔接，再确认整版分镜。',
    kinds: [['storyboard', '完整分镜 JSON']],
    action: 'plan',
    label: '按旁白设计分镜',
  },
  7: {
    description:
      '从本单挑选约三个能确定人物、画风和关键动作的镜头，复制对应生图词到外部 GPT。上传原图并按镜号核对。',
    kinds: [['image', '重点镜头首帧']],
  },
  8: {
    description:
      '按试图结果生成剩余首帧，逐镜选版。确认完整图片集，包括先前试图的人物一致性、服装、道具与动作空间。',
    kinds: [['image', '选定分镜首帧']],
  },
  9: {
    description:
      '下载逐镜文件包，使用每镜实际首帧与视频词生成。也可使用已配置引擎生成。完整观看核心动作、脸手道具和相邻剪点。',
    kinds: [['video', '逐镜视频']],
    action: 'generate',
    label: '使用选定首帧生成视频',
  },
  10: {
    description:
      '试听约三个不同风格的真实音乐样本，在选择表中填写选中素材的 artifactId。下一步再回传选定风格的全长音乐。',
    kinds: [
      ['music', '音乐风格试听'],
      ['selection', '音乐选择 JSON'],
    ],
  },
  11: {
    description:
      '将选定音乐与旁白混合，先听开场、转折和落点的代表片段。确认人声清晰、音乐起伏和两者音量关系，并保留混音参数。',
    kinds: [
      ['music', '选定风格的全长音乐'],
      ['mix', '混合试听音频'],
      ['settings', '混音参数 JSON'],
    ],
    action: 'render',
    label: '制作三个混音试听片段',
  },
  12: {
    description:
      '使用上一阶段确认的音轨和音量参数制作完整混音。完整听一遍接点、音量与结尾。',
    kinds: [['mix', '全长混音 WAV']],
    action: 'render',
    label: '生成完整混音',
  },
  13: {
    description:
      '组装带字幕的完整预览。请完整观看人物、动作、字幕、旁白与音乐，有问题记录具体镜号或时间点。',
    kinds: [['preview', '完整预览 MP4']],
    action: 'render',
    label: '生成带字幕预览',
  },
  14: {
    description:
      '导出正式成片、字幕和可修改剪辑表。制作方先核对正式导出，再转给新人；两项确认都完成后才结束订单。',
    kinds: [
      ['final', '正式成片 MP4'],
      ['subtitles', '字幕 SRT'],
      ['edit', '剪辑表 JSON'],
    ],
    action: 'render',
    label: '导出正式交付文件',
  },
};

function Artifact({ item }: { item: WeddingArtifact }) {
  return (
    <article className="wedding-artifact">
      <div className="wedding-artifact-heading">
        <strong>
          {item.shotId ? `${item.shotId} · ` : ''}
          {item.name}
        </strong>
        <a href={`${item.url}?download=1`} aria-label={`下载${item.name}`}>
          <Download size={17} />
        </a>
      </div>
      {item.text ? (
        <pre>{item.text}</pre>
      ) : item.mime.startsWith('image/') ? (
        <img
          src={item.url}
          alt={`${item.shotId || ''}选定首帧`}
          loading="lazy"
        />
      ) : item.mime.startsWith('audio/') ||
        item.kind === 'voice' ||
        item.kind === 'music' ||
        item.kind === 'mix' ||
        item.kind === 'narration' ? (
        <audio
          controls
          preload="metadata"
          src={item.url}
          aria-label={item.name}
        >
          <track kind="captions" />
        </audio>
      ) : item.mime.startsWith('video/') ? (
        <video
          controls
          playsInline
          preload="metadata"
          src={item.url}
          aria-label={item.name}
        >
          <track kind="captions" />
        </video>
      ) : (
        <a href={item.url}>查看文件</a>
      )}
      <small>
        素材 ID {item.id} · 版本 {item.sha256} ·{' '}
        {(item.bytes / 1024 / 1024).toFixed(2)} MB
      </small>
    </article>
  );
}

export default function WeddingPage() {
  const { auth } = useAccount();
  const canWrite = !!auth?.permissions.includes('project:write');
  const [project, setProject] = useState<Project | null>(null);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [busy, setBusy] = useState(false);
  const [viewStep, setViewStep] = useState(0);
  const [text, setText] = useState('');
  const [kind, setKind] = useState('');
  const [shotId, setShotId] = useState('');
  const [evidence, setEvidence] = useState('');
  const [reason, setReason] = useState('');
  const [quote, setQuote] = useState<Quote | null>(null);
  const upload = useRef<HTMLInputElement>(null);
  const projectID = useRef('');
  const refresh = useCallback(async () => {
    const id =
      projectID.current ||
      new URLSearchParams(window.location.search).get('project');
    if (!id) {
      setError('请先从模板库创建婚礼故事工程。');
      return;
    }
    projectID.current = id;
    setProject(await api<Project>(`projects/${id}`));
  }, []);
  useEffect(() => {
    void refresh().catch((e) => setError(e.message));
  }, [refresh]);
  const running =
    !!project &&
    ['planning', 'generating', 'rendering'].includes(project.status);
  useEffect(() => {
    if (!running) return;
    const timer = setInterval(
      () => void refresh().catch((e) => setError(e.message)),
      3500,
    );
    return () => clearInterval(timer);
  }, [running, refresh]);
  const workflow = project?.wedding;
  const stepNumber = viewStep || workflow?.currentStep || 1;
  const step = workflow?.steps[stepNumber - 1];
  const current = stepNumber === workflow?.currentStep && !workflow?.completed;
  const spec = guide[stepNumber];
  const selectedKind = spec.kinds.some(([value]) => value === kind)
    ? kind
    : spec.kinds[0][0];
  const textual = [
    'facts',
    'factmap',
    'script',
    'writing',
    'timing',
    'selection',
    'storyboard',
    'settings',
    'edit',
    'subtitles',
  ].includes(selectedKind);
  const needsShot = selectedKind === 'image' || selectedKind === 'video';
  async function save(file?: File) {
    if (!project || !step) return;
    setBusy(true);
    setError('');
    setNotice('');
    try {
      const body = new FormData();
      body.set('step', String(stepNumber));
      body.set('kind', selectedKind);
      if (needsShot) body.set('shotId', shotId || project.shots[0]?.id || '');
      if (file) body.set('file', file);
      else body.set('text', text);
      setProject(
        await api<Project>(`projects/${project.id}/wedding/prepare`, {
          method: 'POST',
          body,
        }),
      );
      setText('');
      setEvidence('');
      setNotice('材料已保存，请检查当前版本。');
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
      if (upload.current) upload.current.value = '';
    }
  }
  async function approve() {
    if (!project || !step) return;
    setBusy(true);
    setError('');
    setNotice('');
    const by =
      stepNumber === 3 ||
      (stepNumber === 14 && step.approvals.some((a) => a.by === 'producer'))
        ? 'couple'
        : 'producer';
    try {
      setProject(
        await api<Project>(`projects/${project.id}/wedding/approve`, {
          method: 'POST',
          body: JSON.stringify({
            step: stepNumber,
            version: step.version,
            by,
            evidence,
          }),
        }),
      );
      setEvidence('');
      setViewStep(0);
      setKind('');
      setText('');
      setNotice('已记录此版本的确认，下一步已就绪。');
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function reopen() {
    if (!project) return;
    setBusy(true);
    setError('');
    try {
      setProject(
        await api<Project>(`projects/${project.id}/wedding/reopen`, {
          method: 'POST',
          body: JSON.stringify({ step: stepNumber, reason }),
        }),
      );
      setViewStep(0);
      setReason('');
      setNotice('已保留旧版本，受影响阶段需要重新确认。');
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function run() {
    if (!project || !spec.action) return;
    setBusy(true);
    setError('');
    try {
      setQuote(
        await api<Quote>('billing/quote', {
          method: 'POST',
          body: JSON.stringify({ projectId: project.id, action: spec.action }),
        }),
      );
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function runAutomatic() {
    if (!project) return;
    setBusy(true);
    setError('');
    setViewStep(0);
    try {
      const updated = await api<Project>(
        `projects/${project.id}/wedding/automatic`,
        { method: 'POST', body: '{}' },
      );
      setProject(updated);
      setQuote(
        await api<Quote>('billing/quote', {
          method: 'POST',
          body: JSON.stringify({ projectId: project.id, action: 'generate' }),
        }),
      );
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  const couple =
    stepNumber === 3 ||
    (stepNumber === 14 && step?.approvals.some((a) => a.by === 'producer'));
  return (
    <div className="studio-shell wedding-shell">
      <TaskQuote
        quote={quote}
        onClose={() => setQuote(null)}
        onComplete={refresh}
      />
      <header className="topbar">
        <Link className="brand" href="/" aria-label="誓光工作台">
          <BrandLogo />
        </Link>
        <div className="breadcrumb">婚礼故事向导</div>
        <div className="header-actions">
          <StudioNavLinks />
          <Link className="secondary-button" href="/new">
            <ArrowLeft size={16} />
            模板库
          </Link>
        </div>
      </header>
      <main className="wedding-page">
        <div className="wedding-heading">
          <div>
            <span className="eyebrow">A STORY ONLY YOU CAN TELL</span>
            <h1>{project?.title || '把日子写成我们'}</h1>
            <p>从真实的日常，走到值得珍藏的一天。</p>
          </div>
          <span className="template-spec">
            {project?.duration || 90} 秒 · {project?.ratio || '16:9'}
          </span>
        </div>
        {error && (
          <p className="inline-error wedding-notice" role="alert">
            {error}
          </p>
        )}
        {notice && <output className="wedding-notice">{notice}</output>}
        {workflow && (workflow.automatic || workflow.currentStep <= 2) && (
          <section className="wedding-automatic wedding-panel">
            <div>
              <span className="eyebrow">ONE CLICK FILM</span>
              <h2>
                {project?.filmUrl
                  ? '你的完整影片已就绪'
                  : '故事交给我们，一键生成成片'}
              </h2>
              <p>
                自动完成文案、旁白、分镜、人物画面、动态视频、配乐和字幕。所有提示词与中间素材都会保留。
              </p>
            </div>
            {project?.filmUrl ? (
              <div className="wedding-action-row">
                <a
                  className="primary-button"
                  href={`${project.filmUrl}?download=1`}
                >
                  <Download size={16} />
                  下载完整成片
                </a>
                <a
                  className="secondary-button"
                  href={project.filmUrl}
                  target="_blank"
                  rel="noreferrer"
                >
                  播放成片
                </a>
              </div>
            ) : (
              <button
                className="primary-button"
                disabled={
                  busy || running || !canWrite || !project?.brief.trim()
                }
                onClick={() => void runAutomatic()}
              >
                {running ? (
                  <LoaderCircle size={17} className="spin" />
                ) : (
                  <ArrowRight size={17} />
                )}
                {running
                  ? `正在制作 · 第 ${workflow.currentStep}/14 步`
                  : workflow.automatic
                    ? '继续一键生成'
                    : '一键生成完整影片'}
              </button>
            )}
            {running && <output>{project?.events.at(-1)?.message}</output>}
          </section>
        )}
        {!workflow ? (
          <div className="wedding-panel">
            <p>
              {project
                ? '该工程没有婚礼向导，请从模板库选择「把日子写成我们」。'
                : '正在读取故事工程…'}
            </p>
            <Link href="/new" className="secondary-button">
              查看模板
            </Link>
          </div>
        ) : (
          <div className="wedding-layout">
            <aside className="wedding-steps">
              <div className="wedding-progress">
                <strong>
                  {workflow.completed
                    ? '新人已验收'
                    : `第 ${workflow.currentStep} / 14 步`}
                </strong>
                <progress
                  value={
                    workflow.steps.filter(
                      (s) =>
                        s.status === 'confirmed' || s.status === 'automated',
                    ).length
                  }
                  max={14}
                  aria-label="婚礼制作进度"
                />
              </div>
              <nav aria-label="制作阶段">
                {workflow.steps.map((s) => (
                  <button
                    type="button"
                    key={s.number}
                    onClick={() => {
                      setViewStep(s.number);
                      setKind('');
                      setText('');
                      setEvidence('');
                      setNotice('');
                    }}
                    aria-current={stepNumber === s.number ? 'step' : undefined}
                    className={stepNumber === s.number ? 'selected' : ''}
                  >
                    <span>
                      {s.status === 'confirmed' || s.status === 'automated' ? (
                        <Check size={15} />
                      ) : (
                        String(s.number).padStart(2, '0')
                      )}
                    </span>
                    <span>
                      {s.name}
                      <small>
                        {s.status === 'confirmed' || s.status === 'automated'
                          ? s.status === 'automated'
                            ? '已自动完成'
                            : '当前版本已确认'
                          : s.status === 'needs_review'
                            ? '返工后待复核'
                            : s.status === 'awaiting_confirmation'
                              ? '材料已返回，待确认'
                              : '等待制作'}
                      </small>
                    </span>
                  </button>
                ))}
              </nav>
              <a
                href={`/api/projects/${project!.id}/wedding/sources`}
                className="wedding-source"
              >
                <Download size={15} />
                下载原始模板与 Prompt
              </a>
            </aside>
            <section
              className="wedding-panel"
              aria-labelledby="wedding-step-title"
            >
              <div className="wedding-step-heading">
                <span className="eyebrow">
                  STEP {String(stepNumber).padStart(2, '0')}
                </span>
                <h2 id="wedding-step-title">{step?.name}</h2>
                <p>{spec.description}</p>
              </div>
              <div className="wedding-action-row">
                {stepNumber === 1 && (
                  <a
                    className="secondary-button"
                    href="/templates/wedding-story-intake.html"
                    target="_blank"
                    rel="noreferrer"
                  >
                    <FileText size={16} />
                    打开原版故事采集卡
                  </a>
                )}
                {stepNumber >= 2 && (
                  <a
                    className="secondary-button"
                    href={`/api/projects/${project!.id}/wedding/pack`}
                  >
                    <Download size={16} />
                    下载当前制作材料
                  </a>
                )}
                {stepNumber === 9 && (
                  <a
                    className="secondary-button"
                    href={`/api/projects/${project!.id}/wedding/pack?kind=video`}
                  >
                    <Download size={16} />
                    下载逐镜首帧与视频词
                  </a>
                )}
                {current && !workflow.automatic && spec.action && (
                  <button
                    className="primary-button"
                    disabled={busy || running || !canWrite}
                    onClick={() => void run()}
                  >
                    {running ? (
                      <LoaderCircle className="spin" size={16} />
                    ) : (
                      <ArrowRight size={16} />
                    )}
                    {spec.label}
                  </button>
                )}
              </div>
              {running && (
                <output className="wedding-notice">
                  <LoaderCircle size={16} className="spin" />
                  {project?.events.at(-1)?.message || '正在制作当前阶段…'}
                </output>
              )}
              {stepNumber >= 7 &&
                stepNumber <= 9 &&
                !!project?.shots.length && (
                  <details className="wedding-shot-prompts">
                    <summary>
                      查看逐镜提示词与时间表（{project.shots.length} 镜）
                    </summary>
                    {project.shots.map((s) => (
                      <article key={s.id}>
                        <strong>
                          {s.id} · {s.title} · {s.timelineStart.toFixed(2)}–
                          {(s.timelineStart + s.editSeconds).toFixed(2)} 秒
                        </strong>
                        <strong>生图提示词</strong>
                        <pre>{s.imagePrompt}</pre>
                        <strong>视频动作提示词</strong>
                        <pre>{s.prompt}</pre>
                      </article>
                    ))}
                  </details>
                )}
              <div className="wedding-artifacts">
                {step?.artifacts.map((item) => (
                  <Artifact key={item.id} item={item} />
                ))}
              </div>
              {!step?.artifacts.length && (
                <div className="wedding-empty">
                  <FileText size={28} />
                  <p>本步材料会保存在这里</p>
                  <span>提交后可以查看、试听和下载当前版本。</span>
                </div>
              )}
              {current &&
                canWrite &&
                (!workflow.automatic ||
                  stepNumber === 1 ||
                  stepNumber === 14) && (
                  <>
                    <form
                      className="wedding-upload"
                      onSubmit={(e) => {
                        e.preventDefault();
                        void save();
                      }}
                    >
                      <h3>补充本步材料</h3>
                      <div className="wedding-form-row">
                        <div>
                          <label htmlFor="wedding-kind">材料类型</label>
                          <select
                            id="wedding-kind"
                            value={selectedKind}
                            onChange={(e) => setKind(e.target.value)}
                            disabled={busy || running}
                          >
                            {spec.kinds.map(([value, label]) => (
                              <option value={value} key={value}>
                                {label}
                              </option>
                            ))}
                          </select>
                        </div>
                        {needsShot && (
                          <div>
                            <label htmlFor="wedding-shot">对应镜头</label>
                            <select
                              id="wedding-shot"
                              value={shotId || project?.shots[0]?.id || ''}
                              onChange={(e) => setShotId(e.target.value)}
                            >
                              {project?.shots.map((s) => (
                                <option value={s.id} key={s.id}>
                                  {s.id} · {s.title}
                                </option>
                              ))}
                            </select>
                          </div>
                        )}
                      </div>
                      {textual && (
                        <>
                          <label htmlFor="wedding-text">
                            {selectedKind === 'facts'
                              ? '粘贴完整故事卡'
                              : '材料正文'}
                          </label>
                          <textarea
                            id="wedding-text"
                            value={text}
                            onChange={(e) => setText(e.target.value)}
                            rows={selectedKind === 'facts' ? 10 : 6}
                            maxLength={200000}
                            placeholder={
                              selectedKind === 'timing'
                                ? '{"cues":[{"id":"C01","start":0.5,"end":3.2,"text":"实际听到的正文"}]}'
                                : '粘贴本次完整内容；提交新版本后需要重新确认。'
                            }
                          />
                        </>
                      )}
                      <div className="wedding-action-row">
                        {textual && (
                          <button
                            className="primary-button"
                            disabled={busy || running || !text.trim()}
                            type="submit"
                          >
                            保存材料
                          </button>
                        )}
                        <button
                          type="button"
                          className="secondary-button"
                          disabled={busy || running}
                          onClick={() => upload.current?.click()}
                        >
                          <Upload size={16} />
                          上传原文件
                        </button>
                        <input
                          ref={upload}
                          type="file"
                          hidden
                          onChange={(e) => {
                            const file = e.target.files?.[0];
                            if (file) void save(file);
                          }}
                        />
                      </div>
                    </form>
                    {!!step?.artifacts.length && (
                      <form
                        className="wedding-approval"
                        onSubmit={(e) => {
                          e.preventDefault();
                          void approve();
                        }}
                      >
                        <div>
                          <span className="eyebrow">REVIEW THIS VERSION</span>
                          <h3>
                            {couple ? '记录新人的明确确认' : '制作方检查与确认'}
                          </h3>
                          <p>
                            {couple
                              ? '填写新人对当前版本的具体反馈，由你的账号记录转述。'
                              : '请完成本步要求的通读、试听或观看，再记录检查结论。'}
                          </p>
                        </div>
                        <label htmlFor="wedding-evidence">确认反馈</label>
                        <textarea
                          id="wedding-evidence"
                          value={evidence}
                          onChange={(e) => setEvidence(e.target.value)}
                          rows={3}
                          maxLength={4000}
                          placeholder="记录确认的内容，以及需要保留的具体意见。"
                          required
                        />
                        <button
                          className="primary-button"
                          type="submit"
                          disabled={busy || running || !evidence.trim()}
                        >
                          <Check size={16} />
                          确认此版本并继续
                        </button>
                      </form>
                    )}
                  </>
                )}
              {step && stepNumber <= workflow.currentStep && canWrite && (
                <details className="wedding-reopen">
                  <summary>
                    <RefreshCw size={14} />
                    返工此阶段
                  </summary>
                  <p>保留文件与旧确认记录，从本阶段起重新审核后续成果。</p>
                  <label htmlFor="wedding-reason">返工原因</label>
                  <input
                    id="wedding-reason"
                    value={reason}
                    onChange={(e) => setReason(e.target.value)}
                    maxLength={2000}
                  />
                  <button
                    className="secondary-button"
                    disabled={busy || running || !reason.trim()}
                    onClick={() => void reopen()}
                  >
                    开始返工
                  </button>
                </details>
              )}
              {!!step?.approvals.length && (
                <div className="wedding-receipts">
                  {step.approvals.map((a) => (
                    <p key={a.by}>
                      <Check size={14} />
                      <span>
                        {a.by === 'couple'
                          ? '新人确认（制作方记录）'
                          : '制作方确认'}
                        ：{a.evidence}
                      </span>
                    </p>
                  ))}
                </div>
              )}
            </section>
          </div>
        )}
      </main>
    </div>
  );
}
