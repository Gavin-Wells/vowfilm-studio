export class ApiError extends Error {
  constructor(
    message: string,
    public status: number,
  ) {
    super(message);
  }
}
export async function api<T>(path: string, options?: RequestInit): Promise<T> {
  const headers = new Headers(options?.headers);
  if (!(options?.body instanceof FormData) && !headers.has('Content-Type'))
    headers.set('Content-Type', 'application/json');
  headers.set('X-Vowfilm-CSRF', '1');
  const response = await fetch(`/api/${path}`, {
    signal: AbortSignal.timeout(options?.method ? 120000 : 10000),
    ...options,
    headers,
  });
  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as {
      error?: string;
    };
    if (
      response.status === 401 &&
      !path.startsWith('auth/') &&
      typeof window !== 'undefined'
    )
      window.dispatchEvent(new Event('vowfilm:unauthorized'));
    throw new ApiError(
      body.error || '创作服务暂时不可用，请稍后重试。',
      response.status,
    );
  }
  return response.json();
}
