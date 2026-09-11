'use client';

import { useRef, useState, type Dispatch, type SetStateAction } from 'react';
import { ChevronDown, SlidersHorizontal, Sparkles } from 'lucide-react';
import { ChoiceSelect } from '@/components/choice-select';
import {
  Collapsible,
  CollapsibleTrigger,
  CollapsibleContent,
} from '@/components/ui/collapsible';
import {
  sceneChoice,
  sceneOccasions,
  sceneExamples,
  styleChoices,
  type ProjectDraft,
} from '@/lib/creative';

export function CreativeFields({
  draft,
  setDraft,
  disabled,
  error,
}: {
  draft: ProjectDraft;
  setDraft: Dispatch<SetStateAction<ProjectDraft>>;
  disabled: boolean;
  error: string;
}) {
  const [advanced, setAdvanced] = useState(false);
  const [inspiration, setInspiration] = useState(false);
  const [dismissedError, setDismissedError] = useState('');
  const templated = draft.creationMode === 'template';
  const promptInput = useRef<HTMLTextAreaElement>(null);
  const duration =
    draft.duration < 60
      ? `${draft.duration} 秒`
      : `${draft.duration / 60} 分钟`;
  const style = styleChoices.find((s) => s.id === draft.style)?.name;
  const briefLabel =
    draft.scene === 'commerce'
      ? '商品信息'
      : draft.scene === 'family'
        ? '家族素材'
        : '故事素材';
  const promptPlaceholder = templated
    ? '例如：光线更温暖、表情自然。镜头顺序和场景沿用模板。'
    : draft.scene === 'commerce'
      ? '例如：展示一次使用过程，配简短旁白，不加字幕。'
      : '例如：温暖自然、少量字幕，避免群像。';
  const advancedOpen = advanced || Boolean(error && error !== dismissedError);

  return (
    <div className="creative-fields">
      <div className="creative-field">
        <label htmlFor="creative-brief">{briefLabel}</label>
        <textarea
          id="creative-brief"
          className="creative-brief-input"
          rows={4}
          maxLength={4000}
          value={draft.brief}
          onChange={(e) => setDraft({ ...draft, brief: e.target.value })}
          placeholder={sceneChoice(draft.scene).briefHint}
        />
      </div>

      <Collapsible
        className="creative-prompt-section"
        open={inspiration}
        onOpenChange={setInspiration}
      >
        <div className="creative-field-heading">
          <label htmlFor="custom-prompt">
            {templated ? '画面补充' : '拍摄要求'}{' '}
            <span className="creative-optional">选填</span>
          </label>
          {!templated && (
            <CollapsibleTrigger
              type="button"
              className="creative-inline-action"
              disabled={disabled}
            >
              <Sparkles size={14} aria-hidden="true" />
              {inspiration ? '收起灵感' : '参考灵感'}
            </CollapsibleTrigger>
          )}
        </div>
        <textarea
          ref={promptInput}
          id="custom-prompt"
          rows={3}
          maxLength={6000}
          value={draft.customPrompt}
          onChange={(e) => setDraft({ ...draft, customPrompt: e.target.value })}
          placeholder={promptPlaceholder}
        />
        <CollapsibleContent className="creative-inspiration-panel">
          <div className="prompt-inspiration" aria-label="追加创作灵感">
            {sceneExamples(draft.scene).map((example) => (
              <button
                type="button"
                key={example.name}
                onClick={() => {
                  setDraft((value) => ({
                    ...value,
                    customPrompt: [value.customPrompt, example.text]
                      .filter(Boolean)
                      .join('\n')
                      .slice(0, 6000),
                  }));
                  setInspiration(false);
                  promptInput.current?.focus();
                }}
              >
                {example.name}
              </button>
            ))}
          </div>
        </CollapsibleContent>
      </Collapsible>

      <Collapsible
        className="creative-advanced"
        open={advancedOpen}
        onOpenChange={(open) => {
          setAdvanced(open);
          if (!open) setDismissedError(error);
        }}
      >
        <CollapsibleTrigger type="button" className="creative-advanced-trigger">
          <span className="creative-advanced-label">
            <SlidersHorizontal size={15} aria-hidden="true" />
            高级设置
          </span>
          <span className="creative-spec-summary">
            {duration} · {draft.ratio} · {style}
          </span>
          <ChevronDown
            size={16}
            className="creative-advanced-chevron"
            aria-hidden="true"
          />
        </CollapsibleTrigger>
        <CollapsibleContent className="creative-advanced-panel">
          <div className="creative-advanced-fields">
            {templated ? (
              <p className="fine-print">
                当前模板固定 {duration}、{draft.ratio}
                、六幕结构与分章造型。名称和片尾寄语可自行填写。
              </p>
            ) : (
              <>
                <fieldset className="creative-setting-group">
                  <legend>成片规格</legend>
                  <div className="creative-spec-grid">
                    <div className="creative-field">
                      <label htmlFor="project-duration">时长</label>
                      <ChoiceSelect
                        disabled={disabled}
                        id="project-duration"
                        value={String(draft.duration)}
                        onChange={(s) =>
                          setDraft({ ...draft, duration: Number(s) })
                        }
                        label="影片时长"
                        items={(draft.scene === 'commerce'
                          ? [15]
                          : [60, 120, 180, 240]
                        ).map((s) => ({
                          value: String(s),
                          label: s < 60 ? `${s} 秒` : `${s / 60} 分钟`,
                        }))}
                      />
                    </div>
                    <div className="creative-field">
                      <label htmlFor="project-ratio">画幅</label>
                      <ChoiceSelect
                        disabled={disabled}
                        id="project-ratio"
                        value={draft.ratio}
                        onChange={(ratio) => setDraft({ ...draft, ratio })}
                        label="影片画幅"
                        items={[
                          { value: '16:9', label: '16:9 横屏' },
                          { value: '9:16', label: '9:16 竖屏' },
                        ]}
                      />
                    </div>
                  </div>
                </fieldset>

                <fieldset className="creative-setting-group">
                  <legend>导演偏好</legend>
                  <div className="creative-setting-row">
                    <label htmlFor="project-occasion">用途</label>
                    <ChoiceSelect
                      disabled={disabled}
                      id="project-occasion"
                      value={draft.occasion}
                      onChange={(occasion) => setDraft({ ...draft, occasion })}
                      label="影片用途"
                      items={Object.entries(sceneOccasions(draft.scene)).map(
                        ([value, label]) => ({ value, label }),
                      )}
                    />
                  </div>
                  <div className="creative-setting-row">
                    <label htmlFor="director-style">风格</label>
                    <ChoiceSelect
                      disabled={disabled}
                      id="director-style"
                      value={draft.style}
                      onChange={(style) => setDraft({ ...draft, style })}
                      label="影片风格"
                      items={styleChoices.map((s) => ({
                        value: s.id,
                        label: s.name,
                      }))}
                    />
                  </div>
                  {draft.scene !== 'commerce' && (
                    <div className="creative-setting-row">
                      <label htmlFor="wardrobe-mode">造型</label>
                      <ChoiceSelect
                        disabled={disabled}
                        id="wardrobe-mode"
                        value={draft.wardrobeMode}
                        onChange={(wardrobeMode) =>
                          setDraft({ ...draft, wardrobeMode })
                        }
                        label="造型变化"
                        items={[
                          { value: 'auto', label: '导演自动安排' },
                          { value: 'fixed', label: '全片固定一套' },
                          { value: 'custom', label: '自定义造型' },
                        ]}
                      />
                    </div>
                  )}
                  {draft.scene !== 'commerce' &&
                    draft.wardrobeMode === 'custom' && (
                      <div className="creative-field">
                        <label htmlFor="wardrobe-prompt">
                          造型顺序{' '}
                          <span className="creative-optional">必填</span>
                        </label>
                        <textarea
                          id="wardrobe-prompt"
                          rows={3}
                          maxLength={1500}
                          value={draft.wardrobePrompt}
                          onChange={(e) =>
                            setDraft({
                              ...draft,
                              wardrobePrompt: e.target.value,
                            })
                          }
                          placeholder="例如：日常装 → 中式礼服 → 婚纱西装。"
                        />
                      </div>
                    )}
                </fieldset>
              </>
            )}
            <fieldset className="creative-setting-group">
              <legend>名称与片尾</legend>
              <div className="creative-field">
                <div className="creative-field-heading">
                  <label htmlFor="project-title">
                    影片名称 <span className="creative-optional">选填</span>
                  </label>
                  {draft.title ? (
                    <button
                      type="button"
                      className="creative-inline-action"
                      onClick={() => setDraft({ ...draft, title: '' })}
                    >
                      <Sparkles size={14} aria-hidden="true" />
                      自动命名
                    </button>
                  ) : (
                    <span className="creative-auto-label">自动命名</span>
                  )}
                </div>
                <input
                  id="project-title"
                  maxLength={80}
                  value={draft.title}
                  onChange={(e) =>
                    setDraft({ ...draft, title: e.target.value })
                  }
                  placeholder="留空，根据素材自动命名"
                />
              </div>
              <div className="creative-field">
                <label htmlFor="ending-text">
                  片尾文字 <span className="creative-optional">选填</span>
                </label>
                <input
                  id="ending-text"
                  maxLength={20}
                  value={draft.endingText}
                  onChange={(e) =>
                    setDraft({ ...draft, endingText: e.target.value })
                  }
                  placeholder={
                    draft.scene === 'commerce'
                      ? '留空不加片尾字卡'
                      : '留空由导演安排'
                  }
                />
              </div>
            </fieldset>
          </div>
        </CollapsibleContent>
      </Collapsible>
    </div>
  );
}
