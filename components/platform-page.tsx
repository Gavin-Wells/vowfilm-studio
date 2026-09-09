import Link from 'next/link';
import type { ReactNode } from 'react';
import { ArrowLeft } from 'lucide-react';
export function PlatformPage({
  eyebrow,
  title,
  description,
  children,
}: {
  eyebrow: string;
  title: string;
  description: string;
  children: ReactNode;
}) {
  return (
    <main className="platform-page">
      <header className="platform-heading">
        <div>
          <p className="eyebrow">{eyebrow}</p>
          <h1>{title}</h1>
          <p>{description}</p>
        </div>
        <Link href="/" className="secondary-button">
          <ArrowLeft size={16} />
          返回工作台
        </Link>
      </header>
      {children}
    </main>
  );
}
