'use client';
import Link from 'next/link';
import { useState } from 'react';
import { Wallet, LoaderCircle } from 'lucide-react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { api } from '@/lib/api';
import { credits, actionNames, unitNames, type Quote } from '@/lib/platform';
import { useAccount } from '@/components/account-provider';
export function TaskQuote({
  quote,
  onClose,
  onComplete,
}: {
  quote: Quote | null;
  onClose: () => void;
  onComplete: () => Promise<void>;
}) {
  const { auth, refresh } = useAccount();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const available = (auth?.user.balance || 0) - (auth?.user.held || 0);
  async function confirm() {
    if (!quote) return;
    setBusy(true);
    setError('');
    try {
      const path =
        quote.action === 'shot'
          ? `projects/${quote.projectId}/shots/${quote.shotId}/generate`
          : `projects/${quote.projectId}/${quote.action}`;
      await api(path, {
        method: 'POST',
        body: '{}',
        headers: { 'X-Vowfilm-Quote': quote.id, 'Idempotency-Key': quote.id },
      });
      await refresh();
      await onComplete();
      onClose();
    } catch (e) {
      setError((e as Error).message);
      await refresh();
    } finally {
      setBusy(false);
    }
  }
  return (
    <Dialog
      open={!!quote}
      onOpenChange={(v) => {
        if (!v && !busy) {
          setError('');
          onClose();
        }
      }}
    >
      <DialogContent className="quote-dialog">
        <DialogHeader>
          <DialogTitle>
            <Wallet size={20} />
            确认任务报价
          </DialogTitle>
          <DialogDescription>
            确认后冻结积分，成功结算，失败或取消释放。
          </DialogDescription>
        </DialogHeader>
        {quote && (
          <>
            <div className="quote-amount">
              <span>{actionNames[quote.action]}</span>
              <strong>
                {credits(quote.amount)} <small>积分</small>
              </strong>
            </div>
            <dl className="quote-details">
              <div>
                <dt>价格版本</dt>
                <dd>v{quote.version}</dd>
              </div>
              <div>
                <dt>计量</dt>
                <dd>
                  {unitNames[quote.rule.unit]} × {quote.quantity}
                </dd>
              </div>
              <div>
                <dt>单价 / 起步价</dt>
                <dd>
                  {credits(quote.rule.rate)} / {credits(quote.rule.base)}
                </dd>
              </div>
              <div>
                <dt>场景系数</dt>
                <dd>{quote.factor / 10000} 倍</dd>
              </div>
              <div>
                <dt>当前可用</dt>
                <dd>{credits(available)} 积分</dd>
              </div>
            </dl>
            {available < quote.amount && (
              <p className="inline-error">
                积分不足，请先联系管理员充值。
                <Link href="/billing">查看账单</Link>
              </p>
            )}
            {error && (
              <p role="alert" className="inline-error">
                {error}
              </p>
            )}
            <p className="subtle">
              报价有效期 10
              分钟。内部生成步骤不重复扣费，后续单独重做会重新报价。
            </p>
            <div className="button-row">
              <Button variant="outline" disabled={busy} onClick={onClose}>
                暂不执行
              </Button>
              <Button
                disabled={busy || available < quote.amount}
                onClick={() => void confirm()}
              >
                {busy ? <LoaderCircle className="spin" /> : null}确认并开始
              </Button>
            </div>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
