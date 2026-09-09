import type { Metadata } from 'next';
import './globals.css';
import { AccountProvider } from '@/components/account-provider';

export const metadata: Metadata = {
  title: '誓光 Vowfilm · AI 视频创作工作台',
  icons: {
    icon: { url: '/favicon.svg?v=2', type: 'image/svg+xml', sizes: 'any' },
  },
  description:
    '从婚礼、家族传承与爱情纪念，到商品展示和品牌故事，在独立创作空间中编排、生成并管理影片。',
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="zh-CN">
      <body>
        <AccountProvider>{children}</AccountProvider>
      </body>
    </html>
  );
}
