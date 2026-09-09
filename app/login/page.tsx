'use client';
import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { BrandLogo } from '@/components/brand-logo';
import { ArrowRight, KeyRound, ShieldCheck, LoaderCircle } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { api } from '@/lib/api';
import { formText } from '@/lib/platform';
import { useAccount } from '@/components/account-provider';
export default function LoginPage() {
  const router = useRouter();
  const { refresh } = useAccount();
  const [mode, setMode] = useState('login');
  const [setup, setSetup] = useState(false);
  const [ready, setReady] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [code, setCode] = useState('');
  const [saved, setSaved] = useState(false);
  useEffect(() => {
    api<{ setupRequired: boolean }>('auth/status')
      .then((v) => {
        setSetup(v.setupRequired);
        if (v.setupRequired) setMode('register');
        setReady(true);
      })
      .catch((e) => setError(e.message));
  }, []);
  async function submit(e: React.SyntheticEvent<HTMLFormElement>) {
    e.preventDefault();
    const data = new FormData(e.currentTarget);
    setBusy(true);
    setError('');
    try {
      const body: Record<string, string> = {
        email: formText(data, 'email'),
        password: formText(data, 'password'),
      };
      if (mode === 'register') {
        body.name = formText(data, 'name');
        body.setupToken = formText(data, 'setupToken');
      }
      if (mode === 'recover')
        body.recoveryCode = formText(data, 'recoveryCode');
      const result = await api<{ recoveryCode?: string }>(`auth/${mode}`, {
        method: 'POST',
        body: JSON.stringify(body),
      });
      await refresh();
      if (result.recoveryCode) {
        setCode(result.recoveryCode);
        setSaved(false);
      } else router.replace('/');
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <main className="auth-layout">
      <section className="auth-story">
        <div className="auth-brand">
          <BrandLogo large />
        </div>
        <div>
          <p className="eyebrow">EVERY STORY DESERVES A FILM</p>
          <h1>
            让每一种故事，
            <br />
            都有自己的光。
          </h1>
          <p>
            从珍贵的家族记忆，到打动人心的品牌表达。一个创作空间，连接灵感、影像与观众。
          </p>
          <div className="auth-scenes">
            <span>婚礼影片</span>
            <span>家族传承</span>
            <span>爱情纪念</span>
            <span>电商营销</span>
          </div>
        </div>
        <p className="auth-footnote">
          <ShieldCheck size={16} />
          独立账号 · 私有项目 · 清晰计费
        </p>
      </section>
      <section className="auth-panel">
        <div className="auth-card">
          {code ? (
            <>
              <KeyRound size={30} />
              <h2>保存你的恢复码</h2>
              <p>
                这是找回账号的唯一自助凭据，只显示一次。请保存在密码管理器中；使用后会换发新码。
              </p>
              <code className="recovery-code">{code}</code>
              <label className="check-label">
                <input
                  type="checkbox"
                  checked={saved}
                  onChange={(e) => setSaved(e.target.checked)}
                />
                我已安全保存恢复码
              </label>
              <Button
                disabled={!saved}
                onClick={() => {
                  if (mode === 'recover') {
                    setCode('');
                    setMode('login');
                    setSetup(false);
                  } else router.replace('/');
                }}
              >
                继续 <ArrowRight />
              </Button>
            </>
          ) : (
            <>
              <p className="eyebrow">YOUR CREATIVE SPACE</p>
              <h2>
                {setup
                  ? '初始化你的工作台'
                  : mode === 'register'
                    ? '创建账号'
                    : mode === 'recover'
                      ? '找回账号'
                      : '欢迎回来'}
              </h2>
              <p>
                {setup
                  ? '创建平台管理员，开始配置引擎、价格与成员权限。'
                  : mode === 'recover'
                    ? '使用注册时保存的一次性恢复码重置密码。'
                    : '登录后管理你的影片、素材与创作额度。'}
              </p>
              {!setup && (
                <Tabs
                  value={mode}
                  onValueChange={(v) => {
                    setMode(String(v));
                    setError('');
                  }}
                >
                  <TabsList>
                    <TabsTrigger value="login">登录</TabsTrigger>
                    <TabsTrigger value="register">注册</TabsTrigger>
                    <TabsTrigger value="recover">找回密码</TabsTrigger>
                  </TabsList>
                </Tabs>
              )}
              <form className="platform-form" onSubmit={submit}>
                {mode === 'register' && (
                  <label htmlFor="login-field-1">
                    称呼
                    <Input
                      id="login-field-1"
                      name="name"
                      required
                      maxLength={40}
                      autoComplete="nickname"
                    />
                  </label>
                )}
                <label htmlFor="login-field-2">
                  邮箱
                  <Input
                    id="login-field-2"
                    name="email"
                    type="email"
                    required
                    autoComplete="username"
                    placeholder="you@example.com"
                  />
                </label>
                {setup && (
                  <label htmlFor="login-field-3">
                    管理员初始化密钥
                    <Input
                      id="login-field-3"
                      name="setupToken"
                      type="password"
                      required
                      autoComplete="off"
                    />
                    <small>
                      由部署者从服务器的 data/admin-setup.txt 获取。
                    </small>
                  </label>
                )}
                {mode === 'recover' && (
                  <label htmlFor="login-field-4">
                    一次性恢复码
                    <Input
                      id="login-field-4"
                      name="recoveryCode"
                      type="password"
                      required
                      autoComplete="off"
                    />
                  </label>
                )}
                <label htmlFor="login-field-5">
                  {mode === 'recover' ? '新密码' : '密码'}
                  <Input
                    id="login-field-5"
                    name="password"
                    type="password"
                    required
                    minLength={mode === 'login' ? 1 : 12}
                    autoComplete={
                      mode === 'login' ? 'current-password' : 'new-password'
                    }
                  />
                  {mode !== 'login' && (
                    <small>至少 12 个字符，最多 72 字节，支持粘贴密码。</small>
                  )}
                </label>
                {error && (
                  <p role="alert" className="inline-error">
                    {error}
                  </p>
                )}
                <Button type="submit" disabled={busy || !ready}>
                  {busy ? <LoaderCircle className="spin" /> : <ArrowRight />}
                  {mode === 'login'
                    ? '进入工作台'
                    : mode === 'recover'
                      ? '重置密码'
                      : setup
                        ? '创建管理员'
                        : '创建账号'}
                </Button>
              </form>
            </>
          )}
        </div>
      </section>
    </main>
  );
}
