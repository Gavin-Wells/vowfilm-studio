'use client';
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from 'react';
import { usePathname, useRouter } from 'next/navigation';
import Link from 'next/link';
import {
  LoaderCircle,
  Wallet,
  UserRound,
  ShieldCheck,
  Clapperboard,
} from 'lucide-react';
import { api, ApiError } from '@/lib/api';
import type { Auth } from '@/lib/platform';
import { credits } from '@/lib/platform';
import { Button } from '@/components/ui/button';
const AccountContext = createContext<{
  auth: Auth | null;
  refresh: () => Promise<void>;
}>({ auth: null, refresh: async () => {} });
export const useAccount = () => useContext(AccountContext);
export function AccountProvider({ children }: { children: ReactNode }) {
  const [auth, setAuth] = useState<Auth | null>(null);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState('');
  const path = usePathname();
  const router = useRouter();
  const refresh = useCallback(async () => {
    try {
      setAuth(await api<Auth>('auth/me'));
      setError('');
    } catch (e) {
      setAuth(null);
      setError(
        e instanceof ApiError && e.status === 401 ? '' : (e as Error).message,
      );
    } finally {
      setLoaded(true);
    }
  }, []);
  useEffect(() => {
    void Promise.resolve().then(refresh);
    const reset = () => {
      setAuth(null);
      setLoaded(true);
    };
    window.addEventListener('vowfilm:unauthorized', reset);
    return () => window.removeEventListener('vowfilm:unauthorized', reset);
  }, [refresh]);
  useEffect(() => {
    if (loaded && !auth && !error && path !== '/login')
      router.replace('/login');
  }, [loaded, auth, error, path, router]);
  const restricted =
    auth &&
    ((path === '/settings' && !auth.permissions.includes('config:manage')) ||
      (path === '/admin' &&
        !auth.permissions.some(
          (p) => p === 'users:manage' || p === 'billing:manage',
        )) ||
      (path === '/new' && !auth.permissions.includes('project:write')));
  return (
    <AccountContext.Provider value={{ auth, refresh }}>
      {path === '/login' ? (
        children
      ) : !loaded || (!auth && !error) ? (
        <output className="account-loading">
          <LoaderCircle className="spin" /> 正在验证登录状态…
        </output>
      ) : error ? (
        <div className="account-loading">
          <p role="alert">{error}</p>
          <Button onClick={() => void refresh()}>重试连接</Button>
        </div>
      ) : restricted ? (
        <div className="account-loading">
          <ShieldCheck />
          <h1>当前账号没有此页面的权限</h1>
          <Link href="/">返回创作工作台</Link>
        </div>
      ) : (
        <>
          <nav className="account-nav" aria-label="账号导航">
            <Link href="/" aria-current={path === '/' ? 'page' : undefined}>
              <Clapperboard size={16} />
              创作工作台
            </Link>
            <Link
              href="/billing"
              aria-current={path === '/billing' ? 'page' : undefined}
            >
              <Wallet size={16} />
              积分与账单{' '}
              <span>
                {credits((auth?.user.balance || 0) - (auth?.user.held || 0))}
              </span>
            </Link>
            {auth?.permissions.some(
              (p) => p === 'users:manage' || p === 'billing:manage',
            ) && (
              <Link
                href="/admin"
                aria-current={path === '/admin' ? 'page' : undefined}
              >
                <ShieldCheck size={16} />
                管理中心
              </Link>
            )}
            <Link
              className="account-nav-user"
              href="/account"
              aria-current={path === '/account' ? 'page' : undefined}
            >
              <UserRound size={16} />
              {auth?.user.name}
            </Link>
          </nav>
          {children}
        </>
      )}
    </AccountContext.Provider>
  );
}
