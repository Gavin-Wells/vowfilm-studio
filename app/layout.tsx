import type { Metadata } from 'next';
import './globals.css';

export const metadata: Metadata = {
  title: '誓光 Vowfilm · AI 视频创作工作台',
  description:
    '从照片与故事出发，编排分镜、生成镜头，为值得铭记的时刻制作一部电影。',
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="zh-CN" className="dark">
      <body>{children}</body>
    </html>
  );
}
