'use client';
import { useCallback, useEffect, useState } from 'react';
import Link from 'next/link';
import { Calculator, Save, ShieldCheck, Users, PlusCircle } from 'lucide-react';
import { PlatformPage } from '@/components/platform-page';
import { useAccount } from '@/components/account-provider';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { ChoiceSelect } from '@/components/choice-select';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs';
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from '@/components/ui/table';
import { api } from '@/lib/api';
import {
  formText,
  credits,
  dateTime,
  roleNames,
  actionNames,
  unitNames,
  type Pricing,
  type User,
} from '@/lib/platform';
import { sceneChoices } from '@/lib/creative';
export default function AdminPage() {
  const { auth, refresh } = useAccount();
  const canUsers = !!auth?.permissions.includes('users:manage');
  const canPricing = !!auth?.permissions.includes('pricing:manage');
  const [creditUser, setCreditUser] = useState('');
  const [users, setUsers] = useState<User[]>([]);
  const [pricing, setPricing] = useState<Pricing | null>(null);
  const [roles, setRoles] = useState<Record<string, string[]>>({});
  const [audit, setAudit] = useState<
    {
      actor: string;
      action: string;
      target: string;
      detail: string;
      createdAt: string;
    }[]
  >([]);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [busy, setBusy] = useState(false);
  const [simulation, setSimulation] = useState({
    action: 'generate',
    scene: 'wedding',
    seconds: 60,
    shots: 16,
  });
  const [estimate, setEstimate] = useState<{
    amount: number;
    quantity: number;
  } | null>(null);
  const load = useCallback(async () => {
    try {
      const [u, p, r] = await Promise.all([
        api<User[]>('admin/users'),
        api<Pricing>('billing/pricing'),
        api<Record<string, string[]>>('admin/roles'),
      ]);
      setUsers(u);
      setPricing(p);
      setRoles(r);
      if (canUsers) setAudit(await api<typeof audit>('admin/audit'));
    } catch (e) {
      setError((e as Error).message);
    }
  }, [canUsers]);
  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);
  async function mutate(
    path: string,
    body: unknown,
    message: string,
    method = 'POST',
  ) {
    setBusy(true);
    setError('');
    setNotice('');
    try {
      await api(path, { method, body: JSON.stringify(body) });
      setNotice(message);
      await load();
      await refresh();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  function priceField(
    index: number,
    key: 'base' | 'rate' | 'minimum',
    value: string,
  ) {
    if (!pricing) return;
    setPricing({
      ...pricing,
      rules: pricing.rules.map((r, i) =>
        i === index ? { ...r, [key]: Math.round(Number(value) * 1000) } : r,
      ),
    });
    setEstimate(null);
  }
  async function simulate() {
    setBusy(true);
    setError('');
    try {
      setEstimate(
        await api('admin/simulate', {
          method: 'POST',
          body: JSON.stringify({ pricing, ...simulation }),
        }),
      );
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <PlatformPage
      eyebrow="PLATFORM ADMINISTRATION"
      title="管理中心"
      description="集中管理账号、计费规则与额度，关键操作保留审计记录。"
    >
      <div className="page-toolbar">
        <span className="platform-badge">
          <ShieldCheck size={14} />
          {roleNames[auth?.user.role || '']}
        </span>
        {auth?.permissions.includes('config:manage') && (
          <Link className="secondary-button" href="/settings">
            创作引擎配置 →
          </Link>
        )}
      </div>
      {error && (
        <p role="alert" className="inline-error">
          {error}
        </p>
      )}
      {notice && <output className="platform-success">{notice}</output>}
      <Tabs defaultValue={canUsers ? 'users' : 'credit'}>
        <TabsList>
          {canUsers && <TabsTrigger value="users">成员与权限</TabsTrigger>}
          <TabsTrigger value="credit">积分充值</TabsTrigger>
          {canPricing && <TabsTrigger value="pricing">价格模型</TabsTrigger>}
          {canUsers && <TabsTrigger value="audit">操作审计</TabsTrigger>}
        </TabsList>
        {canUsers && (
          <TabsContent value="users">
            <section className="platform-card">
              <h2>
                <Users size={20} />
                成员管理
              </h2>
              <p>
                修改角色或停用账号将使该账号的登录会话失效；至少保留一位管理员。
              </p>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>账号</TableHead>
                    <TableHead>角色</TableHead>
                    <TableHead>状态</TableHead>
                    <TableHead>可用积分</TableHead>
                    <TableHead>操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {users.map((u) => (
                    <TableRow key={u.id}>
                      <TableCell>
                        <strong>{u.name}</strong>
                        <small>{u.email}</small>
                      </TableCell>
                      <TableCell>
                        <ChoiceSelect
                          label={`${u.name}的角色`}
                          value={u.role}
                          onChange={(role) =>
                            setUsers(
                              users.map((v) =>
                                v.id === u.id ? { ...v, role } : v,
                              ),
                            )
                          }
                          items={Object.entries(roleNames).map(
                            ([value, label]) => ({ value, label }),
                          )}
                        />
                      </TableCell>
                      <TableCell>
                        <ChoiceSelect
                          label={`${u.name}的状态`}
                          value={u.disabled ? 'disabled' : 'active'}
                          onChange={(value) =>
                            setUsers(
                              users.map((v) =>
                                v.id === u.id
                                  ? { ...v, disabled: value === 'disabled' }
                                  : v,
                              ),
                            )
                          }
                          items={[
                            { value: 'active', label: '正常' },
                            { value: 'disabled', label: '已停用' },
                          ]}
                        />
                      </TableCell>
                      <TableCell>{credits(u.balance - u.held)}</TableCell>
                      <TableCell>
                        <Button
                          variant="outline"
                          disabled={busy || u.id === auth?.user.id}
                          onClick={() =>
                            void mutate(
                              `admin/users/${u.id}`,
                              { role: u.role, disabled: u.disabled },
                              '成员权限已更新',
                              'PATCH',
                            )
                          }
                        >
                          保存
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </section>
            <section className="platform-card">
              <h2>角色权限矩阵</h2>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>权限</TableHead>
                    {Object.entries(roleNames).map(([k, v]) => (
                      <TableHead key={k}>{v}</TableHead>
                    ))}
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {Object.entries({
                    'project:read': '查看自己的项目',
                    'project:write': '创建与执行项目',
                    'project:all': '访问全部项目',
                    'users:manage': '管理账号与角色',
                    'billing:manage': '核验充值',
                    'pricing:manage': '发布价格',
                    'config:manage': '配置创作引擎',
                  }).map(([p, label]) => (
                    <TableRow key={p}>
                      <TableCell>{label}</TableCell>
                      {Object.keys(roleNames).map((r) => (
                        <TableCell key={r}>
                          {roles[r]?.includes(p) ? '允许' : '—'}
                        </TableCell>
                      ))}
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </section>
          </TabsContent>
        )}
        <TabsContent value="credit">
          <section className="platform-card narrow-card">
            <h2>
              <PlusCircle size={20} />
              核验并充值积分
            </h2>
            <p>仅在核验凭据后入账。相同凭据重复提交不会重复充值。</p>
            <form
              className="platform-form"
              onSubmit={(e) => {
                e.preventDefault();
                const f = new FormData(e.currentTarget);
                void mutate(
                  'admin/credit',
                  {
                    userId: creditUser || users[0]?.id,
                    amount: Math.round(Number(f.get('amount')) * 1000),
                    reference: formText(f, 'reference'),
                    note: formText(f, 'note'),
                  },
                  '积分已入账，流水已保存',
                );
              }}
            >
              <label htmlFor="admin-field-1">
                收款账号
                <ChoiceSelect
                  id="admin-field-1"
                  label="选择充值账号"
                  value={creditUser || users[0]?.id || ''}
                  onChange={setCreditUser}
                  items={users.map((u) => ({
                    value: u.id,
                    label: `${u.name} · ${u.email}`,
                  }))}
                />
              </label>
              <label htmlFor="admin-field-2">
                积分数量
                <Input
                  id="admin-field-2"
                  name="amount"
                  type="number"
                  min="0.001"
                  max="100000"
                  step="0.001"
                  required
                />
              </label>
              <label htmlFor="admin-field-3">
                唯一凭据编号
                <Input
                  id="admin-field-3"
                  name="reference"
                  required
                  minLength={4}
                  maxLength={120}
                  placeholder="例如：银行流水编号或内部赠送单号"
                />
              </label>
              <label htmlFor="admin-field-4">
                充值原因
                <Input
                  id="admin-field-4"
                  name="note"
                  required
                  maxLength={500}
                  placeholder="请注明已核验的依据"
                />
              </label>
              <Button type="submit" disabled={busy || !users.length}>
                {busy ? '正在入账…' : '确认核验并入账'}
              </Button>
            </form>
          </section>
        </TabsContent>
        {canPricing && (
          <TabsContent value="pricing">
            {pricing && (
              <>
                <section className="platform-card">
                  <div className="page-toolbar">
                    <div>
                      <h2>可配置计费模型</h2>
                      <p>
                        当前版本 v{pricing.version}。积分精确到
                        0.001，发布后生成新版本。
                      </p>
                    </div>
                    <Button
                      disabled={busy}
                      onClick={() =>
                        void mutate(
                          'admin/pricing',
                          pricing,
                          '新价格版本已发布；历史账单和已签发报价保持原价',
                        )
                      }
                    >
                      <Save />
                      发布新版本
                    </Button>
                  </div>
                  <div className="formula">
                    报价 = max(最低价, ⌈(起步价 + 单价 × 数量) × 场景系数⌉)
                  </div>
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>计费动作</TableHead>
                        <TableHead>计量方式</TableHead>
                        <TableHead>起步价 / 积分</TableHead>
                        <TableHead>单价 / 积分</TableHead>
                        <TableHead>最低价 / 积分</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {pricing.rules.map((r, i) => (
                        <TableRow key={r.action}>
                          <TableCell>{actionNames[r.action]}</TableCell>
                          <TableCell>
                            <ChoiceSelect
                              label={`${actionNames[r.action]}计量方式`}
                              value={r.unit}
                              onChange={(unit) => {
                                setPricing({
                                  ...pricing,
                                  rules: pricing.rules.map((v, j) =>
                                    j === i ? { ...v, unit } : v,
                                  ),
                                });
                                setEstimate(null);
                              }}
                              items={Object.entries(unitNames).map(
                                ([value, label]) => ({ value, label }),
                              )}
                            />
                          </TableCell>
                          {(['base', 'rate', 'minimum'] as const).map((key) => (
                            <TableCell key={key}>
                              <Input
                                aria-label={`${actionNames[r.action]}${{ base: '起步价', rate: '单价', minimum: '最低价' }[key]}`}
                                type="number"
                                min="0"
                                max="100000"
                                step="0.001"
                                value={r[key] / 1000}
                                onChange={(e) =>
                                  priceField(i, key, e.target.value)
                                }
                              />
                            </TableCell>
                          ))}
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                  <div className="factor-grid">
                    {sceneChoices.map((s) => (
                      <label key={s.id} htmlFor={`factor-${s.id}`}>
                        {s.name}系数
                        <Input
                          id={`factor-${s.id}`}
                          type="number"
                          min="0"
                          max="10"
                          step="0.0001"
                          value={pricing.sceneFactors[s.id] / 10000}
                          onChange={(e) => {
                            setPricing({
                              ...pricing,
                              sceneFactors: {
                                ...pricing.sceneFactors,
                                [s.id]: Math.round(
                                  Number(e.target.value) * 10000,
                                ),
                              },
                            });
                            setEstimate(null);
                          }}
                        />
                      </label>
                    ))}
                  </div>
                  <p className="subtle">
                    按次和按任务数量均为
                    1；按秒取目标时长，按镜头取镜头数量。内部步骤不重复扣费，失败或取消释放全部冻结积分。
                  </p>
                </section>
                <section className="platform-card">
                  <h2>
                    <Calculator size={20} />
                    发布前试算
                  </h2>
                  <p>
                    使用上方尚未发布的规则，通过后端计价函数计算，不扣除积分。
                  </p>
                  <div className="simulation-grid">
                    <label htmlFor="admin-field-5">
                      动作
                      <ChoiceSelect
                        id="admin-field-5"
                        label="试算动作"
                        value={simulation.action}
                        onChange={(action) => {
                          setSimulation({ ...simulation, action });
                          setEstimate(null);
                        }}
                        items={Object.entries(actionNames).map(
                          ([value, label]) => ({ value, label }),
                        )}
                      />
                    </label>
                    <label htmlFor="admin-field-6">
                      场景
                      <ChoiceSelect
                        id="admin-field-6"
                        label="试算场景"
                        value={simulation.scene}
                        onChange={(scene) => {
                          setSimulation({ ...simulation, scene });
                          setEstimate(null);
                        }}
                        items={sceneChoices.map((s) => ({
                          value: s.id,
                          label: s.name,
                        }))}
                      />
                    </label>
                    <label htmlFor="admin-field-7">
                      秒数
                      <Input
                        id="admin-field-7"
                        type="number"
                        min={1}
                        max={240}
                        value={simulation.seconds}
                        onChange={(e) => {
                          setSimulation({
                            ...simulation,
                            seconds: Number(e.target.value),
                          });
                          setEstimate(null);
                        }}
                      />
                    </label>
                    <label htmlFor="admin-field-8">
                      镜头数
                      <Input
                        id="admin-field-8"
                        type="number"
                        min={1}
                        max={100}
                        value={simulation.shots}
                        onChange={(e) => {
                          setSimulation({
                            ...simulation,
                            shots: Number(e.target.value),
                          });
                          setEstimate(null);
                        }}
                      />
                    </label>
                  </div>
                  <div className="page-toolbar">
                    <Button
                      variant="outline"
                      disabled={busy}
                      onClick={() => void simulate()}
                    >
                      计算报价
                    </Button>
                    <output className="estimate-result" aria-live="polite">
                      {estimate
                        ? `${credits(estimate.amount)} 积分 · 数量 ${estimate.quantity}`
                        : '调整参数后计算'}
                    </output>
                  </div>
                </section>
              </>
            )}
          </TabsContent>
        )}
        {canUsers && (
          <TabsContent value="audit">
            <section className="platform-card">
              <h2>最近 200 条管理与安全记录</h2>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>时间</TableHead>
                    <TableHead>操作者</TableHead>
                    <TableHead>操作</TableHead>
                    <TableHead>对象</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {audit.map((a, i) => (
                    <TableRow key={i}>
                      <TableCell>{dateTime(a.createdAt)}</TableCell>
                      <TableCell>
                        {users.find((u) => u.id === a.actor)?.name || a.actor}
                      </TableCell>
                      <TableCell>{a.action}</TableCell>
                      <TableCell>
                        {users.find((u) => u.id === a.target)?.email ||
                          a.target}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </section>
          </TabsContent>
        )}
      </Tabs>
    </PlatformPage>
  );
}
