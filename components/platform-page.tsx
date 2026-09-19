import Link from 'next/link';
import type { ReactNode } from 'react';
import { ArrowLeft } from 'lucide-react';
export function PlatformPage({
  title,
  children,
}: {
  title: string;
  children: ReactNode;
}) {
  return (
    <main className="platform-page">
      <div className="platform-page-top">
        <h1 className="platform-page-title">{title}</h1>
        <Link href="/" className="secondary-button">
          <ArrowLeft size={16} />
          返回工作台
        </Link>
      </div>
      {children}
    </main>
  );
}
