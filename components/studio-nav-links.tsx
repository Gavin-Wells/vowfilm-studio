'use client';

import Link from 'next/link';
import { ShieldCheck, UserRound, Wallet } from 'lucide-react';
import { useAccount } from '@/components/account-provider';
import { credits } from '@/lib/platform';

export function StudioNavLinks() {
  const { auth } = useAccount();
  if (!auth) return null;
  const balance = credits((auth.user.balance || 0) - (auth.user.held || 0));
  const canAdmin = auth.permissions.some(
    (p) => p === 'users:manage' || p === 'billing:manage',
  );
  return (
    <nav className="studio-nav-links" aria-label="快捷入口">
      <Link className="secondary-button compact-nav-link" href="/billing">
        <Wallet size={16} />
        <span>{balance}</span>
      </Link>
      {canAdmin && (
        <Link
          className="secondary-button compact-nav-link"
          href="/admin"
          title="管理中心"
        >
          <ShieldCheck size={16} />
        </Link>
      )}
      <Link className="secondary-button compact-nav-link" href="/account">
        <UserRound size={16} />
        <span className="nav-link-label">{auth.user.name}</span>
      </Link>
    </nav>
  );
}
