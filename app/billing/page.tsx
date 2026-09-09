'use client';
import { useCallback, useEffect, useState } from 'react';
import Link from 'next/link';
import {
  Wallet as WalletIcon,
  LockKeyhole,
  ReceiptText,
  RefreshCw,
} from 'lucide-react';
import { PlatformPage } from '@/components/platform-page';
import { Button } from '@/components/ui/button';
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from '@/components/ui/table';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs';
import { Empty } from '@/components/ui/empty';
import { api } from '@/lib/api';
import {
  credits,
  dateTime,
  actionNames,
  unitNames,
  type Wallet,
  type Pricing,
} from '@/lib/platform';
import { useAccount } from '@/components/account-provider';
const entryNames: Record<string, string> = {
  credit: '充值入账',
  hold: '任务冻结',
  consume: '任务结算',
  release: '额度释放',
};
export default function BillingPage() {
  const [wallet, setWallet] = useState<Wallet | null>(null);
  const [pricing, setPricing] = useState<Pricing | null>(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const { refresh } = useAccount();
  const load = useCallback(async () => {
    setBusy(true);
    try {
      const [w, p] = await Promise.all([
        api<Wallet>('billing'),
        api<Pricing>('billing/pricing'),
      ]);
      setWallet(w);
      setPricing(p);
      setError('');
      await refresh();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }, [refresh]);
  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);
  return (
    <PlatformPage
      eyebrow="CREDITS & BILLING"
      title="每一笔创作，都有记录"
      description="确认报价后冻结积分；任务成功结算，失败或取消释放。"
    >
      <div className="page-toolbar">
        <p>积分仅用于平台内计费，不等同于货币。</p>
        <Button variant="outline" disabled={busy} onClick={() => void load()}>
          <RefreshCw size={16} />
          {busy ? '刷新中…' : '刷新账单'}
        </Button>
      </div>
      {error && (
        <p className="inline-error" role="alert">
          {error}
        </p>
      )}
      <div className="balance-grid">
        <section className="platform-card balance-main">
          <WalletIcon />
          <p>可用积分</p>
          <strong>
            {wallet ? credits(wallet.user.balance - wallet.user.held) : '—'}
          </strong>
          <small>可用于新的创作任务</small>
        </section>
        <section className="platform-card">
          <LockKeyhole />
          <p>冻结积分</p>
          <strong>{wallet ? credits(wallet.user.held) : '—'}</strong>
          <small>等待当前任务完成结算</small>
        </section>
        <section className="platform-card">
          <ReceiptText />
          <p>账户总积分</p>
          <strong>{wallet ? credits(wallet.user.balance) : '—'}</strong>
          <small>可用积分 + 冻结积分</small>
        </section>
      </div>
      <div className="platform-note">
        <div>
          <strong>需要增加创作额度？</strong>
          <p>联系管理员核验充值。每笔入账都保留操作者、原因和凭据编号。</p>
        </div>
        <Link href="/account">查看我的账号 →</Link>
      </div>
      <Tabs defaultValue="ledger">
        <TabsList>
          <TabsTrigger value="ledger">账户流水</TabsTrigger>
          <TabsTrigger value="charges">任务账单</TabsTrigger>
          <TabsTrigger value="pricing">当前价格</TabsTrigger>
        </TabsList>
        <TabsContent value="ledger">
          <section className="platform-card">
            <h2>账户流水</h2>
            <p>最近 200 条余额与冻结变动。</p>
            {wallet?.entries.length ? (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>时间 / 类型</TableHead>
                    <TableHead>积分变动</TableHead>
                    <TableHead>冻结变动</TableHead>
                    <TableHead>变后总额</TableHead>
                    <TableHead>备注 / 凭据</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {wallet.entries.map((e) => (
                    <TableRow key={e.id}>
                      <TableCell>
                        <strong>{entryNames[e.kind]}</strong>
                        <small>{dateTime(e.createdAt)}</small>
                      </TableCell>
                      <TableCell>
                        {e.delta > 0 ? '+' : ''}
                        {credits(e.delta)}
                      </TableCell>
                      <TableCell>
                        {e.heldDelta > 0 ? '+' : ''}
                        {credits(e.heldDelta)}
                      </TableCell>
                      <TableCell>{credits(e.balanceAfter)}</TableCell>
                      <TableCell>
                        <span>{actionNames[e.note] || e.note}</span>
                        <small>{e.reference}</small>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            ) : (
              <Empty>
                还没有流水。充值或确认任务报价后，记录将显示在这里。
              </Empty>
            )}
          </section>
        </TabsContent>
        <TabsContent value="charges">
          <section className="platform-card">
            <h2>任务账单</h2>
            <p>每张账单保留提交时的价格版本与计量数量，最近 100 条。</p>
            {wallet?.charges.length ? (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>任务</TableHead>
                    <TableHead>计价</TableHead>
                    <TableHead>积分</TableHead>
                    <TableHead>状态</TableHead>
                    <TableHead>提交时间</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {wallet.charges.map((c) => (
                    <TableRow key={c.id}>
                      <TableCell>
                        <Link href={`/?project=${c.projectId}`}>
                          {actionNames[c.quote.action]}
                        </Link>
                        <small>{c.id}</small>
                      </TableCell>
                      <TableCell>
                        v{c.quote.version} · {unitNames[c.quote.rule.unit]} ×{' '}
                        {c.quote.quantity}
                      </TableCell>
                      <TableCell>{credits(c.amount)}</TableCell>
                      <TableCell>
                        {
                          {
                            held: '冻结中',
                            settled: '已结算',
                            released: '已释放',
                          }[c.state]
                        }
                      </TableCell>
                      <TableCell>{dateTime(c.createdAt)}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            ) : (
              <Empty>
                还没有付费任务。创建项目免费，执行任务前会展示报价。
              </Empty>
            )}
          </section>
        </TabsContent>
        <TabsContent value="pricing">
          <section className="platform-card">
            <h2>价格版本 v{pricing?.version}</h2>
            <p>
              一键生成的内部步骤不重复扣费；单独发起的编排、重做和合成为独立任务。
            </p>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>动作</TableHead>
                  <TableHead>单位</TableHead>
                  <TableHead>起步价</TableHead>
                  <TableHead>单价</TableHead>
                  <TableHead>最低价</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {pricing?.rules.map((r) => (
                  <TableRow key={r.action}>
                    <TableCell>{actionNames[r.action]}</TableCell>
                    <TableCell>{unitNames[r.unit]}</TableCell>
                    <TableCell>{credits(r.base)}</TableCell>
                    <TableCell>{credits(r.rate)}</TableCell>
                    <TableCell>{credits(r.minimum)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
            <p className="subtle">
              最终报价还会应用场景系数。价格调整后，已确认的账单保留原价格。
            </p>
          </section>
        </TabsContent>
      </Tabs>
    </PlatformPage>
  );
}
