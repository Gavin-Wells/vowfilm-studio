'use client';
import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { KeyRound, LogOut, ShieldCheck, Monitor } from 'lucide-react';
import { PlatformPage } from '@/components/platform-page';
import { useAccount } from '@/components/account-provider';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { api } from '@/lib/api';
import { roleNames, dateTime } from '@/lib/platform';
export default function AccountPage() {
  const { auth, refresh } = useAccount();
  const router = useRouter();
  const [sessions, setSessions] = useState<
    { current: boolean; createdAt: string; expires: number; agent: string }[]
  >([]);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [code, setCode] = useState('');
  useEffect(() => {
    api<typeof sessions>('auth/sessions')
      .then(setSessions)
      .catch((e) => setError(e.message));
  }, []);
  async function logout(all: boolean) {
    setBusy(true);
    setError('');
    try {
      await api(`auth/${all ? 'logout-all' : 'logout'}`, {
        method: 'POST',
        body: '{}',
      });
      await refresh();
      router.replace('/login');
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function change(e: React.SyntheticEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    setBusy(true);
    setError('');
    try {
      const v = await api<{ recoveryCode: string }>('auth/password', {
        method: 'POST',
        body: JSON.stringify(Object.fromEntries(f)),
      });
      setCode(v.recoveryCode);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <PlatformPage
      eyebrow="ACCOUNT & SECURITY"
      title="账号与安全"
      description="管理身份凭据和登录设备。"
    >
      {error && (
        <p className="inline-error" role="alert">
          {error}
        </p>
      )}
      <div className="platform-columns">
        <section className="platform-card">
          <ShieldCheck />
          <h2>{auth?.user.name}</h2>
          <p>{auth?.user.email}</p>
          <span className="platform-badge">
            {roleNames[auth?.user.role || '']}
          </span>
          <p className="subtle">
            注册于 {auth && dateTime(auth.user.createdAt)}
          </p>
          <div className="button-row">
            <Button
              variant="outline"
              disabled={busy}
              onClick={() => void logout(false)}
            >
              <LogOut />
              退出登录
            </Button>
            <Button
              variant="outline"
              disabled={busy}
              onClick={() => void logout(true)}
            >
              退出全部设备
            </Button>
          </div>
        </section>
        <section className="platform-card">
          <KeyRound />
          <h2>修改密码</h2>
          {code ? (
            <>
              <p>密码已更新，所有设备已退出。请保存新的恢复码，旧码已失效。</p>
              <code className="recovery-code">{code}</code>
              <Button
                onClick={() => {
                  void refresh();
                  router.replace('/login');
                }}
              >
                已保存，重新登录
              </Button>
            </>
          ) : (
            <form onSubmit={change} className="platform-form">
              <label htmlFor="account-field-1">
                当前密码
                <Input
                  id="account-field-1"
                  name="currentPassword"
                  type="password"
                  required
                  autoComplete="current-password"
                />
              </label>
              <label htmlFor="account-field-2">
                新密码
                <Input
                  id="account-field-2"
                  name="password"
                  type="password"
                  required
                  minLength={12}
                  autoComplete="new-password"
                />
                <small>至少 12 个字符，最多 72 字节。</small>
              </label>
              <Button type="submit" disabled={busy}>
                {busy ? '正在更新…' : '更新密码并退出所有设备'}
              </Button>
            </form>
          )}
        </section>
      </div>
      <section className="platform-card">
        <h2>有效登录会话</h2>
        <p>会话有效期为 7 天，权限变化或密码重置会使会话失效。</p>
        <div className="session-list">
          {sessions.map((s, i) => (
            <div key={i}>
              <Monitor size={20} />
              <div>
                <strong>{s.current ? '当前设备' : '其他设备'}</strong>
                <p>{s.agent || '未提供设备信息'}</p>
                <small>{dateTime(s.createdAt)} 登录</small>
              </div>
            </div>
          ))}
        </div>
      </section>
    </PlatformPage>
  );
}
