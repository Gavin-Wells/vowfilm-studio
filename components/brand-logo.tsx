import Image from 'next/image';

export function BrandLogo({ large = false }: { large?: boolean }) {
  return (
    <span className={`brand-lockup${large ? ' brand-lockup-large' : ''}`}>
      <Image
        className="brand-mark"
        src="/logo-mark.svg"
        alt=""
        width={large ? 56 : 40}
        height={large ? 56 : 40}
        unoptimized
        loading="eager"
      />
      <span className="brand-wordmark">
        <span className="brand-name">誓光</span>
        <span className="brand-english">
          {large ? 'VOWFILM STUDIO' : 'VOWFILM'}
        </span>
      </span>
    </span>
  );
}
