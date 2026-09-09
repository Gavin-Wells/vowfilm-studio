import type { Dispatch, SetStateAction, SyntheticEvent } from 'react';
import { Check, LoaderCircle } from 'lucide-react';
import { CreativeFields } from '@/components/creative-fields';
import type { ProjectDraft } from '@/lib/creative';

export function DirectorSettings({
  draft,
  setDraft,
  onSubmit,
  disabled,
  busy,
  dirty,
  error,
}: {
  draft: ProjectDraft;
  setDraft: Dispatch<SetStateAction<ProjectDraft>>;
  onSubmit: (e: SyntheticEvent<HTMLFormElement>) => void;
  disabled: boolean;
  busy: boolean;
  dirty: boolean;
  error: string;
}) {
  return (
    <form className="project-form director-settings-form" onSubmit={onSubmit}>
      <fieldset className="director-fields" disabled={disabled || busy}>
        <CreativeFields
          draft={draft}
          setDraft={setDraft}
          disabled={disabled || busy}
          error={error}
        />
      </fieldset>
      {error && (
        <p className="inline-error" role="alert">
          {error}
        </p>
      )}
      <div className="director-savebar">
        <output
          className={`director-save-status${dirty ? ' is-dirty' : ''}`}
          aria-live="polite"
        >
          {busy ? (
            <LoaderCircle size={14} className="spin" aria-hidden="true" />
          ) : !dirty ? (
            <Check size={14} aria-hidden="true" />
          ) : null}
          {busy ? '正在处理…' : dirty ? '保存后需重新编排' : '已保存'}
        </output>
        {dirty && (
          <button
            className="secondary-button director-save-button"
            disabled={disabled || busy || !dirty}
          >
            {busy ? (
              <LoaderCircle size={14} className="spin" />
            ) : (
              <Check size={14} />
            )}
            保存修改
          </button>
        )}
      </div>
    </form>
  );
}
